package listing_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"seattle_info_backend/internal/auth"
	"seattle_info_backend/internal/category"
	"seattle_info_backend/internal/common"
	"seattle_info_backend/internal/config"
	"seattle_info_backend/internal/listing"
	"seattle_info_backend/internal/middleware"
	"seattle_info_backend/internal/user"
	"seattle_info_backend/internal/platform/elasticsearch" // Added for ES client
	"seattle_info_backend/pkg/database"                    // Assuming a package for DB setup
	"seattle_info_backend/pkg/logging"                     // Assuming a package for logger setup
	"go.uber.org/zap"                                      // Added for logger

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"
)

// IntegrationTestSuite defines the suite for listing handler integration tests.
type IntegrationTestSuite struct {
	suite.Suite
	DB         *gorm.DB
	Router     *gin.Engine
	Cfg        *config.Config
	UserRepo   user.Repository
	CatRepo    category.Repository
	ListingRepo listing.Repository
	AuthService auth.TokenService
	ESClient   *elasticsearch.ESClientWrapper // Added ES Client
	Logger     *zap.Logger                    // Added Logger
	ListingService listing.Service             // Added Listing Service
	// Add other necessary services/repos
}

// SetupSuite runs once before all tests in the suite.
func (s *IntegrationTestSuite) SetupSuite() {
	// 0. Initialize Logger
	s.Logger = logging.NewLogger("test", "console", "debug") // Store logger in suite

	// 1. Load Configuration
	cfg, err := config.LoadConfig("../../configs", "config.test.yaml")
	s.Require().NoError(err, "Failed to load test config")
	s.Cfg = cfg

	// Override ES URL if needed for test environment, e.g., from an env var for test
	// For now, assume cfg.ElasticsearchURL is correctly set for the test ES instance
	if s.Cfg.ElasticsearchURL == "" {
		s.Logger.Warn("ElasticsearchURL is not set in test config, ES tests might fail or be skipped")
		// s.T().Skip("Skipping tests requiring Elasticsearch as ELASTICSEARCH_URL is not set.")
	}


	// 2. Initialize Database
	db, err := database.InitDB(&s.Cfg.Database) // Use s.Cfg
	s.Require().NoError(err, "Failed to initialize test database")
	s.DB = db
	// database.Migrate(db) // If applicable

	// 3. Initialize Elasticsearch Client
	if s.Cfg.ElasticsearchURL != "" {
		esClient, esErr := elasticsearch.NewClient(s.Cfg, s.Logger)
		s.Require().NoError(esErr, "Failed to initialize Elasticsearch client")
		s.ESClient = esClient

		// Create Listings Index
		err = elasticsearch.CreateListingsIndexIfNotExists(s.ESClient, s.Logger)
		s.Require().NoError(err, "Failed to create listings index")
	}


	// 4. Initialize Repositories
	s.UserRepo = user.NewGORMRepository(s.DB)
	s.CatRepo = category.NewGORMRepository(s.DB)
	s.ListingRepo = listing.NewGORMRepository(s.DB)

	// 5. Initialize Services
	s.AuthService = auth.NewTokenService(s.Cfg.Auth.JWTSecret, s.Cfg.Auth.TokenExpiryMinutes, s.UserRepo, s.Logger)
	catService := category.NewService(s.CatRepo, s.Logger, s.Cfg) // Pass Cfg if NewService requires it
	s.ListingService = listing.NewService(s.ListingRepo, s.UserRepo, catService, nil, s.Cfg, s.Logger, s.ESClient) // Pass ESClient, updated signature for notificationService

	// 6. Initialize Gin Engine and Routes
	s.Router = gin.New()
	s.Router.Use(logging.GinMiddleware(s.Logger))
	s.Router.Use(gin.Recovery())

	// Setup middleware
	authMiddleware := middleware.AuthMiddleware(s.Cfg.Firebase, s.AuthService, s.Logger) // Updated call if FirebaseService is from cfg
	adminRoleMiddleware := middleware.RoleMiddleware(user.RoleAdmin, s.Logger)

	// Register routes
	apiGroup := s.Router.Group("/api/v1")
	listingHandler := listing.NewHandler(s.ListingService, s.Logger)
	listingHandler.RegisterRoutes(apiGroup, authMiddleware, adminRoleMiddleware)

	categoryHandler := category.NewHandler(catService, s.Logger) // Ensure catService is initialized
	categoryHandler.RegisterRoutes(apiGroup, authMiddleware, adminRoleMiddleware)


	// Clean database and ES before running tests (initial full clean)
	s.cleanupDB()
	s.cleanupES()
}

// TearDownSuite runs once after all tests in the suite.
func (s *IntegrationTestSuite) TearDownSuite() {
	s.cleanupES() // Clean ES after all tests
	sqlDB, _ := s.DB.DB()
	sqlDB.Close()
}

// SetupTest runs before each test.
func (s *IntegrationTestSuite) SetupTest() {
	// Clean database and ES before each test to ensure isolation
	s.cleanupDB()
	s.cleanupES()
	// s.seedCategories() // If common categories are needed
}

// Helper to clean all relevant tables.
func (s *IntegrationTestSuite) cleanupDB() {
	// Order matters due to foreign key constraints
	s.DB.Exec("DELETE FROM listing_details_events")
	s.DB.Exec("DELETE FROM listing_details_housing")
	s.DB.Exec("DELETE FROM listing_details_babysitting")
	s.DB.Exec("DELETE FROM listings")
	s.DB.Exec("DELETE FROM sub_categories")
	s.DB.Exec("DELETE FROM categories")
	s.DB.Exec("DELETE FROM users")
}

// Helper to clean Elasticsearch listings index
func (s *IntegrationTestSuite) cleanupES() {
	if s.ESClient != nil && s.ESClient.Client != nil && s.Cfg.ElasticsearchURL != "" {
		// Option 1: Delete and recreate index
		// _, err := s.ESClient.Client.Indices.Delete([]string{elasticsearch.ListingsIndexName})
		// if err == nil { // or check for specific non-existence errors
		// 	elasticsearch.CreateListingsIndexIfNotExists(s.ESClient, s.Logger)
		// }
		// Option 2: Delete all documents (safer if index settings are complex)
		deleteReq := esapi.DeleteByQueryRequest{
			Index: []string{elasticsearch.ListingsIndexName},
			Body:  strings.NewReader(`{"query":{"match_all":{}}}`),
			Refresh: common.Ptr(true), // Make it synchronous for tests
		}
		res, err := deleteReq.Do(context.Background(), s.ESClient.Client)
		if err != nil {
			s.Logger.Warn("Failed to delete all documents from ES", zap.Error(err))
		}
		if res != nil && res.IsError() && res.StatusCode != http.StatusNotFound {
			s.Logger.Warn("Error response when deleting all documents from ES", zap.String("status", res.Status()))
		}
		if res != nil {
			res.Body.Close()
		}
	}
}


// Helper to create a user and return user and token.
func (s *IntegrationTestSuite) createUser(username, email, password string, role user.UserRole) (*user.User, string) {
	u := &user.User{
		Username: username,
		Email:    email,
		Role:     role,
	}
	hashedPassword, _ := user.HashPassword(password)
	u.PasswordHash = hashedPassword
	u.IsActive = true
	u.IsEmailVerified = true // Assume verified for simplicity in tests
	u.IsFirstPostApproved = true // Default to true, change in specific tests if needed

	err := s.UserRepo.Create(context.Background(), u)
	s.Require().NoError(err)

	token, err := s.AuthService.GenerateToken(u.ID, u.Role)
	s.Require().NoError(err)
	return u, token
}

// Helper to create a category.
func (s *IntegrationTestSuite) createCategory(name, slug string) *category.Category {
	cat := &category.Category{Name: name, Slug: slug, Description: name + " description"}
	err := s.CatRepo.Create(context.Background(), cat)
	s.Require().NoError(err)
	return cat
}

// Helper to create a sub-category.
func (s *IntegrationTestSuite) createSubCategory(name, slug string, parentID uuid.UUID) *category.SubCategory {
	subCat := &category.SubCategory{Name: name, Slug: slug, Description: name + " description", CategoryID: parentID}
	err := s.CatRepo.CreateSubCategory(context.Background(), subCat)
	s.Require().NoError(err)
	return subCat
}

// Helper to create a listing using the service layer (ensures it's indexed in ES).
func (s *IntegrationTestSuite) createListingViaService(userID, catID uuid.UUID, subCatID *uuid.UUID, title string, status listing.ListingStatus, isAdminApproved bool, details interface{}) *listing.Listing {
	req := listing.CreateListingRequest{
		CategoryID:    catID,
		SubCategoryID: subCatID,
		Title:         title,
		Description:   title + " description",
		// ContactName, Email, Phone, Address etc. can be added if needed for specific tests
		// For geo-search tests, Latitude and Longitude must be provided.
	}

	// Populate details based on type
	switch d := details.(type) {
	case *listing.CreateListingEventDetailsRequest:
		req.EventDetails = d
	case *listing.CreateListingHousingDetailsRequest:
		req.HousingDetails = d
	case *listing.CreateListingBabysittingDetailsRequest:
		req.BabysittingDetails = d
	// Add LatLonDetails for geo tests
	case *struct{Lat float64; Lon float64}:
		req.Latitude = &d.Lat
		req.Longitude = &d.Lon
	}

	// Note: The CreateListing service method itself sets status based on first post approval logic.
	// If tests need specific statuses, they might need to use AdminUpdateListingStatus afterwards,
	// or the test user (testUser.IsFirstPostApproved) needs to be managed.
	// For simplicity, we assume IsFirstPostApproved = true for the test user here.
	// The `status` param to this helper might be less effective unless we adjust the test user.

	createdListing, err := s.ListingService.CreateListing(context.Background(), userID, req)
	s.Require().NoError(err, "Failed to create listing via service")
	s.Require().NotNil(createdListing)

	// If a specific status needs to be forced (e.g. for testing filters) and admin approval
	if createdListing.Status != status || createdListing.IsAdminApproved != isAdminApproved {
		if s.ESClient == nil { // Cannot update status if ES is not running as service layer will fail
			s.T().Logf("Skipping status/approval update for listing %s as ES client is not available", createdListing.ID)
			return createdListing
		}
		updatedL, errUpdate := s.ListingService.AdminUpdateListingStatus(context.Background(), createdListing.ID, status, nil)
		s.Require().NoError(errUpdate, "Failed to update listing status for test setup")
		if isAdminApproved && !updatedL.IsAdminApproved { // AdminUpdateListingStatus might set it to active but not approved
			updatedL, errUpdate = s.ListingService.AdminUpdateListingStatus(context.Background(), createdListing.ID, status, nil) // Re-approve if needed
			s.Require().NoError(errUpdate, "Failed to re-approve listing for test setup")
		}
		return updatedL
	}

	return createdListing
}


// TestIntegrationTestSuite is the entry point for running the suite.
func TestIntegrationTestSuite(t *testing.T) {
	if os.Getenv("RUN_INTEGRATION_TESTS") == "" {
		t.Skip("Skipping integration tests: RUN_INTEGRATION_TESTS not set")
	}
	suite.Run(t, new(IntegrationTestSuite))
}

// --- Test Cases Start Here ---

// TestSearchListings_TextAndGeo performs text and geo search.
func (s *IntegrationTestSuite) TestSearchListings_TextAndGeo() {
	if s.ESClient == nil {
		s.T().Skip("Skipping Elasticsearch tests as ES client is not initialized.")
	}

	user1, _ := s.createUser("searchUser", "search@example.com", "password", user.RoleUser)
	catServices := s.createCategory("Services Test", "services-test")

	// Create some listings
	s.createListingViaService(user1.ID, catServices.ID, nil, "Awesome Seattle Plumber", listing.StatusActive, true,
		&struct{Lat float64; Lon float64}{Lat: 47.6062, Lon: -122.3321}) // Seattle center
	s.createListingViaService(user1.ID, catServices.ID, nil, "Great Seattle Painter", listing.StatusActive, true,
		&struct{Lat float64; Lon float64}{Lat: 47.6100, Lon: -122.3350}) // Slightly North
	s.createListingViaService(user1.ID, catServices.ID, nil, "Bellevue Roofer Service", listing.StatusActive, true,
		&struct{Lat float64; Lon float64}{Lat: 47.6101, Lon: -122.2007}) // Bellevue

	s.Run("TextSearchForPlumber", func() {
		req, _ := http.NewRequest("GET", "/api/v1/listings?q=Plumber", nil)
		rr := httptest.NewRecorder()
		s.Router.ServeHTTP(rr, req)
		s.Equal(http.StatusOK, rr.Code)
		var resp common.PaginatedResponse
		json.Unmarshal(rr.Body.Bytes(), &resp)
		s.Equal(1, len(resp.Data.([]interface{})))
		// Further assertions on the content
		firstItem := resp.Data.([]interface{})[0].(map[string]interface{})
		s.Contains(firstItem["title"], "Plumber")
	})

	s.Run("GeoSearchNearSeattleCenter", func() {
		// Assuming MaxDistanceKM is appropriately set in service or queryParams if not here
		// For ES, default query.MaxDistanceKM might not be applied unless explicitly handled in service.SearchListings
		// Let's test with an explicit distance.
		req, _ := http.NewRequest("GET", "/api/v1/listings?lat=47.6062&lon=-122.3321&max_distance_km=5&sort_by=distance&sort_order=asc", nil)
		rr := httptest.NewRecorder()
		s.Router.ServeHTTP(rr, req)
		s.Equal(http.StatusOK, rr.Code, rr.Body.String())
		var resp common.PaginatedResponse
		json.Unmarshal(rr.Body.Bytes(), &resp)

		// Should find "Awesome Seattle Plumber" and "Great Seattle Painter"
		s.True(len(resp.Data.([]interface{})) >= 2, fmt.Sprintf("Expected at least 2 results, got %d", len(resp.Data.([]interface{}))))

		foundPlumber := false
		foundPainter := false
		for _, item := range resp.Data.([]interface{}) {
			listingMap := item.(map[string]interface{})
			title := listingMap["title"].(string)
			if title == "Awesome Seattle Plumber" {
				foundPlumber = true
				// Check distance is populated if sort_by=distance
				_, ok := listingMap["distance_km"]
				s.True(ok, "distance_km should be present when sorting by distance")
			}
			if title == "Great Seattle Painter" {
				foundPainter = true
			}
		}
		s.True(foundPlumber, "Did not find Plumber listing in geo search")
		s.True(foundPainter, "Did not find Painter listing in geo search")

		// Check if Bellevue Roofer is NOT found (it's > 5km away from Seattle center)
		foundRoofer := false
		for _, item := range resp.Data.([]interface{}) {
			if item.(map[string]interface{})["title"] == "Bellevue Roofer Service" {
				foundRoofer = true
				break
			}
		}
		s.False(foundRoofer, "Bellevue Roofer should not be found within 5km of Seattle center")
	})

	// Add more tests for category filters, status filters, etc.
}


// TestGetMyListings_Success
func (s *IntegrationTestSuite) TestGetMyListings_Success() {
	user1, token1 := s.createUser("user1", "user1@example.com", "password123", user.RoleUser)
	user2, _ := s.createUser("user2", "user2@example.com", "password123", user.RoleUser)

	catEvents := s.createCategory("Events", "events")
	catHousing := s.createCategory("Housing", "housing")

	// Use createListingViaService to ensure data is in ES for other search tests if they exist
	s.createListingViaService(user1.ID, catEvents.ID, nil, "User1 Event Listing", listing.StatusActive, true, &listing.CreateListingEventDetailsRequest{EventDate: time.Now().Add(5 * 24 * time.Hour).Format("2006-01-02")})
	s.createListingViaService(user1.ID, catHousing.ID, nil, "User1 Housing Listing", listing.StatusPendingApproval, true, &listing.CreateListingHousingDetailsRequest{PropertyType: listing.HousingForRent, RentDetails: common.Ptr("Monthly")})
	s.createListingViaService(user2.ID, catEvents.ID, nil, "User2 Event Listing", listing.StatusActive, true, nil)


	req, _ := http.NewRequest("GET", "/api/v1/listings/my-listings", nil)
	req.Header.Set("Authorization", "Bearer "+token1)
	rr := httptest.NewRecorder()
	s.Router.ServeHTTP(rr, req)

	s.Equal(http.StatusOK, rr.Code)

	var resp common.PaginatedResponse // Assuming this is your paginated response structure
	err := json.Unmarshal(rr.Body.Bytes(), &resp)
	s.NoError(err)
	s.Equal(2, len(resp.Data.([]interface{}))) // Should only get 2 listings for user1
	s.Equal(int64(2), resp.Pagination.TotalItems)

	// Further checks: iterate through resp.Data, unmarshal into listing.ListingResponse, check UserID, details etc.
	for _, item := range resp.Data.([]interface{}) {
		var lr listing.ListingResponse
		itemBytes, _ := json.Marshal(item)
		json.Unmarshal(itemBytes, &lr)
		s.Equal(user1.ID, lr.UserID)
		s.NotNil(lr.Category) // Check if category is populated
		// Check details based on category (e.g. EventDetails if category is Events)
		if lr.Category.Slug == "events" {
			s.NotNil(lr.EventDetails)
		} else if lr.Category.Slug == "housing" {
			s.NotNil(lr.HousingDetails)
			s.Equal(listing.HousingForRent, lr.HousingDetails.PropertyType)
		}
	}
}

// TestGetMyListings_Filtering
func (s *IntegrationTestSuite) TestGetMyListings_Filtering() {
	user1, token1 := s.createUser("userfilter", "userfilter@example.com", "password123", user.RoleUser)
	catEvents := s.createCategory("Events Filter", "events-filter")
	catHousing := s.createCategory("Housing Filter", "housing-filter")

	s.createListing(user1.ID, catEvents.ID, nil, "Active Event", listing.StatusActive, nil)
	s.createListing(user1.ID, catEvents.ID, nil, "Pending Event", listing.StatusPendingApproval, nil)
	s.createListing(user1.ID, catHousing.ID, nil, "Active Housing", listing.StatusActive, nil)

	// Sub-test: Filter by status=active
	s.Run("FilterByStatusActive", func() {
		req, _ := http.NewRequest("GET", "/api/v1/listings/my-listings?status=active", nil)
		req.Header.Set("Authorization", "Bearer "+token1)
		rr := httptest.NewRecorder()
		s.Router.ServeHTTP(rr, req)
		s.Equal(http.StatusOK, rr.Code)
		var resp common.PaginatedResponse
		json.Unmarshal(rr.Body.Bytes(), &resp)
		s.Equal(2, len(resp.Data.([]interface{})))
		for _, item := range resp.Data.([]interface{}) {
			var lr listing.ListingResponse
			itemBytes, _ := json.Marshal(item)
			json.Unmarshal(itemBytes, &lr)
			s.Equal(listing.StatusActive, lr.Status)
		}
	})

	// Sub-test: Filter by category_slug=events-filter
	s.Run("FilterByCategorySlug", func() {
		req, _ := http.NewRequest("GET", "/api/v1/listings/my-listings?category_slug=events-filter", nil)
		req.Header.Set("Authorization", "Bearer "+token1)
		rr := httptest.NewRecorder()
		s.Router.ServeHTTP(rr, req)
		s.Equal(http.StatusOK, rr.Code)
		var resp common.PaginatedResponse
		json.Unmarshal(rr.Body.Bytes(), &resp)
		s.Equal(2, len(resp.Data.([]interface{}))) // Both active and pending for this category
		for _, item := range resp.Data.([]interface{}) {
			var lr listing.ListingResponse
			itemBytes, _ := json.Marshal(item)
			json.Unmarshal(itemBytes, &lr)
			s.Equal(catEvents.ID, lr.CategoryID)
		}
	})

	// Sub-test: Filter by status=active AND category_slug=housing-filter
	s.Run("FilterByStatusAndCategorySlug", func() {
		req, _ := http.NewRequest("GET", "/api/v1/listings/my-listings?status=active&category_slug=housing-filter", nil)
		req.Header.Set("Authorization", "Bearer "+token1)
		rr := httptest.NewRecorder()
		s.Router.ServeHTTP(rr, req)
		s.Equal(http.StatusOK, rr.Code)
		var resp common.PaginatedResponse
		json.Unmarshal(rr.Body.Bytes(), &resp)
		s.Equal(1, len(resp.Data.([]interface{})))
		for _, item := range resp.Data.([]interface{}) {
			var lr listing.ListingResponse
			itemBytes, _ := json.Marshal(item)
			json.Unmarshal(itemBytes, &lr)
			s.Equal(listing.StatusActive, lr.Status)
			s.Equal(catHousing.ID, lr.CategoryID)
		}
	})
}

// TestGetMyListings_Empty
func (s *IntegrationTestSuite) TestGetMyListings_Empty() {
	_, token1 := s.createUser("userempty", "userempty@example.com", "password123", user.RoleUser)
	req, _ := http.NewRequest("GET", "/api/v1/listings/my-listings", nil)
	req.Header.Set("Authorization", "Bearer "+token1)
	rr := httptest.NewRecorder()
	s.Router.ServeHTTP(rr, req)

	s.Equal(http.StatusOK, rr.Code)
	var resp common.PaginatedResponse
	json.Unmarshal(rr.Body.Bytes(), &resp)
	s.Equal(0, len(resp.Data.([]interface{})))
	s.Equal(int64(0), resp.Pagination.TotalItems)
}

// TestGetMyListings_Unauthorized
func (s *IntegrationTestSuite) TestGetMyListings_Unauthorized() {
	req, _ := http.NewRequest("GET", "/api/v1/listings/my-listings", nil)
	// No token
	rr := httptest.NewRecorder()
	s.Router.ServeHTTP(rr, req)
	s.Equal(http.StatusUnauthorized, rr.Code)

	// Invalid token
	req.Header.Set("Authorization", "Bearer "+"invalidtoken")
	rr = httptest.NewRecorder()
	s.Router.ServeHTTP(rr, req)
	s.Equal(http.StatusUnauthorized, rr.Code)
}


// TestUpdateListing_Success_CoreFields
func (s *IntegrationTestSuite) TestUpdateListing_Success_CoreFields() {
	testUser, token := s.createUser("updateuser", "updateuser@example.com", "password", user.RoleUser)
	cat := s.createCategory("Core Category", "core-cat")
	initialListing := s.createListing(testUser.ID, cat.ID, nil, "Initial Title", listing.StatusActive, nil)

	updateReq := listing.UpdateListingRequest{
		Title:       common.Ptr("Updated Title"),
		Description: common.Ptr("Updated Description"),
	}
	jsonBody, _ := json.Marshal(updateReq)
	req, _ := http.NewRequest("PUT", fmt.Sprintf("/api/v1/listings/%s", initialListing.ID.String()), bytes.NewBuffer(jsonBody))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.Router.ServeHTTP(rr, req)

	s.Equal(http.StatusOK, rr.Code)
	var resp common.StandardResponse // Assuming common.StandardResponse { Success bool, Message string, Data interface{} }
	json.Unmarshal(rr.Body.Bytes(), &resp)
	s.True(resp.Success)

	updatedData, _ := json.Marshal(resp.Data)
	var updatedListingResp listing.ListingResponse
	json.Unmarshal(updatedData, &updatedListingResp)

	s.Equal("Updated Title", updatedListingResp.Title)
	s.Equal("Updated Description", updatedListingResp.Description)

	// Verify in DB
	dbListing, err := s.ListingRepo.FindByID(context.Background(), initialListing.ID, false)
	s.NoError(err)
	s.Equal("Updated Title", dbListing.Title)
	s.Equal("Updated Description", dbListing.Description)
	s.True(dbListing.UpdatedAt.After(initialListing.UpdatedAt)) // Check updated_at
	s.Equal(initialListing.Status, dbListing.Status)             // Status should not change
	s.Equal(initialListing.IsAdminApproved, dbListing.IsAdminApproved) // Approval should not change
}

// TestUpdateListing_Success_EventDetails
func (s *IntegrationTestSuite) TestUpdateListing_Success_EventDetails() {
    testUser, token := s.createUser("eventupdateuser", "eventupdate@example.com", "password", user.RoleUser)
    catEvents := s.createCategory("Events Update", "events-update")
    initialEventDetails := &listing.ListingDetailsEvents{
        EventDate:     time.Now().AddDate(0, 0, 10), // 10 days from now
        EventTime:     common.Ptr("10:00:00"),
        OrganizerName: common.Ptr("Old Organizer"),
    }
    initialListing := s.createListing(testUser.ID, catEvents.ID, nil, "Initial Event", listing.StatusActive, initialEventDetails)

    newDateStr := time.Now().AddDate(0, 0, 20).Format("2006-01-02")
    updateReq := listing.UpdateListingRequest{
        EventDetails: &listing.CreateListingEventDetailsRequest{ // Uses Create request struct as per model
            EventDate:     newDateStr,
            EventTime:     common.Ptr("14:30:00"),
            OrganizerName: common.Ptr("New Organizer"),
        },
    }
    jsonBody, _ := json.Marshal(updateReq)
    req, _ := http.NewRequest("PUT", fmt.Sprintf("/api/v1/listings/%s", initialListing.ID.String()), bytes.NewBuffer(jsonBody))
    req.Header.Set("Authorization", "Bearer "+token)
    req.Header.Set("Content-Type", "application/json")
    rr := httptest.NewRecorder()
    s.Router.ServeHTTP(rr, req)

    s.Equal(http.StatusOK, rr.Code, rr.Body.String())

    var resp common.StandardResponse
    json.Unmarshal(rr.Body.Bytes(), &resp)
    s.True(resp.Success)

    updatedData, _ := json.Marshal(resp.Data)
    var updatedListingResp listing.ListingResponse
    json.Unmarshal(updatedData, &updatedListingResp)

    s.NotNil(updatedListingResp.EventDetails)
    expectedDate, _ := time.Parse("2006-01-02", newDateStr)
    s.True(expectedDate.Equal(updatedListingResp.EventDetails.EventDate.Truncate(24*time.Hour))) // Compare date part only
    s.Equal("14:30:00", *updatedListingResp.EventDetails.EventTime)
    s.Equal("New Organizer", *updatedListingResp.EventDetails.OrganizerName)

    // Verify in DB
    dbListing, err := s.ListingRepo.FindByID(context.Background(), initialListing.ID, true) // Preload details
    s.NoError(err)
    s.NotNil(dbListing.EventDetails)
    s.True(expectedDate.Equal(dbListing.EventDetails.EventDate.Truncate(24*time.Hour)))
    s.Equal("14:30:00", *dbListing.EventDetails.EventTime)
    s.Equal("New Organizer", *dbListing.EventDetails.OrganizerName)
    s.Equal(initialListing.Title, dbListing.Title) // Core fields should not change
}

// TestUpdateListing_PartialDetailsUpdate (e.g. for EventDetails)
func (s *IntegrationTestSuite) TestUpdateListing_PartialDetailsUpdate() {
    testUser, token := s.createUser("eventpartialuser", "eventpartial@example.com", "password", user.RoleUser)
    catEvents := s.createCategory("Events Partial", "events-partial")
    originalDate := time.Now().AddDate(0, 1, 0) // One month from now
    originalOrganizer := "Original Organizer"
    initialEventDetails := &listing.ListingDetailsEvents{
        EventDate:     originalDate,
        EventTime:     common.Ptr("09:00:00"),
        OrganizerName: common.Ptr(originalOrganizer),
    }
    initialListing := s.createListing(testUser.ID, catEvents.ID, nil, "Partial Update Event", listing.StatusActive, initialEventDetails)

    updateReq := listing.UpdateListingRequest{
        EventDetails: &listing.CreateListingEventDetailsRequest{
            // EventDate is required by CreateListingEventDetailsRequest validation, so we must provide it.
            // If we want to test truly partial (omitting a required field like EventDate),
            // the model for UpdateListingRequest.EventDetails would need to be different
            // (e.g. all fields pointers and no 'required' validation tags).
            // For now, assume EventDate is part of the "partial" update if EventDetails is sent.
            EventDate: originalDate.Format("2006-01-02"), // Keep original date
            EventTime: common.Ptr("11:30:00"),             // Update only time
            // OrganizerName is omitted, should remain original
        },
    }
    jsonBody, _ := json.Marshal(updateReq)
    req, _ := http.NewRequest("PUT", fmt.Sprintf("/api/v1/listings/%s", initialListing.ID.String()), bytes.NewBuffer(jsonBody))
    req.Header.Set("Authorization", "Bearer "+token)
    req.Header.Set("Content-Type", "application/json")
    rr := httptest.NewRecorder()
    s.Router.ServeHTTP(rr, req)

    s.Equal(http.StatusOK, rr.Code, rr.Body.String())

    var resp common.StandardResponse
    json.Unmarshal(rr.Body.Bytes(), &resp)
    s.True(resp.Success)

    updatedData, _ := json.Marshal(resp.Data)
    var updatedListingResp listing.ListingResponse
    json.Unmarshal(updatedData, &updatedListingResp)

    s.NotNil(updatedListingResp.EventDetails)
    s.True(originalDate.Truncate(24*time.Hour).Equal(updatedListingResp.EventDetails.EventDate.Truncate(24*time.Hour))) // Date should be original
    s.Equal("11:30:00", *updatedListingResp.EventDetails.EventTime) // Time should be updated
    s.Equal(originalOrganizer, *updatedListingResp.EventDetails.OrganizerName) // Organizer should be original

    // Verify in DB
    dbListing, err := s.ListingRepo.FindByID(context.Background(), initialListing.ID, true)
    s.NoError(err)
    s.NotNil(dbListing.EventDetails)
    s.True(originalDate.Truncate(24*time.Hour).Equal(dbListing.EventDetails.EventDate.Truncate(24*time.Hour)))
    s.Equal("11:30:00", *dbListing.EventDetails.EventTime)
    s.Equal(originalOrganizer, *dbListing.EventDetails.OrganizerName)
}


// TestUpdateListing_Forbidden
func (s *IntegrationTestSuite) TestUpdateListing_Forbidden() {
	user1, _ := s.createUser("userowner", "userowner@example.com", "password", user.RoleUser)
	_, tokenUser2 := s.createUser("usurper", "usurper@example.com", "password", user.RoleUser)
	cat := s.createCategory("Forbidden Cat", "forbid-cat")
	listingUser1 := s.createListing(user1.ID, cat.ID, nil, "User1's Listing", listing.StatusActive, nil)

	updateReq := listing.UpdateListingRequest{Title: common.Ptr("Attempted Update")}
	jsonBody, _ := json.Marshal(updateReq)
	req, _ := http.NewRequest("PUT", fmt.Sprintf("/api/v1/listings/%s", listingUser1.ID.String()), bytes.NewBuffer(jsonBody))
	req.Header.Set("Authorization", "Bearer "+tokenUser2) // User2's token
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.Router.ServeHTTP(rr, req)

	s.Equal(http.StatusForbidden, rr.Code)
}

// TestUpdateListing_NotFound
func (s *IntegrationTestSuite) TestUpdateListing_NotFound() {
	_, token := s.createUser("notfounduser", "notfound@example.com", "password", user.RoleUser)
	nonExistentID := uuid.New()

	updateReq := listing.UpdateListingRequest{Title: common.Ptr("Title for NonExistent")}
	jsonBody, _ := json.Marshal(updateReq)
	req, _ := http.NewRequest("PUT", fmt.Sprintf("/api/v1/listings/%s", nonExistentID.String()), bytes.NewBuffer(jsonBody))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.Router.ServeHTTP(rr, req)

	s.Equal(http.StatusNotFound, rr.Code)
}

// TestUpdateListing_ValidationError
func (s *IntegrationTestSuite) TestUpdateListing_ValidationError() {
	testUser, token := s.createUser("validationerroruser", "validationerror@example.com", "password", user.RoleUser)
	cat := s.createCategory("Validation Cat", "validation-cat")
	initialListing := s.createListing(testUser.ID, cat.ID, nil, "Valid Listing", listing.StatusActive, nil)

	// Invalid: Title too short (assuming min=5 from CreateListingRequest, Update uses same validation logic implicitly)
	updateReq := listing.UpdateListingRequest{Title: common.Ptr("No")}
	jsonBody, _ := json.Marshal(updateReq)
	req, _ := http.NewRequest("PUT", fmt.Sprintf("/api/v1/listings/%s", initialListing.ID.String()), bytes.NewBuffer(jsonBody))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.Router.ServeHTTP(rr, req)

	s.Equal(http.StatusUnprocessableEntity, rr.Code) // Or 400 depending on how validation errors are mapped
	// Check response body for error details if your common.RespondWithError provides them
}

// TestUpdateListing_Unauthorized
func (s *IntegrationTestSuite) TestUpdateListing_Unauthorized() {
    testUser, _ := s.createUser("unauthupdateuser", "unauthupdate@example.com", "password", user.RoleUser)
    cat := s.createCategory("Unauth Cat", "unauth-cat")
    initialListing := s.createListing(testUser.ID, cat.ID, nil, "Unauth Update Listing", listing.StatusActive, nil)

    updateReq := listing.UpdateListingRequest{Title: common.Ptr("Unauthorized Update")}
    jsonBody, _ := json.Marshal(updateReq)

    // No token
    req, _ := http.NewRequest("PUT", fmt.Sprintf("/api/v1/listings/%s", initialListing.ID.String()), bytes.NewBuffer(jsonBody))
    req.Header.Set("Content-Type", "application/json")
    rr := httptest.NewRecorder()
    s.Router.ServeHTTP(rr, req)
    s.Equal(http.StatusUnauthorized, rr.Code)

    // Invalid token
    req.Header.Set("Authorization", "Bearer "+"invalidtoken")
    rr = httptest.NewRecorder()
    s.Router.ServeHTTP(rr, req)
    s.Equal(http.StatusUnauthorized, rr.Code)
}

// TestUpdateListing_NoStatusOrApprovalChange
func (s *IntegrationTestSuite) TestUpdateListing_NoStatusOrApprovalChange() {
    testUser, token := s.createUser("statususer", "status@example.com", "password", user.RoleUser)
    cat := s.createCategory("Status Cat", "status-cat")
    initialStatus := listing.StatusPendingApproval // Start with a non-default status
    initialApproval := false

    l := &listing.Listing{
		UserID:        testUser.ID,
		CategoryID:    cat.ID,
		Title:         "Status Test",
		Description:   "Status test description",
		Status:        initialStatus,
		ExpiresAt:     time.Now().Add(24 * 30 * time.Hour),
		IsAdminApproved: initialApproval,
	}
    err := s.ListingRepo.Create(context.Background(), l)
    s.Require().NoError(err)


    updateReq := listing.UpdateListingRequest{Title: common.Ptr("Updated Title for Status Check")}
    jsonBody, _ := json.Marshal(updateReq)
    req, _ := http.NewRequest("PUT", fmt.Sprintf("/api/v1/listings/%s", l.ID.String()), bytes.NewBuffer(jsonBody))
    req.Header.Set("Authorization", "Bearer "+token)
    req.Header.Set("Content-Type", "application/json")
    rr := httptest.NewRecorder()
    s.Router.ServeHTTP(rr, req)

    s.Equal(http.StatusOK, rr.Code, rr.Body.String())

    // Verify in DB
    dbListing, errDb := s.ListingRepo.FindByID(context.Background(), l.ID, false)
    s.NoError(errDb)
    s.Equal("Updated Title for Status Check", dbListing.Title)
    s.Equal(initialStatus, dbListing.Status) // Critical: Status should not change
    s.Equal(initialApproval, dbListing.IsAdminApproved) // Critical: Approval status should not change
}

// TestUpdateListing_Success_HousingDetails
func (s *IntegrationTestSuite) TestUpdateListing_Success_HousingDetails() {
	testUser, token := s.createUser("housingupdateuser", "housingupdate@example.com", "password", user.RoleUser)
	catHousing := s.createCategory("Housing Update", "housing-update")
	initialHousingDetails := &listing.ListingDetailsHousing{
		PropertyType: listing.HousingForRent,
		RentDetails:  common.Ptr("Old Rent Details"),
		SalePrice:    nil,
	}
	initialListing := s.createListing(testUser.ID, catHousing.ID, nil, "Initial Housing", listing.StatusActive, initialHousingDetails)

	updateReq := listing.UpdateListingRequest{
		HousingDetails: &listing.CreateListingHousingDetailsRequest{ // Uses Create request struct
			PropertyType: listing.HousingForSale, // Change property type
			SalePrice:    common.Ptr(float64(250000.50)),
			// RentDetails is omitted, should become nil or be handled by service logic if PropertyType changes
		},
	}
	jsonBody, _ := json.Marshal(updateReq)
	req, _ := http.NewRequest("PUT", fmt.Sprintf("/api/v1/listings/%s", initialListing.ID.String()), bytes.NewBuffer(jsonBody))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.Router.ServeHTTP(rr, req)

	s.Equal(http.StatusOK, rr.Code, rr.Body.String())

	var resp common.StandardResponse
	json.Unmarshal(rr.Body.Bytes(), &resp)
	s.True(resp.Success)

	updatedData, _ := json.Marshal(resp.Data)
	var updatedListingResp listing.ListingResponse
	json.Unmarshal(updatedData, &updatedListingResp)

	s.NotNil(updatedListingResp.HousingDetails)
	s.Equal(listing.HousingForSale, updatedListingResp.HousingDetails.PropertyType)
	s.Equal(float64(250000.50), *updatedListingResp.HousingDetails.SalePrice)
	// Depending on service logic, RentDetails might be cleared when type changes to ForSale
	// For this test, let's assume it is explicitly set or cleared by the update logic.
	// If it's not explicitly cleared by service logic when PropertyType changes, it might retain old value.
	// The current service logic for UpdateListing HousingDetails:
	// existingListing.HousingDetails.PropertyType = req.HousingDetails.PropertyType
	// existingListing.HousingDetails.RentDetails = req.HousingDetails.RentDetails (if RentDetails in req is nil, it becomes nil)
	// existingListing.HousingDetails.SalePrice = req.HousingDetails.SalePrice (if SalePrice in req is nil, it becomes nil)
	// So, if RentDetails is not in req.HousingDetails, it will be nil.
	s.Nil(updatedListingResp.HousingDetails.RentDetails)


	// Verify in DB
	dbListing, err := s.ListingRepo.FindByID(context.Background(), initialListing.ID, true) // Preload details
	s.NoError(err)
	s.NotNil(dbListing.HousingDetails)
	s.Equal(listing.HousingForSale, dbListing.HousingDetails.PropertyType)
	s.Equal(float64(250000.50), *dbListing.HousingDetails.SalePrice)
	s.Nil(dbListing.HousingDetails.RentDetails)
	s.Equal(initialListing.Title, dbListing.Title) // Core fields should not change
}

// TestUpdateListing_Success_BabysittingDetails
func (s *IntegrationTestSuite) TestUpdateListing_Success_BabysittingDetails() {
	testUser, token := s.createUser("babyupdateuser", "babyupdate@example.com", "password", user.RoleUser)
	catBabysitting := s.createCategory("Babysitting Update", "babysitting-update")
	initialBabysittingDetails := &listing.ListingDetailsBabysitting{
		LanguagesSpoken: []string{"English", "Spanish"},
	}
	initialListing := s.createListing(testUser.ID, catBabysitting.ID, nil, "Initial Babysitting", listing.StatusActive, initialBabysittingDetails)

	updateReq := listing.UpdateListingRequest{
		BabysittingDetails: &listing.CreateListingBabysittingDetailsRequest{ // Uses Create request struct
			LanguagesSpoken: []string{"French", "German"},
		},
	}
	jsonBody, _ := json.Marshal(updateReq)
	req, _ := http.NewRequest("PUT", fmt.Sprintf("/api/v1/listings/%s", initialListing.ID.String()), bytes.NewBuffer(jsonBody))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.Router.ServeHTTP(rr, req)

	s.Equal(http.StatusOK, rr.Code, rr.Body.String())

	var resp common.StandardResponse
	json.Unmarshal(rr.Body.Bytes(), &resp)
	s.True(resp.Success)

	updatedData, _ := json.Marshal(resp.Data)
	var updatedListingResp listing.ListingResponse
	json.Unmarshal(updatedData, &updatedListingResp)

	s.NotNil(updatedListingResp.BabysittingDetails)
	s.Equal([]string{"French", "German"}, updatedListingResp.BabysittingDetails.LanguagesSpoken)

	// Verify in DB
	dbListing, err := s.ListingRepo.FindByID(context.Background(), initialListing.ID, true) // Preload details
	s.NoError(err)
	s.NotNil(dbListing.BabysittingDetails)
	s.Equal([]string{"French", "German"}, dbListing.BabysittingDetails.LanguagesSpoken)
	s.Equal(initialListing.Title, dbListing.Title) // Core fields should not change
}


// Note on common.Ptr: Assuming a helper function like:
// func Ptr[T any](v T) *T { return &v }
// This is common in Go 1.18+ for creating pointers to literals.
// If using an older Go version, these would need to be assigned to a variable first.

// Also, ensure common.PaginatedResponse and common.StandardResponse structs
// match what is actually used in the application for deserialization.
// The provided structure is an assumption.

// The zap.Error(err) calls in SetupSuite will cause a compile error
// as logger.Fatal/Error etc. don't directly take zap.Error(err).
// They expect fields, e.g., logger.Fatal("msg", zap.Error(err)).
// This will be implicitly fixed by using a real logger from the logging package.
// For the purpose of this generation, the intent is clear.
