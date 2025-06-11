package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"seattle_info_backend/internal/common"
	"seattle_info_backend/internal/listing"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	// "github.com/stretchr/testify/suite" // If using testify suites later
)

// TestGetRecentListingsAPI is an integration test for the GET /listings/recent endpoint.
// It assumes a test server (TestServer) is available and configured,
// and helper functions like SeedListing, ClearListings, MakeRequest exist.
func TestGetRecentListingsAPI(t *testing.T) {
	// This is a placeholder for actual test server setup.
	// In a real scenario, you'd initialize your app, database, and router.
	// For now, we'll assume 's.router' is a *gin.Engine from a test suite or helper.
	// router := s.router // Example if using a suite 's'
	router, cleanup := setupTestServer(t) // Placeholder for your test server setup
	defer cleanup()                       // Placeholder for cleanup

	// 1. Seed Data
	//    - Active/approved non-event listings
	//    - Expired listings
	//    - Event listings
	//    - Pending listings

	// Helper to create category if it doesn't exist (simplified)
	catNonEvent, _ := seedCategoryIfNotExists(router, "Non-Event Category", "non-event")
	catEvent, _ := seedCategoryIfNotExists(router, "Events", "events")

	// User for listings
	testUser, _ := seedUserIfNotExists(router, "recentlistinguser@test.com", "password123", "Recent", "User")


	// Active, approved, non-event (should be returned)
	listing1Time := time.Now().Add(-1 * time.Hour)
	listing1 := listing.Listing{
		UserID:          testUser.ID,
		CategoryID:      catNonEvent.ID,
		Title:           "Recent Listing 1 (Non-Event)",
		Description:     "Desc 1",
		Status:          listing.StatusActive,
		IsAdminApproved: true,
		ExpiresAt:       time.Now().Add(24 * time.Hour),
		BaseModel:       common.BaseModel{ID: uuid.New(), CreatedAt: listing1Time, UpdatedAt: listing1Time},
	}
	seedListing(t, router, &listing1) // Placeholder for your seeding function

	listing2Time := time.Now().Add(-2 * time.Hour)
	listing2 := listing.Listing{
		UserID:          testUser.ID,
		CategoryID:      catNonEvent.ID,
		Title:           "Recent Listing 2 (Non-Event)",
		Description:     "Desc 2",
		Status:          listing.StatusActive,
		IsAdminApproved: true,
		ExpiresAt:       time.Now().Add(24 * time.Hour),
		BaseModel:       common.BaseModel{ID: uuid.New(), CreatedAt: listing2Time, UpdatedAt: listing2Time},
	}
	seedListing(t, router, &listing2)

	// Expired listing (should NOT be returned)
	listingExpired := listing.Listing{
		UserID:          testUser.ID,
		CategoryID:      catNonEvent.ID,
		Title:           "Expired Listing",
		Description:     "Desc expired",
		Status:          listing.StatusExpired, // or StatusActive but ExpiresAt in past
		IsAdminApproved: true,
		ExpiresAt:       time.Now().Add(-24 * time.Hour),
		BaseModel:       common.BaseModel{ID: uuid.New(), CreatedAt: time.Now().Add(-48 * time.Hour)},
	}
	seedListing(t, router, &listingExpired)

	// Event listing (should NOT be returned by /listings/recent)
	listingEvent := listing.Listing{
		UserID:          testUser.ID,
		CategoryID:      catEvent.ID, // Event category
		Title:           "Upcoming Event",
		Description:     "Desc event",
		Status:          listing.StatusActive,
		IsAdminApproved: true,
		ExpiresAt:       time.Now().Add(24 * time.Hour),
		EventDetails:    &listing.ListingDetailsEvents{EventDate: time.Now().Add(5 * 24 * time.Hour)},
		BaseModel:       common.BaseModel{ID: uuid.New(), CreatedAt: time.Now().Add(-3 * time.Hour)},
	}
	seedListing(t, router, &listingEvent)


	// Pending listing (should NOT be returned)
	listingPending := listing.Listing{
		UserID:          testUser.ID,
		CategoryID:      catNonEvent.ID,
		Title:           "Pending Listing",
		Description:     "Desc pending",
		Status:          listing.StatusPendingApproval,
		IsAdminApproved: false,
		ExpiresAt:       time.Now().Add(24 * time.Hour),
		BaseModel:       common.BaseModel{ID: uuid.New(), CreatedAt: time.Now().Add(-4 * time.Hour)},
	}
	seedListing(t, router, &listingPending)

	// Another active, approved, non-event (should be returned, but might be on page 2 depending on default page size)
	listing3Time := time.Now().Add(-30 * time.Minute) // Newest
	listing3 := listing.Listing{
		UserID:          testUser.ID,
		CategoryID:      catNonEvent.ID,
		Title:           "Recent Listing 3 (Non-Event, Newest)",
		Description:     "Desc 3",
		Status:          listing.StatusActive,
		IsAdminApproved: true,
		ExpiresAt:       time.Now().Add(24 * time.Hour),
		BaseModel:       common.BaseModel{ID: uuid.New(), CreatedAt: listing3Time, UpdatedAt: listing3Time},
	}
	seedListing(t, router, &listing3)

	listing4Time := time.Now().Add(-5 * time.Hour) // Oldest of the recents
	listing4 := listing.Listing{
		UserID:          testUser.ID,
		CategoryID:      catNonEvent.ID,
		Title:           "Recent Listing 4 (Non-Event, Oldest)",
		Description:     "Desc 4",
		Status:          listing.StatusActive,
		IsAdminApproved: true,
		ExpiresAt:       time.Now().Add(24 * time.Hour),
		BaseModel:       common.BaseModel{ID: uuid.New(), CreatedAt: listing4Time, UpdatedAt: listing4Time},
	}
	seedListing(t, router, &listing4)


	// 2. Make Request (default page, default page_size = 3 for this endpoint)
	rr := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/listings/recent", nil)
	router.ServeHTTP(rr, req) // Use the test router

	assert.Equal(t, http.StatusOK, rr.Code)

	var responseBody struct {
		Status  string                    `json:"status"`
		Message string                    `json:"message"`
		Data    []listing.ListingResponse `json:"data"`
		Pagination common.Pagination       `json:"pagination"`
	}
	err := json.Unmarshal(rr.Body.Bytes(), &responseBody)
	assert.NoError(t, err)

	// 3. Assertions
	assert.Equal(t, "success", responseBody.Status)
	assert.Len(t, responseBody.Data, 3, "Expected 3 listings on page 1 (default page_size for /recent is 3)")
	assert.Equal(t, 1, responseBody.Pagination.CurrentPage)
	assert.Equal(t, 3, responseBody.Pagination.PageSize)
	assert.Equal(t, int64(4), responseBody.Pagination.TotalItems, "Expected total of 4 recent, active, non-event listings") // listing1, listing2, listing3, listing4
	assert.Equal(t, 2, responseBody.Pagination.TotalPages) // 4 items / 3 per page = 2 pages

	// Check order (newest first) and content
	assert.Equal(t, listing3.ID, responseBody.Data[0].ID) // listing3 is newest
	assert.Equal(t, listing1.ID, responseBody.Data[1].ID)
	assert.Equal(t, listing2.ID, responseBody.Data[2].ID)

	for _, l := range responseBody.Data {
		assert.Equal(t, listing.StatusActive, l.Status)
		assert.True(t, l.IsAdminApproved)
		assert.NotEqual(t, "events", l.Category.Slug, "Event category should be excluded")
		assert.Nil(t, l.ContactEmail, "Contact details should be hidden for public recent listings")
	}

	// Test with page_size query param
	rrPageSize := httptest.NewRecorder()
	reqPageSize, _ := http.NewRequest("GET", "/api/v1/listings/recent?page_size=2", nil)
	router.ServeHTTP(rrPageSize, reqPageSize)
	assert.Equal(t, http.StatusOK, rrPageSize.Code)
	var responseBodyPageSize struct {
		Data    []listing.ListingResponse `json:"data"`
		Pagination common.Pagination `json:"pagination"`
	}
	json.Unmarshal(rrPageSize.Body.Bytes(), &responseBodyPageSize)
	assert.Len(t, responseBodyPageSize.Data, 2)
	assert.Equal(t, 2, responseBodyPageSize.Pagination.PageSize)
	assert.Equal(t, int64(4), responseBodyPageSize.Pagination.TotalItems)
	assert.Equal(t, 2, responseBodyPageSize.Pagination.TotalPages) // 4 items / 2 per page = 2 pages
	assert.Equal(t, listing3.ID, responseBodyPageSize.Data[0].ID)
	assert.Equal(t, listing1.ID, responseBodyPageSize.Data[1].ID)


	// Test page 2
	rrPage2 := httptest.NewRecorder()
	reqPage2, _ := http.NewRequest("GET", "/api/v1/listings/recent?page=2&page_size=3", nil)
	router.ServeHTTP(rrPage2, reqPage2)
	assert.Equal(t, http.StatusOK, rrPage2.Code)
	var responseBodyPage2 struct {
		Data    []listing.ListingResponse `json:"data"`
		Pagination common.Pagination `json:"pagination"`
	}
	json.Unmarshal(rrPage2.Body.Bytes(), &responseBodyPage2)
	assert.Len(t, responseBodyPage2.Data, 1) // Remaining 1 item on page 2
	assert.Equal(t, 2, responseBodyPage2.Pagination.CurrentPage)
	assert.Equal(t, listing4.ID, responseBodyPage2.Data[0].ID)

	// Clean up (if using a persistent test DB, otherwise often handled by test suite teardown)
	// clearTestData(t, router, []uuid.UUID{listing1.ID, listing2.ID, listingExpired.ID, listingEvent.ID, listingPending.ID, listing3.ID, listing4.ID}, testUser.ID, catNonEvent.ID, catEvent.ID)
}

func TestCreateListingAPI_WithLocation(t *testing.T) {
	router, cleanup := setupTestServer(t)
	defer cleanup()

	// 1. Seed User and Category
	testUser, err := seedUserIfNotExists(router, "createlistinguser@test.com", "password123", "Create", "User")
	assert.NoError(t, err)
	assert.NotNil(t, testUser)

	categoryData, err := seedCategoryIfNotExists(router, "Test Category for Location Listing", "test-cat-location")
	assert.NoError(t, err)
	assert.NotNil(t, categoryData)

	// 2. Construct Listing Payload
	lat := 47.6062
	lon := -122.3321
	title := "Listing with Location Data"
	description := "This is a test listing that includes latitude and longitude."

	createReq := listing.CreateListingRequest{
		CategoryID:  categoryData.ID,
		Title:       title,
		Description: description,
		Latitude:    &lat,
		Longitude:   &lon,
		// Other required fields if any (e.g., ContactName, ExpiresAt - assuming defaults or handled by API)
		// For simplicity, we're focusing on location. Add other fields as per your model's requirements.
	}

	payloadBytes, err := json.Marshal(createReq)
	assert.NoError(t, err)
	payloadBody := bytes.NewReader(payloadBytes)

	// 3. Make Authenticated POST Request
	rr := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/listings", payloadBody)

	// --- Authentication Placeholder ---
	// In a real test, you would get a token for testUser and set it in the header.
	// Example:
	// token := getAuthToken(router, testUser.Email, "password123") // Assuming getAuthToken helper
	// req.Header.Set("Authorization", "Bearer "+token)
	// For now, we proceed without explicit auth, but note it.
	// Depending on your API setup, this might result in 401 if auth is strictly enforced.
	// If your setupTestServer or specific test user seeding handles auth implicitly, this comment can be adjusted.
	req.Header.Set("Content-Type", "application/json")
	// Simulate a user ID in context if your setupTestServer doesn't handle full auth middleware for tests
	// This is a common workaround if full auth flow is too complex for basic integration tests
	// req = req.WithContext(context.WithValue(req.Context(), "userID", testUser.ID)) // Example

	router.ServeHTTP(rr, req)

	// 4. Assert HTTP Status
	// If auth is missing and required, this might fail. Adjust expected status if necessary.
	// For this test, we'll assume the endpoint is reachable and creates the listing.
	assert.Equal(t, http.StatusCreated, rr.Code, "Expected status 201 Created. Body: %s", rr.Body.String())

	// 5. Unmarshal and Assert Response Data
	if rr.Code == http.StatusCreated {
		var responseBody struct {
			Status  string                    `json:"status"`
			Message string                    `json:"message"`
			Data    listing.ListingResponse   `json:"data"`
		}
		err = json.Unmarshal(rr.Body.Bytes(), &responseBody)
		assert.NoError(t, err, "Failed to unmarshal response body")

		assert.Equal(t, "success", responseBody.Status)
		assert.NotEmpty(t, responseBody.Data.ID, "Listing ID should not be empty")
		assert.Equal(t, title, responseBody.Data.Title)

		// Assert Location (PostGISPoint)
		assert.NotNil(t, responseBody.Data.Location, "Location (PostGISPoint) should not be nil")
		if responseBody.Data.Location != nil {
			assert.InDelta(t, lat, responseBody.Data.Location.Lat, 0.00001, "Latitude in Location does not match")
			assert.InDelta(t, lon, responseBody.Data.Location.Lon, 0.00001, "Longitude in Location does not match")
		}

		// Assert Latitude and Longitude (*float64 fields)
		assert.NotNil(t, responseBody.Data.Latitude, "Latitude (*float64) should not be nil")
		assert.NotNil(t, responseBody.Data.Longitude, "Longitude (*float64) should not be nil")
		if responseBody.Data.Latitude != nil && responseBody.Data.Longitude != nil {
			assert.InDelta(t, lat, *responseBody.Data.Latitude, 0.00001, "Latitude (*float64) does not match")
			assert.InDelta(t, lon, *responseBody.Data.Longitude, 0.00001, "Longitude (*float64) does not match")
		}

		// Optionally, clean up the created listing
		// clearTestData(t, router, []uuid.UUID{responseBody.Data.ID}, testUser.ID, categoryData.ID)
	}
}

// --- Placeholder Helper Functions ---
// These would be part of your actual integration test setup

// var testServer *app.Server // Or however your test server is structured
// var testDB *gorm.DB

// setupTestServer initializes the server for tests.
// This is a simplified placeholder. Your actual setup might involve:
// - Loading test configuration.
// - Setting up a test database (e.g., Docker container, in-memory SQLite).
// - Running migrations.
// - Initializing all services and handlers (potentially using Wire for test environment).
// - Returning the Gin router and a cleanup function.
func setupTestServer(t *testing.T) (*http.Handler, func()) {
	// For now, this is a very basic placeholder.
	// It should return a configured gin.Engine or http.Handler for the test.
	// And a cleanup function to run after the test (e.g., close DB connections).
	// This would typically involve initializing your main application's components
	// with test-specific configurations (e.g., a test database).

	// Example of what you might do:
	// cfg := config.LoadConfigForTest() // Load a test-specific config
	// logger := zap.NewNop()
	// db := database.SetupTestDB(cfg) // Setup test DB, run migrations
	// serverInstance, _, _ := InitializeTestServer(cfg, logger, db) // Your DI for test
	// router := serverInstance.Router() // Assuming Router() gives *gin.Engine

	// This is highly dependent on your project structure.
	// For this example, we'll just return a nil handler and no-op cleanup.
	// You will need to replace this with your actual test server initialization logic.

	// A simple gin router for placeholder
	gin.SetMode(gin.TestMode)
    r := gin.New()
    // You would register your actual routes here, e.g., from app.NewServer()
    // For now, this example won't have real routes unless you copy server setup here.
    // This is insufficient for real integration tests without proper DI and route setup.
	// The tests above will fail if the router doesn't have the /api/v1/listings/recent path.
	// You need to ensure your test server setup correctly wires everything.
	// This function should ideally call your project's main DI (InitializeServer from wire_gen.go)
	// with a test config and test DB.

	// Placeholder:
	// cfg, _ := config.Load(".env.test") // Assuming you have a test env file
	// logger, _ := zap.NewDevelopment()
	// db, _ := database.NewGORM(cfg) // Connects to test DB specified in .env.test
	// server, _, _ := main.InitializeServer(cfg) // Using actual DI
	// return server.Router(), func() { database.CloseGORMDB(db); }

	// For now, to make the test runnable in isolation of full DI:
	// We'd have to manually set up a minimal set of dependencies for listingHandler.
	// This is not ideal for true integration tests but can work for focused tests if DI is complex.
	// However, the goal of integration tests is to test the integrated system.
	// So, using the DI from wire_gen.go is preferred.

	// This function needs to be properly implemented for the tests to run.
	// For this subtask, I'm providing the test structure. Actual setup is outside this immediate scope
	// but crucial for running them.

	// A minimal router that might allow the test to run if handlers are manually set up (not shown)
	// r := gin.Default()
	// This is a placeholder and will not work without actual route registration from your app.
	// To properly test, the router must be the one from your app.Server instance,
    // initialized with all dependencies (DB, services, handlers).

	t.Log("Warning: setupTestServer is a placeholder. Real integration tests require proper server and DB setup.")
	return r, func() { t.Log("Test cleanup placeholder.") }
}


// seedCategoryIfNotExists (placeholder)
func seedCategoryIfNotExists(router http.Handler, name, slug string) (*category.Category, error) {
	// In a real test:
	// 1. Check if category with slug exists via an API call or direct DB query
	// 2. If not, create it via API call (POST /categories) or direct DB insertion
	// 3. Return the category
	t := &testing.T{} // Hack to get testing.T for Logf
	t.Logf("Placeholder: Seeding category %s (%s)", name, slug)
	// This needs to be implemented using your actual API or DB access for tests
	return &category.Category{BaseModel: common.BaseModel{ID: uuid.New()}, Name: name, Slug: slug}, nil
}

// seedUserIfNotExists (placeholder)
func seedUserIfNotExists(router http.Handler, email, password, firstName, lastName string) (*user.UserResponse, error) {
	t := &testing.T{}
	t.Logf("Placeholder: Seeding user %s", email)
	// This needs to be implemented. Typically involves:
	// 1. Calling a registration endpoint (if exists and usable for tests) OR direct DB insert.
	// 2. Returning the created user details.
	// For tests requiring auth, you'd also need to "log in" this user to get a token.
	return &user.UserResponse{ID: uuid.New(), Email: email, FirstName: &firstName}, nil
}


// seedListing (placeholder)
func seedListing(t *testing.T, router http.Handler, l *listing.Listing) {
	// In a real test:
	// - Use a test HTTP client to make a POST request to create the listing, OR
	// - Insert directly into the test database.
	// This requires l to have its CreatedAt, UpdatedAt, ExpiresAt, etc. set if not handled by DB.
	// The ID should also be set if not auto-generated or if you need to know it.
	t.Logf("Placeholder: Seeding listing: %s (ID: %s, CreatedAt: %s)", l.Title, l.ID, l.CreatedAt)
	// This function needs to be properly implemented.
}

// clearTestData (placeholder)
func clearTestData(t *testing.T, router http.Handler, listingIDs []uuid.UUID, userID uuid.UUID, catIDs ...uuid.UUID) {
    t.Logf("Placeholder: Clearing test data.")
    // This would involve:
    // - Deleting listings by ID via API or DB.
    // - Deleting the user by ID via API or DB.
    // - Deleting categories by ID via API or DB.
}

// Note: The placeholder functions (setupTestServer, seed*, clear*) need to be
// implemented based on your project's specific testing infrastructure.
// The integration tests rely on these helpers to interact with a test instance
// of your application.
