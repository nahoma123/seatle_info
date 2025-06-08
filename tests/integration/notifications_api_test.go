package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"seattle_info_backend/internal/auth" // For AuthResponse to get token
	"seattle_info_backend/internal/common"
	"seattle_info_backend/internal/listing" // For CreateListingRequest
	"seattle_info_backend/internal/notification"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

// TestNotificationsAPI covers the lifecycle of notifications:
// 1. Generation (indirectly via listing creation/approval)
// 2. Fetching notifications
// 3. Marking a notification as read
// 4. Marking all notifications as read
func TestNotificationsAPI_FullLifecycle(t *testing.T) {
	router, cleanup := setupTestServer(t) // Placeholder
	defer cleanup()

	// === User Setup ===
	userEmail := fmt.Sprintf("notifuser-%s@test.com", uuid.NewString()[:8])
	userPassword := "password123"
	// This seedUser should ideally register AND return an authenticated user (or token)
	// For now, assume it creates the user. Login will be a separate step.
	seededUser, _ := seedUserIfNotExists(router, userEmail, userPassword, "Notif", "TestUser")

	// Login the user to get a token
	loginReqBody := auth.LoginRequest{Email: userEmail, Password: userPassword} // Assuming this DTO exists
	loginBodyBytes, _ := json.Marshal(loginReqBody)

	// This part is tricky without a real Firebase Auth emulator.
	// The /auth/login endpoint in the original code was removed in favor of client-side Firebase auth.
	// For integration tests, you'd typically:
	// 1. Use Firebase Admin SDK to create a custom token for the test user.
	// 2. Exchange this custom token for an ID token using Firebase REST API.
	// OR:
	// 3. If you have a test-only endpoint that mints a valid JWT for a given UserID (bypass Firebase).
	// For this test, we'll assume a helper `getAuthTokenForUser(userID)` exists.
	// This is a MAJOR assumption for integration testing auth-dependent endpoints.
	authToken := getAuthTokenForUser(t, router, seededUser.ID) // Placeholder for getting a real token
	assert.NotEmpty(t, authToken, "Auth token should not be empty")


	// === Category Setup ===
	category, _ := seedCategoryIfNotExists(router, "Test Category for Notifs", "test-cat-notifs")

	// === 1. Test Notification Generation (Listing Created Pending Approval) ===
	createListingReq := listing.CreateListingRequest{
		Title:       "My Pending Listing for Notifications",
		Description: "This listing should generate a notification.",
		CategoryID:  category.ID,
		// Add other required fields for CreateListingRequest as per your model validation
		ContactName: ptrToString("Test Contact"),
		ContactEmail: ptrToString("testcontact@example.com"),
	}
	listingBodyBytes, _ := json.Marshal(createListingReq)

	rrCreate := httptest.NewRecorder()
	reqCreate, _ := http.NewRequest("POST", "/api/v1/listings", bytes.NewBuffer(listingBodyBytes))
	reqCreate.Header.Set("Authorization", "Bearer "+authToken)
	reqCreate.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rrCreate, reqCreate)

	assert.Equal(t, http.StatusCreated, rrCreate.Code, "Failed to create listing")
	var createdListingResp common.ResponseWrapper // Assuming common response structure
	json.Unmarshal(rrCreate.Body.Bytes(), &createdListingResp)

	// Extract listing ID from the response data
    // This depends on the actual structure of your common.ResponseWrapper and how ToListingResponse serializes
    var createdListingData listing.ListingResponse
    if dataBytes, err := json.Marshal(createdListingResp.Data); err == nil {
        json.Unmarshal(dataBytes, &createdListingData)
    }
    assert.NotEqual(t, uuid.Nil, createdListingData.ID, "Created listing ID should not be nil")

	// --- Verify "listing_created_pending_approval" notification ---
	time.Sleep(500 * time.Millisecond) // Give a moment for async notification if any (usually not needed for direct calls)

	rrGetNotifs1 := httptest.NewRecorder()
	reqGetNotifs1, _ := http.NewRequest("GET", "/api/v1/notifications?page_size=5", nil)
	reqGetNotifs1.Header.Set("Authorization", "Bearer "+authToken)
	router.ServeHTTP(rrGetNotifs1, reqGetNotifs1)

	assert.Equal(t, http.StatusOK, rrGetNotifs1.Code)
	var notifsResp1 struct {
		Data       []notification.Notification `json:"data"`
		Pagination common.Pagination           `json:"pagination"`
	}
	json.Unmarshal(rrGetNotifs1.Body.Bytes(), &notifsResp1)

	assert.NotEmpty(t, notifsResp1.Data, "Expected notifications for user")
	foundPendingNotif := false
	var pendingNotificationID uuid.UUID
	for _, n := range notifsResp1.Data {
		if n.Type == notification.ListingCreatedPendingApproval && n.RelatedListingID != nil && *n.RelatedListingID == createdListingData.ID {
			foundPendingNotif = true
			pendingNotificationID = n.ID
			assert.False(t, n.IsRead, "New notification should be unread")
			break
		}
	}
	assert.True(t, foundPendingNotif, "ListingCreatedPendingApproval notification not found")


	// === (Admin Action) Approve the listing - This step is complex for integration test ===
	// To do this properly, you'd need an admin user token and call the admin approval endpoint.
	// For simplicity here, we'll assume this happens and a notification is generated.
	// In a real test, you would:
	// 1. Get an admin token.
	// 2. Call `PATCH /api/v1/listings/admin/{listing_id}/status` or `POST /api/v1/listings/admin/{listing_id}/approve`.
	// For now, we'll skip the actual admin call and directly check for the next notification
	// This part would need to be fully implemented in a real test environment.
	t.Log("Skipping actual admin approval HTTP call. Assuming it triggers 'ListingApprovedLive' notification.")
	// Simulate that the listing service would call notificationService.CreateNotification for ListingApprovedLive
	// This is an indirect test. A direct way is to seed this notification if admin action is too complex to automate here.

	// --- Verify "listing_approved_live" notification (simulated) ---
	// To test this properly, you'd need to trigger the admin approval.
	// If testing generation is key, you might have a test helper that calls the notification service directly
	// or seed the notification.
	// For this test, we'll assume the listing service is correctly calling the notification service upon approval.
	// We'll look for it in the next fetch. (This part of the test is weak without actual admin action)


	// === 2. Test GET /api/v1/notifications (again, to see if new one appeared) ===
	// This is illustrative. A real admin approval would be needed.
	// Let's assume another notification (e.g., ListingApprovedLive) was generated for createdListingData.ID
	// For the sake of this test structure, we'll proceed as if it might be there.


	// === 3. Test POST /api/v1/notifications/{notification_id}/mark-read ===
	assert.NotEqual(t, uuid.Nil, pendingNotificationID, "Pending notification ID should have been captured")

	rrMarkRead := httptest.NewRecorder()
	reqMarkRead, _ := http.NewRequest("POST", fmt.Sprintf("/api/v1/notifications/%s/mark-read", pendingNotificationID), nil)
	reqMarkRead.Header.Set("Authorization", "Bearer "+authToken)
	router.ServeHTTP(rrMarkRead, reqMarkRead)

	assert.Equal(t, http.StatusOK, rrMarkRead.Code, "Failed to mark notification as read")

	// Verify it's marked as read
	rrGetNotifs2 := httptest.NewRecorder()
	reqGetNotifs2, _ := http.NewRequest("GET", "/api/v1/notifications?page_size=5", nil)
	reqGetNotifs2.Header.Set("Authorization", "Bearer "+authToken)
	router.ServeHTTP(rrGetNotifs2, reqGetNotifs2)

	var notifsResp2 struct { Data []notification.Notification `json:"data"` }
	json.Unmarshal(rrGetNotifs2.Body.Bytes(), &notifsResp2)

	foundMarkedNotif := false
	for _, n := range notifsResp2.Data {
		if n.ID == pendingNotificationID {
			assert.True(t, n.IsRead, "Notification should be marked as read")
			foundMarkedNotif = true
			break
		}
	}
	assert.True(t, foundMarkedNotif, "Previously marked notification not found in list")

	// === 4. Test POST /api/v1/notifications/mark-all-read ===
	// Seed another unread notification to test "mark-all-read"
	// This would typically be another action, but for test simplicity:
	// We can assume another notification might have been created (e.g. the ListingApprovedLive one)
	// Or, if not, this test might not change much if only one was read.
	// For a robust test, ensure multiple unread notifications exist before this step.

	rrMarkAll := httptest.NewRecorder()
	reqMarkAll, _ := http.NewRequest("POST", "/api/v1/notifications/mark-all-read", nil)
	reqMarkAll.Header.Set("Authorization", "Bearer "+authToken)
	router.ServeHTTP(rrMarkAll, reqMarkAll)

	assert.Equal(t, http.StatusOK, rrMarkAll.Code, "Failed to mark all notifications as read")

	// Verify all are read
	rrGetNotifs3 := httptest.NewRecorder()
	reqGetNotifs3, _ := http.NewRequest("GET", "/api/v1/notifications", nil)
	reqGetNotifs3.Header.Set("Authorization", "Bearer "+authToken)
	router.ServeHTTP(rrGetNotifs3, reqGetNotifs3)

	var notifsResp3 struct { Data []notification.Notification `json:"data"` }
	json.Unmarshal(rrGetNotifs3.Body.Bytes(), &notifsResp3)

	for _, n := range notifsResp3.Data {
		assert.True(t, n.IsRead, fmt.Sprintf("Notification ID %s should be marked as read after mark-all", n.ID))
	}
}


// getAuthTokenForUser is a placeholder for a function that would return a valid JWT
// for a given user ID in a test environment. This is non-trivial for Firebase unless
// using Admin SDK to mint custom tokens and then exchange them, or a test-only endpoint.
func getAuthTokenForUser(t *testing.T, router http.Handler, userID uuid.UUID) string {
	t.Logf("Placeholder: Returning mock token for user ID %s. Replace with real token generation for integration tests.", userID)
	// In a real scenario, this would involve:
	// 1. Using Firebase Admin SDK to create a custom token for `userID`.
	// 2. Using Firebase REST API (`signInWithCustomToken`) to exchange it for an ID token.
	// OR
	// 3. If you have a backdoor test endpoint (NOT FOR PRODUCTION) that issues a valid JWT for testing.
	// Example: return "mock-firebase-id-token-for-" + userID.String()
	// This mock token WILL NOT PASS actual Firebase token validation in your middleware.
	// Your AuthMiddleware needs to be adaptable for tests (e.g., allow a test flag to bypass Firebase verification
	// and instead use a pre-defined user from context, or use a Firebase emulator).

	// For now, returning a dummy token. The AuthMiddleware must be configured to handle this in tests.
	return "TEST_TOKEN_USER_" + userID.String()
}
