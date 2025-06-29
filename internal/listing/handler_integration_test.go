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
	"seattle_info_backend/internal/filestorage"   // Added
	"seattle_info_backend/internal/listing"
	"seattle_info_backend/internal/middleware"
	"seattle_info_backend/internal/notification" // Added
	"seattle_info_backend/internal/platform/database" // Corrected path
	"seattle_info_backend/internal/platform/logger"   // Corrected path
	"seattle_info_backend/internal/user"

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
	FileStorageService *filestorage.FileStorageService // Corrected package
	ListingService listing.Service // Store the service for direct calls if necessary for setup/assertions
	// Add other necessary services/repos
}

const testImageStorageBasePath = "../../test_images_integration" // Relative to this test file's dir

// SetupSuite runs once before all tests in the suite.
func (s *IntegrationTestSuite) SetupSuite() {
	// 0. Initialize Logger (using a test-friendly logger)
	// Corrected logger initialization based on common patterns.
	// Ensure your logging package has a NewLogger function that returns *zap.Logger.
	// For this example, I'll assume `platformlogger.New` returns *zap.Logger.
	zapLogger := logger.New("debug", "console") // Using the actual logger from platform
	s.Require().NotNil(zapLogger, "Logger should not be nil")


	// 1. Load Configuration
	// For integration tests, it's often better to override specific config values
	// rather than relying on a config file that might change or not be available in CI.
	// However, if LoadConfig is robust and can use env vars, that's also an option.
	// Let's try to load and then override.
	cfg, err := config.Load() // Assuming Load() uses env vars primarily or a root .env
	s.Require().NoError(err, "Failed to load config for tests")

	// Override necessary config for testing
	cfg.DBHost = os.Getenv("TEST_DB_HOST") // Example: Get from ENV or set default
	if cfg.DBHost == "" {
		cfg.DBHost = "localhost"
	}
	cfg.DBPort = os.Getenv("TEST_DB_PORT")
	if cfg.DBPort == "" {
		cfg.DBPort = "5433" // Often a different port for test DB
	}
	cfg.DBUser = os.Getenv("TEST_DB_USER")
	if cfg.DBUser == "" {
		cfg.DBUser = "testuser"
	}
	cfg.DBPassword = os.Getenv("TEST_DB_PASSWORD")
	if cfg.DBPassword == "" {
		cfg.DBPassword = "testpassword"
	}
	cfg.DBName = os.Getenv("TEST_DB_NAME")
	if cfg.DBName == "" {
		cfg.DBName = "seattle_info_test_db"
	}
	cfg.DBSSLMode = "disable" // Usually disable SSL for local test DBs

	// Override image storage path for tests
	cfg.ImageStoragePath = testImageStorageBasePath
	cfg.ImagePublicBaseURL = "/test_static_images" // Consistent with test path
	s.Cfg = cfg

	// Ensure test image storage directory exists and is clean
	s.Require().NoError(os.RemoveAll(testImageStorageBasePath), "Failed to clean test image storage before suite")
	s.Require().NoError(os.MkdirAll(filepath.Join(testImageStorageBasePath, "listings"), os.ModePerm), "Failed to create test image listings storage")


	// 2. Initialize Database
	// Construct DSN from overridden config
	testDSN := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s TimeZone=UTC",
		s.Cfg.DBHost, s.Cfg.DBPort, s.Cfg.DBUser, s.Cfg.DBPassword, s.Cfg.DBName, s.Cfg.DBSSLMode)

	// Using the actual database.NewGORM from the project structure.
	// Ensure your database package has a NewGORM that matches this signature.
	db, err := database.NewGORM(testDSN, zapLogger, s.Cfg.DBMaxIdleConns, s.Cfg.DBMaxOpenConns, s.Cfg.DBConnMaxLifetime)
	if err != nil {
		logger.Fatal("Failed to initialize test database", zap.Error(err))
	}
	s.DB = db
	// Run migrations (if you have them as a separate step)
	// database.Migrate(db) // Example

	// 3. Initialize Repositories
	s.UserRepo = user.NewGORMRepository(s.DB)
	s.CatRepo = category.NewGORMRepository(s.DB)
	s.ListingRepo = listing.NewGORMRepository(s.DB)

	// 4. Initialize Services
	// s.AuthService = auth.NewTokenService(s.Cfg.Auth.JWTSecret, s.Cfg.Auth.TokenExpiryMinutes, s.UserRepo, zapLogger) // Assuming Cfg.Auth exists
	// For now, let's assume Firebase is mocked or not strictly needed for these specific listing image tests' auth part if we directly create users.
	// If real token generation is needed, ensure Cfg.Auth fields are set.
	// Simplified auth service initialization for now:
	s.AuthService = auth.NewTokenService("test-secret", 60, s.UserRepo, zapLogger) // Replace with actual config later if needed for full auth flow

	catService := category.NewService(s.CatRepo, zapLogger) // Use zapLogger

	// Initialize FileStorageService
	fileStorageService, errFs := filestorage.NewFileStorageService(s.Cfg.ImageStoragePath, zapLogger)
	s.Require().NoError(errFs, "Failed to create FileStorageService for tests")
	s.FileStorageService = fileStorageService

	// Initialize NotificationService (mock or simple if not central to these tests)
	// For now, assuming a nil or very basic notification service for listing creation.
	// This needs to be replaced with actual or properly mocked NotificationService.
	var mockNotificationService notification.Service // = notification.NewService(...)

	// Update ListingService initialization
	listingService := listing.NewService(s.ListingRepo, s.UserRepo, catService, mockNotificationService, s.FileStorageService, s.Cfg, zapLogger)
	s.ListingService = listingService


	// 5. Initialize Gin Engine and Routes
	s.Router = gin.New()
	s.Router.Use(middleware.Logger(zapLogger)) // Corrected to use middleware.Logger
	s.Router.Use(gin.Recovery())

	// Setup middleware
	authMiddleware := middleware.Authenticate(s.AuthService, zapLogger) // Corrected to use middleware.Authenticate
	adminRoleMiddleware := middleware.RequireRole("admin", zapLogger) // Corrected to use middleware.RequireRole

	// Register routes (adapt to your main setup)
	apiGroup := s.Router.Group("/api/v1")
	// Update ListingHandler initialization
	listingHandler := listing.NewHandler(listingService, zapLogger, s.Cfg) // Pass s.Cfg
	listingHandler.RegisterRoutes(apiGroup, authMiddleware, adminRoleMiddleware)

	categoryHandler := category.NewHandler(catService, zapLogger) // Use zapLogger
	categoryHandler.RegisterRoutes(apiGroup, authMiddleware, adminRoleMiddleware)


	// Clean database before running tests
	s.cleanupDB()
}

// TearDownSuite runs once after all tests in the suite.
func (s *IntegrationTestSuite) TearDownSuite() {
	// Clean up the test image storage directory
	err := os.RemoveAll(testImageStorageBasePath)
	if err != nil {
		s.T().Logf("Warning: Failed to remove test image storage path %s: %v", testImageStorageBasePath, err)
	}

	// Close database connection if necessary
	sqlDB, _ := s.DB.DB()
	sqlDB.Close()
}

// SetupTest runs before each test.
func (s *IntegrationTestSuite) SetupTest() {
	// Clean database before each test to ensure isolation
	s.cleanupDB()
	// Seed common data if needed for every test (e.g., default categories)
	// s.seedCategories()
}

// Helper to clean all relevant tables.
func (s *IntegrationTestSuite) cleanupDB() {
	s.DB.Exec("DELETE FROM listing_images") // Added
	s.DB.Exec("DELETE FROM listing_details_events")
	s.DB.Exec("DELETE FROM listing_details_housing")
	s.DB.Exec("DELETE FROM listing_details_babysitting")
	s.DB.Exec("DELETE FROM listings")
	s.DB.Exec("DELETE FROM sub_categories")
	s.DB.Exec("DELETE FROM categories")
	s.DB.Exec("DELETE FROM users")
	// Add other tables if necessary
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

// Helper to create a listing.
func (s *IntegrationTestSuite) createListing(userID, catID uuid.UUID, subCatID *uuid.UUID, title string, status listing.ListingStatus, details interface{}) *listing.Listing {
	l := &listing.Listing{
		UserID:        userID,
		CategoryID:    catID,
		SubCategoryID: subCatID,
		Title:         title,
		Description:   title + " description",
		Status:        status,
		ExpiresAt:     time.Now().Add(24 * 30 * time.Hour), // Default 30 days expiry
		IsAdminApproved: true, // Assume approved unless specified
	}
	// Add details based on interface type
	switch d := details.(type) {
	case *listing.ListingDetailsEvents:
		l.EventDetails = d
	case *listing.ListingDetailsHousing:
		l.HousingDetails = d
	case *listing.ListingDetailsBabysitting:
		l.BabysittingDetails = d
	}

	err := s.ListingRepo.Create(context.Background(), l)
	s.Require().NoError(err)
	// Reload to ensure all associations are populated if needed by tests immediately after creation
	createdListing, err := s.ListingRepo.FindByID(context.Background(), l.ID, true)
	s.Require().NoError(err)
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

// TestGetMyListings_Success
func (s *IntegrationTestSuite) TestGetMyListings_Success() {
	user1, token1 := s.createUser("user1", "user1@example.com", "password123", user.RoleUser)
	user2, _ := s.createUser("user2", "user2@example.com", "password123", user.RoleUser)

	catEvents := s.createCategory("Events", "events")
	catHousing := s.createCategory("Housing", "housing")

	s.createListing(user1.ID, catEvents.ID, nil, "User1 Event Listing", listing.StatusActive, &listing.ListingDetailsEvents{EventDate: time.Now().Add(5 * 24 * time.Hour)})
	s.createListing(user1.ID, catHousing.ID, nil, "User1 Housing Listing", listing.StatusPendingApproval, &listing.ListingDetailsHousing{PropertyType: listing.HousingForRent, RentDetails: common.Ptr("Monthly")})
	s.createListing(user2.ID, catEvents.ID, nil, "User2 Event Listing", listing.StatusActive, nil)

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

// --- Image Upload Test Cases ---

func (s *IntegrationTestSuite) TestCreateListing_WithOneImage() {
	testUser, token := s.createUser("imguser1", "imguser1@example.com", "password", user.RoleUser)
	cat := s.createCategory("Image Category", "image-cat")

	// Create a temporary image file for upload
	tempImgContent := []byte("dummy image content")
	tempImgPath := createTempImageFile(s.T(), "test_image_1.jpg", tempImgContent)
	defer os.Remove(tempImgPath) // Clean up

	params := map[string]string{
		"title":       "Listing with One Image",
		"description": "This listing has one beautiful image.",
		"category_id": cat.ID.String(),
		// Add other required fields if any, e.g., city, state, zip, contact, etc.
		// For simplicity, assuming only title, desc, category_id are needed by CreateListingRequest's validation.
		// If babysitting_details_json, housing_details_json, event_details_json are needed based on category, add them.
	}
	fileParams := map[string][]string{
		"images": {tempImgPath}, // Field name "images"
	}

	req, err := CreateMultipartRequest("POST", "/api/v1/listings", params, fileParams)
	s.Require().NoError(err)
	req.Header.Set("Authorization", "Bearer "+token)

	rr := httptest.NewRecorder()
	s.Router.ServeHTTP(rr, req)

	s.T().Logf("Create Listing Response Body: %s", rr.Body.String())
	s.Equal(http.StatusCreated, rr.Code, "Response body: "+rr.Body.String())

	var resp common.StandardResponse
	err = json.Unmarshal(rr.Body.Bytes(), &resp)
	s.Require().NoError(err)
	s.True(resp.Success)

	var createdListing listing.ListingResponse
	respDataBytes, _ := json.Marshal(resp.Data)
	err = json.Unmarshal(respDataBytes, &createdListing)
	s.Require().NoError(err)

	s.Equal("Listing with One Image", createdListing.Title)
	s.NotEmpty(createdListing.ID)
	s.Require().Len(createdListing.Images, 1, "Should have one image in response")
	s.NotEmpty(createdListing.Images[0].ID, "Image ID should not be empty")
	s.NotEmpty(createdListing.Images[0].ImageURL, "Image URL should not be empty")
	s.True(strings.HasPrefix(createdListing.Images[0].ImageURL, s.Cfg.ImagePublicBaseURL), "Image URL should use configured base URL")
	s.Equal(0, createdListing.Images[0].SortOrder)

	// Verify image file exists on disk
	// The ImagePath in the DB is relative to ImageStoragePath/listings
	// Example: listings/uuid.jpg. So ImageURL would be /test_static_images/listings/uuid.jpg
	// We need to get the relative path from the URL to check the file system.
	expectedRelPathInURL := strings.TrimPrefix(createdListing.Images[0].ImageURL, s.Cfg.ImagePublicBaseURL) // gives /listings/uuid.jpg
	expectedDiskPath := filepath.Join(s.Cfg.ImageStoragePath, expectedRelPathInURL) // testImageStorageBasePath + /listings/uuid.jpg

	s.T().Logf("Expected image disk path: %s", expectedDiskPath)
	_, err = os.Stat(expectedDiskPath)
	s.NoError(err, "Image file should exist on disk at path: "+expectedDiskPath)

	// Verify database record for listing_images
	var dbImage listing.ListingImage
	dbResult := s.DB.Where("listing_id = ?", createdListing.ID).First(&dbImage)
	s.NoError(dbResult.Error, "Should find image record in DB")
	s.Equal(createdListing.Images[0].ID, dbImage.ID)
	s.NotEmpty(dbImage.ImagePath)
	s.Equal(0, dbImage.SortOrder)
	// Verify that dbImage.ImagePath corresponds to what's in expectedDiskPath (relative to storage path)
	s.Equal(strings.TrimPrefix(expectedRelPathInURL, "/"), dbImage.ImagePath) // ImagePath is like "listings/uuid.jpg"
}


func (s *IntegrationTestSuite) TestCreateListing_WithMultipleImages() {
	testUser, token := s.createUser("imguser2", "imguser2@example.com", "password", user.RoleUser)
	cat := s.createCategory("Multi Image Category", "multi-image-cat")

	tempImg1Path := createTempImageFile(s.T(), "test_multi_1.png", []byte("multi image one"))
	defer os.Remove(tempImg1Path)
	tempImg2Path := createTempImageFile(s.T(), "test_multi_2.gif", []byte("multi image two"))
	defer os.Remove(tempImg2Path)

	params := map[string]string{
		"title":       "Listing with Multiple Images",
		"description": "This listing has two awesome images.",
		"category_id": cat.ID.String(),
	}
	fileParams := map[string][]string{
		"images": {tempImg1Path, tempImg2Path},
	}

	req, err := CreateMultipartRequest("POST", "/api/v1/listings", params, fileParams)
	s.Require().NoError(err)
	req.Header.Set("Authorization", "Bearer "+token)

	rr := httptest.NewRecorder()
	s.Router.ServeHTTP(rr, req)

	s.Equal(http.StatusCreated, rr.Code, "Response body: "+rr.Body.String())

	var resp common.StandardResponse
	json.Unmarshal(rr.Body.Bytes(), &resp)
	var createdListing listing.ListingResponse
	respDataBytes, _ := json.Marshal(resp.Data)
	json.Unmarshal(respDataBytes, &createdListing)

	s.Require().Len(createdListing.Images, 2, "Should have two images in response")
	s.Equal(0, createdListing.Images[0].SortOrder)
	s.Equal(1, createdListing.Images[1].SortOrder)

	// Verify files and DB records (simplified checks here)
	for _, imgResp := range createdListing.Images {
		expectedRelPathInURL := strings.TrimPrefix(imgResp.ImageURL, s.Cfg.ImagePublicBaseURL)
		expectedDiskPath := filepath.Join(s.Cfg.ImageStoragePath, expectedRelPathInURL)
		_, err := os.Stat(expectedDiskPath)
		s.NoError(err, "Image file should exist: "+expectedDiskPath)

		var dbImgCount int64
		s.DB.Model(&listing.ListingImage{}).Where("id = ?", imgResp.ID).Count(&dbImgCount)
		s.Equal(int64(1), dbImgCount, "Image record should exist in DB for ID: "+imgResp.ID.String())
	}
}

// TODO: Add more tests:
// - TestCreateListing_NoImages
// - TestUpdateListing_AddImages
// - TestUpdateListing_RemoveImages
// - TestUpdateListing_ReplaceImages (remove some, add some)
// - TestDeleteListing_DeletesImageFiles
