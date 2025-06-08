package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"seattle_info_backend/internal/common"
	"seattle_info_backend/internal/listing" // Assuming ListingResponse is here
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

// TestGetUpcomingEventsAPI is an integration test for the GET /api/v1/events/upcoming endpoint.
func TestGetUpcomingEventsAPI(t *testing.T) {
	router, cleanup := setupTestServer(t) // Placeholder - use your actual test server setup
	defer cleanup()

	// Seed Categories
	catEvent, _ := seedCategoryIfNotExists(router, "Events", "events")
	catNonEvent, _ := seedCategoryIfNotExists(router, "Other", "other")

	// Seed User
	testUser, _ := seedUserIfNotExists(router, "upcomingeventsuser@test.com", "password123", "Upcoming", "User")

	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	// --- Seed Listings ---
	// 1. Future Event (should be returned, earliest)
	eventTime1 := time.Date(today.Year(), today.Month(), today.Day()+1, 10, 0, 0, 0, time.UTC) // Tomorrow 10:00 UTC
	listingEvent1 := listing.Listing{
		UserID:          testUser.ID,
		CategoryID:      catEvent.ID,
		Title:           "Future Event 1 (Tomorrow 10:00)",
		Status:          listing.StatusActive,
		IsAdminApproved: true,
		ExpiresAt:       eventTime1.Add(48 * time.Hour),
		EventDetails:    &listing.ListingDetailsEvents{EventDate: eventTime1, EventTime: ptrToString(eventTime1.Format("15:04:05"))},
		BaseModel:       common.BaseModel{ID: uuid.New(), CreatedAt: now.Add(-5 * time.Hour)},
	}
	seedListing(t, router, &listingEvent1)

	// 2. Future Event (should be returned, later today if "now" is before 14:00)
	eventTime2 := time.Date(today.Year(), today.Month(), today.Day(), 14, 0, 0, 0, time.UTC) // Today 14:00 UTC
	// Ensure this event is "upcoming" relative to 'now' for the test to be stable
	if now.After(eventTime2) { // If current time is past 14:00 UTC, schedule for tomorrow
		eventTime2 = eventTime2.Add(24 * time.Hour)
	}
	listingEvent2 := listing.Listing{
		UserID:          testUser.ID,
		CategoryID:      catEvent.ID,
		Title:           "Future Event 2 (Today/Tomorrow 14:00)",
		Status:          listing.StatusActive,
		IsAdminApproved: true,
		ExpiresAt:       eventTime2.Add(5 * time.Hour),
		EventDetails:    &listing.ListingDetailsEvents{EventDate: eventTime2, EventTime: ptrToString(eventTime2.Format("15:04:05"))},
		BaseModel:       common.BaseModel{ID: uuid.New(), CreatedAt: now.Add(-6 * time.Hour)},
	}
	seedListing(t, router, &listingEvent2)

	// 3. Future Event, different day (should be returned, latest)
	eventTime3 := time.Date(today.Year(), today.Month(), today.Day()+2, 9, 0, 0, 0, time.UTC) // Day after tomorrow 09:00 UTC
	listingEvent3 := listing.Listing{
		UserID:          testUser.ID,
		CategoryID:      catEvent.ID,
		Title:           "Future Event 3 (Day After Tomorrow 09:00)",
		Status:          listing.StatusActive,
		IsAdminApproved: true,
		ExpiresAt:       eventTime3.Add(5 * time.Hour),
		EventDetails:    &listing.ListingDetailsEvents{EventDate: eventTime3, EventTime: ptrToString(eventTime3.Format("15:04:05"))},
		BaseModel:       common.BaseModel{ID: uuid.New(), CreatedAt: now.Add(-7 * time.Hour)},
	}
	seedListing(t, router, &listingEvent3)


	// 4. Past Event (should NOT be returned)
	pastEventTime := now.Add(-24 * time.Hour)
	listingPastEvent := listing.Listing{
		UserID:          testUser.ID,
		CategoryID:      catEvent.ID,
		Title:           "Past Event",
		Status:          listing.StatusActive,
		IsAdminApproved: true,
		ExpiresAt:       now.Add(5 * time.Hour), // Still not expired overall
		EventDetails:    &listing.ListingDetailsEvents{EventDate: pastEventTime, EventTime: ptrToString("10:00:00")},
		BaseModel:       common.BaseModel{ID: uuid.New(), CreatedAt: now.Add(-48 * time.Hour)},
	}
	seedListing(t, router, &listingPastEvent)

	// 5. Non-Event Listing (should NOT be returned)
	listingNonEvent := listing.Listing{
		UserID:          testUser.ID,
		CategoryID:      catNonEvent.ID,
		Title:           "Not an Event",
		Status:          listing.StatusActive,
		IsAdminApproved: true,
		ExpiresAt:       now.Add(24 * time.Hour),
		BaseModel:       common.BaseModel{ID: uuid.New(), CreatedAt: now.Add(-1 * time.Hour)},
	}
	seedListing(t, router, &listingNonEvent)

	// 6. Expired Event (even if future event date, listing itself expired - should NOT be returned)
	expiredFutureEventTime := now.Add(72 * time.Hour)
	listingExpiredEvent := listing.Listing{
		UserID:          testUser.ID,
		CategoryID:      catEvent.ID,
		Title:           "Expired Future Event",
		Status:          listing.StatusExpired,
		IsAdminApproved: true,
		ExpiresAt:       now.Add(-1 * time.Hour), // Expired
		EventDetails:    &listing.ListingDetailsEvents{EventDate: expiredFutureEventTime, EventTime: ptrToString("10:00:00")},
		BaseModel:       common.BaseModel{ID: uuid.New(), CreatedAt: now.Add(-50 * time.Hour)},
	}
	seedListing(t, router, &listingExpiredEvent)

	// 7. Pending Event (should NOT be returned)
	pendingEventTime := now.Add(25 * time.Hour)
	listingPendingEvent := listing.Listing{
		UserID:          testUser.ID,
		CategoryID:      catEvent.ID,
		Title:           "Pending Future Event",
		Status:          listing.StatusPendingApproval,
		IsAdminApproved: false, // Not approved
		ExpiresAt:       pendingEventTime.Add(5 * time.Hour),
		EventDetails:    &listing.ListingDetailsEvents{EventDate: pendingEventTime, EventTime: ptrToString("11:00:00")},
		BaseModel:       common.BaseModel{ID: uuid.New(), CreatedAt: now.Add(-2 * time.Hour)},
	}
	seedListing(t, router, &listingPendingEvent)


	// Make Request
	rr := httptest.NewRecorder()
	// Default page size for /events/upcoming is 10
	req, _ := http.NewRequest("GET", "/api/v1/events/upcoming?page_size=5", nil) // Use smaller page_size for easier testing
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code, "Response code should be 200 OK")

	var responseBody struct {
		Status     string                    `json:"status"`
		Message    string                    `json:"message"`
		Data       []listing.ListingResponse `json:"data"`
		Pagination common.Pagination         `json:"pagination"`
	}
	err := json.Unmarshal(rr.Body.Bytes(), &responseBody)
	assert.NoError(t, err, "Failed to unmarshal response body")

	// Assertions
	assert.Equal(t, "success", responseBody.Status)
	// We seeded 3 valid upcoming events
	assert.Len(t, responseBody.Data, 3, "Expected 3 upcoming events based on seeding")
	assert.Equal(t, int64(3), responseBody.Pagination.TotalItems)
	assert.Equal(t, 1, responseBody.Pagination.TotalPages) // 3 items / 5 per page = 1 page
	assert.Equal(t, 5, responseBody.Pagination.PageSize)


	// Check order: eventTime2 (today/tomorrow 14:00), then eventTime1 (tomorrow 10:00), then eventTime3 (day after 09:00)
	// The order depends on exact value of 'now' vs eventTime2.
	// If eventTime2 was scheduled for today (now < 14:00 UTC):
	//   1. listingEvent2 (Today 14:00)
	//   2. listingEvent1 (Tomorrow 10:00)
	//   3. listingEvent3 (Day After Tomorrow 09:00)
	// If eventTime2 was scheduled for tomorrow (now >= 14:00 UTC):
	//   1. listingEvent1 (Tomorrow 10:00)
	//   2. listingEvent2 (Tomorrow 14:00)
	//   3. listingEvent3 (Day After Tomorrow 09:00)

	// For simplicity, let's check IDs are present and event details are populated.
	// Exact order testing here can be tricky without precise time control in seeding/query.
	foundEvent1 := false
	foundEvent2 := false
	foundEvent3 := false
	for _, ev := range responseBody.Data {
		assert.Equal(t, listing.StatusActive, ev.Status)
		assert.True(t, ev.IsAdminApproved)
		assert.NotNil(t, ev.EventDetails, "EventDetails should be populated")
		assert.Equal(t, catEvent.ID, ev.Category.ID, "Category should be 'Events'")
		if ev.ID == listingEvent1.ID { foundEvent1 = true }
		if ev.ID == listingEvent2.ID { foundEvent2 = true }
		if ev.ID == listingEvent3.ID { foundEvent3 = true }
	}
	assert.True(t, foundEvent1, "ListingEvent1 not found in response")
	assert.True(t, foundEvent2, "ListingEvent2 not found in response")
	assert.True(t, foundEvent3, "ListingEvent3 not found in response")

	// Check specific order if possible (this is simplified, assumes eventTime2 is before eventTime1 if on same effective day)
    // A more robust way is to parse EventDate and EventTime from response and compare.
    // For now, assuming the DB query's ORDER BY `event_date ASC, event_time ASC` works:
    // This check is illustrative and might need adjustment based on the actual eventTime2 value.
    if now.Before(time.Date(today.Year(), today.Month(), today.Day(), 14, 0, 0, 0, time.UTC)) { // eventTime2 is today
        assert.Equal(t, listingEvent2.ID, responseBody.Data[0].ID, "Event 2 should be first if it's today")
        assert.Equal(t, listingEvent1.ID, responseBody.Data[1].ID, "Event 1 should be second")
        assert.Equal(t, listingEvent3.ID, responseBody.Data[2].ID, "Event 3 should be third")
    } else { // eventTime2 is tomorrow
        assert.Equal(t, listingEvent1.ID, responseBody.Data[0].ID, "Event 1 should be first if Event 2 is also tomorrow")
        assert.Equal(t, listingEvent2.ID, responseBody.Data[1].ID, "Event 2 should be second")
        assert.Equal(t, listingEvent3.ID, responseBody.Data[2].ID, "Event 3 should be third")
    }


	// Clean up (placeholders)
	// clearTestData(...)
}

// ptrToString is a helper to get a pointer to a string. Useful for nullable string fields in structs.
func ptrToString(s string) *string {
	return &s
}
