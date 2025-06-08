package listing

import (
	"context"
	"errors"
	"seattle_info_backend/internal/category"
	"seattle_info_backend/internal/common"
	"seattle_info_backend/internal/config"
	"seattle_info_backend/internal/notification"
	"seattle_info_backend/internal/user"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

// MockListingRepository is a mock type for listing.Repository
type MockListingRepository struct {
	mock.Mock
}

func (m *MockListingRepository) Create(ctx context.Context, listing *Listing) error {
	args := m.Called(ctx, listing)
	return args.Error(0)
}

func (m *MockListingRepository) FindByID(ctx context.Context, id uuid.UUID, preloadAssociations bool) (*Listing, error) {
	args := m.Called(ctx, id, preloadAssociations)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Listing), args.Error(1)
}

func (m *MockListingRepository) Update(ctx context.Context, listing *Listing) error {
	args := m.Called(ctx, listing)
	return args.Error(0)
}

func (m *MockListingRepository) Delete(ctx context.Context, id uuid.UUID, userID uuid.UUID) error {
	args := m.Called(ctx, id, userID)
	return args.Error(0)
}

func (m *MockListingRepository) Search(ctx context.Context, query ListingSearchQuery) ([]Listing, *common.Pagination, error) {
	args := m.Called(ctx, query)
	if args.Get(0) == nil {
		return nil, nil, args.Error(2)
	}
	return args.Get(0).([]Listing), args.Get(1).(*common.Pagination), args.Error(2)
}

func (m *MockListingRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status ListingStatus, adminNotes *string) error {
	args := m.Called(ctx, id, status, adminNotes)
	return args.Error(0)
}

func (m *MockListingRepository) FindExpiredListings(ctx context.Context, now time.Time) ([]Listing, error) {
	args := m.Called(ctx, now)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]Listing), args.Error(1)
}

func (m *MockListingRepository) CountListingsByUserIDAndStatus(ctx context.Context, userID uuid.UUID, status ListingStatus) (int64, error) {
	args := m.Called(ctx, userID, status)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockListingRepository) CountListingsByUserID(ctx context.Context, userID uuid.UUID) (int64, error) {
	args := m.Called(ctx, userID)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockListingRepository) GetRecentListings(ctx context.Context, page, pageSize int, currentUserID *uuid.UUID) ([]Listing, *common.Pagination, error) {
	args := m.Called(ctx, page, pageSize, currentUserID)
	if args.Get(0) == nil {
		return nil, nil, args.Error(2)
	}
	var listings []Listing
	if args.Get(0) != nil {
		listings = args.Get(0).([]Listing)
	}
	var pagination *common.Pagination
	if args.Get(1) != nil {
		pagination = args.Get(1).(*common.Pagination)
	}
	return listings, pagination, args.Error(2)
}

func (m *MockListingRepository) GetUpcomingEvents(ctx context.Context, page, pageSize int) ([]Listing, *common.Pagination, error) {
	args := m.Called(ctx, page, pageSize)
		if args.Get(0) == nil {
		return nil, nil, args.Error(2)
	}
	var listings []Listing
	if args.Get(0) != nil {
		listings = args.Get(0).([]Listing)
	}
	var pagination *common.Pagination
	if args.Get(1) != nil {
		pagination = args.Get(1).(*common.Pagination)
	}
	return listings, pagination, args.Error(2)
}

// MockUserRepository is a mock type for user.Repository
type MockUserRepository struct {
	mock.Mock
}

func (m *MockUserRepository) Create(ctx context.Context, u *user.User) error {
	args := m.Called(ctx, u)
	return args.Error(0)
}
func (m *MockUserRepository) FindByEmail(ctx context.Context, email string) (*user.User, error) {
	args := m.Called(ctx, email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*user.User), args.Error(1)
}
func (m *MockUserRepository) FindByID(ctx context.Context, id uuid.UUID) (*user.User, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*user.User), args.Error(1)
}
func (m *MockUserRepository) Update(ctx context.Context, u *user.User) error {
	args := m.Called(ctx, u)
	return args.Error(0)
}
func (m *MockUserRepository) FindByProvider(ctx context.Context, authProvider string, providerID string) (*user.User, error) {
	args := m.Called(ctx, authProvider, providerID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*user.User), args.Error(1)
}
func (m *MockUserRepository) FindByFirebaseUID(ctx context.Context, firebaseUID string) (*user.User, error) {
	args := m.Called(ctx, firebaseUID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*user.User), args.Error(1)
}

// MockCategoryService is a mock type for category.Service
type MockCategoryService struct {
	mock.Mock
}

func (m *MockCategoryService) AdminCreateCategory(ctx context.Context, req category.AdminCreateCategoryRequest) (*category.Category, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*category.Category), args.Error(1)
}
func (m *MockCategoryService) AdminCreateSubCategory(ctx context.Context, categoryID uuid.UUID, req category.AdminCreateSubCategoryRequest) (*category.SubCategory, error) {
	args := m.Called(ctx, categoryID, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*category.SubCategory), args.Error(1)
}
func (m *MockCategoryService) AdminUpdateCategory(ctx context.Context, id uuid.UUID, req category.AdminCreateCategoryRequest) (*category.Category, error) {
	args := m.Called(ctx, id, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*category.Category), args.Error(1)
}
func (m *MockCategoryService) AdminUpdateSubCategory(ctx context.Context, id uuid.UUID, req category.AdminCreateSubCategoryRequest) (*category.SubCategory, error) {
	args := m.Called(ctx, id, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*category.SubCategory), args.Error(1)
}
func (m *MockCategoryService) AdminDeleteCategory(ctx context.Context, id uuid.UUID) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}
func (m *MockCategoryService) AdminDeleteSubCategory(ctx context.Context, id uuid.UUID) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}
func (m *MockCategoryService) GetCategoryByID(ctx context.Context, id uuid.UUID, preloadSubcategories bool) (*category.Category, error) {
	args := m.Called(ctx, id, preloadSubcategories)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*category.Category), args.Error(1)
}
func (m *MockCategoryService) GetCategoryBySlug(ctx context.Context, slug string, preloadSubcategories bool) (*category.Category, error) {
	args := m.Called(ctx, slug, preloadSubcategories)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*category.Category), args.Error(1)
}
func (m *MockCategoryService) GetAllCategories(ctx context.Context, preloadSubcategories bool) ([]category.Category, error) {
	args := m.Called(ctx, preloadSubcategories)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]category.Category), args.Error(1)
}
func (m *MockCategoryService) GetSubCategoryByID(ctx context.Context, id uuid.UUID) (*category.SubCategory, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*category.SubCategory), args.Error(1)
}

// MockNotificationService is a mock type for notification.Service
type MockNotificationService struct {
	mock.Mock
}

func (m *MockNotificationService) CreateNotification(ctx context.Context, userID uuid.UUID, notificationType notification.NotificationType, message string, relatedListingID *uuid.UUID) (*notification.Notification, error) {
	args := m.Called(ctx, userID, notificationType, message, relatedListingID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*notification.Notification), args.Error(1)
}
func (m *MockNotificationService) GetNotificationsForUser(ctx context.Context, userID uuid.UUID, page, pageSize int) ([]notification.Notification, *common.Pagination, error) {
	args := m.Called(ctx, userID, page, pageSize)
	if args.Get(0) == nil {
		return nil, nil, args.Error(2)
	}
	return args.Get(0).([]notification.Notification), args.Get(1).(*common.Pagination), args.Error(2)
}
func (m *MockNotificationService) MarkNotificationAsRead(ctx context.Context, notificationID uuid.UUID, userID uuid.UUID) error {
	args := m.Called(ctx, notificationID, userID)
	return args.Error(0)
}
func (m *MockNotificationService) MarkAllUserNotificationsAsRead(ctx context.Context, userID uuid.UUID) (int64, error) {
	args := m.Called(ctx, userID)
	return args.Get(0).(int64), args.Error(1)
}

// Test Suite Setup
type ListingServiceTestSuite struct {
	service             Service // listing.Service (the one we are testing)
	mockListingRepo     *MockListingRepository
	mockUserRepo        *MockUserRepository
	mockCategoryService *MockCategoryService
	mockNotifService    *MockNotificationService
	logger              *zap.Logger
	cfg                 *config.Config
}

func setupListingServiceTestSuite(t *testing.T) *ListingServiceTestSuite {
	ts := &ListingServiceTestSuite{}
	ts.mockListingRepo = new(MockListingRepository)
	ts.mockUserRepo = new(MockUserRepository)
	ts.mockCategoryService = new(MockCategoryService)
	ts.mockNotifService = new(MockNotificationService)
	ts.logger = zap.NewNop() // Use Nop logger for tests unless specific log output is needed
	ts.cfg = &config.Config{ // Basic config, adjust if specific values are needed for tests
		DefaultListingLifespanDays: 30,
	}

	// Initialize the service with mocks
	ts.service = NewService(
		ts.mockListingRepo,
		ts.mockUserRepo,
		ts.mockCategoryService,
		ts.mockNotifService, // Pass the mock notification service
		ts.cfg,
		ts.logger,
	)
	return ts
}

// --- Test Cases ---

func TestService_GetRecentListings_Success(t *testing.T) {
	ts := setupListingServiceTestSuite(t)
	ctx := context.Background()
	page, pageSize := 1, 3

	mockListings := []Listing{
		{BaseModel: common.BaseModel{ID: uuid.New()}, Title: "Recent 1", UserID: uuid.New(), CategoryID: uuid.New()},
		{BaseModel: common.BaseModel{ID: uuid.New()}, Title: "Recent 2", UserID: uuid.New(), CategoryID: uuid.New()},
	}
	mockPagination := &common.Pagination{CurrentPage: page, PageSize: pageSize, TotalItems: 2, TotalPages: 1}

	ts.mockListingRepo.On("GetRecentListings", ctx, page, pageSize, (*uuid.UUID)(nil)).Return(mockListings, mockPagination, nil)

	listings, pagination, err := ts.service.GetRecentListings(ctx, page, pageSize)

	assert.NoError(t, err)
	assert.NotNil(t, listings)
	assert.NotNil(t, pagination)
	assert.Len(t, listings, 2)
	assert.Equal(t, mockPagination, pagination)
	// Check that ToListingResponse was implicitly called with isAuthenticatedForContact = false
	// This can be checked by ensuring contact details are nil/empty in ListingResponse if they were present in Listing model
	for _, lr := range listings {
		assert.Nil(t, lr.ContactEmail) // Assuming ToListingResponse correctly nil-ifies this
		assert.Nil(t, lr.ContactPhone)
	}
	ts.mockListingRepo.AssertExpectations(t)
}

func TestService_GetRecentListings_Empty(t *testing.T) {
	ts := setupListingServiceTestSuite(t)
	ctx := context.Background()
	page, pageSize := 1, 3

	ts.mockListingRepo.On("GetRecentListings", ctx, page, pageSize, (*uuid.UUID)(nil)).Return([]Listing{}, &common.Pagination{TotalItems:0}, nil)

	listings, pagination, err := ts.service.GetRecentListings(ctx, page, pageSize)

	assert.NoError(t, err)
	assert.NotNil(t, listings)
	assert.NotNil(t, pagination)
	assert.Len(t, listings, 0)
	ts.mockListingRepo.AssertExpectations(t)
}

func TestService_GetRecentListings_Error(t *testing.T) {
	ts := setupListingServiceTestSuite(t)
	ctx := context.Background()
	page, pageSize := 1, 3
	expectedError := common.ErrInternalServer.WithDetails("Could not retrieve recent listings.")

	ts.mockListingRepo.On("GetRecentListings", ctx, page, pageSize, (*uuid.UUID)(nil)).Return(nil, nil, errors.New("repo error"))

	listings, pagination, err := ts.service.GetRecentListings(ctx, page, pageSize)

	assert.Error(t, err)
	assert.Nil(t, listings)
	assert.Nil(t, pagination)
	apiErr, ok := err.(*common.APIError)
	assert.True(t, ok)
	assert.Equal(t, expectedError.Code, apiErr.Code)
	assert.Equal(t, expectedError.Message, apiErr.Message) // Details might differ slightly based on internal error message
	ts.mockListingRepo.AssertExpectations(t)
}


func TestService_GetUpcomingEvents_Success(t *testing.T) {
	ts := setupListingServiceTestSuite(t)
	ctx := context.Background()
	page, pageSize := 1, 10

	mockEvents := []Listing{
		{BaseModel: common.BaseModel{ID: uuid.New()}, Title: "Event 1", UserID: uuid.New(), CategoryID: uuid.New(), EventDetails: &ListingDetailsEvents{EventDate: time.Now().Add(24 * time.Hour)}},
		{BaseModel: common.BaseModel{ID: uuid.New()}, Title: "Event 2", UserID: uuid.New(), CategoryID: uuid.New(), EventDetails: &ListingDetailsEvents{EventDate: time.Now().Add(48 * time.Hour)}},
	}
	mockPagination := &common.Pagination{CurrentPage: page, PageSize: pageSize, TotalItems: 2, TotalPages: 1}

	ts.mockListingRepo.On("GetUpcomingEvents", ctx, page, pageSize).Return(mockEvents, mockPagination, nil)

	events, pagination, err := ts.service.GetUpcomingEvents(ctx, page, pageSize)

	assert.NoError(t, err)
	assert.NotNil(t, events)
	assert.NotNil(t, pagination)
	assert.Len(t, events, 2)
	assert.Equal(t, mockPagination, pagination)
	for _, er := range events {
		assert.Nil(t, er.ContactEmail)
		assert.Nil(t, er.ContactPhone)
		assert.NotNil(t, er.EventDetails) // Ensure event details are present
	}
	ts.mockListingRepo.AssertExpectations(t)
}
// TODO: Add TestService_GetUpcomingEvents_Empty and TestService_GetUpcomingEvents_Error similar to GetRecentListings

// Further tests for CreateListing and AdminUpdateListingStatus notification aspects would go here
// For brevity, I'll outline one for CreateListing
func TestService_CreateListing_PendingApproval_Notification(t *testing.T) {
	ts := setupListingServiceTestSuite(t)
	ctx := context.Background()
	userID := uuid.New()
	listingID := uuid.New()
	catID := uuid.New()

	req := CreateListingRequest{
		Title:       "Test Pending Listing",
		Description: "Description",
		CategoryID:  catID,
		// ... other required fields for validation ...
	}

	mockUser := &user.User{BaseModel: common.BaseModel{ID: userID}, IsFirstPostApproved: false}
	// Assume FIRST_POST_APPROVAL_MODEL_ACTIVE_UNTIL is in the future
	ts.cfg.FirstPostApprovalActiveMonths = 1 // Make model active

	mockCategory := &category.Category{BaseModel: common.BaseModel{ID: catID}, Name: "Test Cat", Slug: "test-cat"}

	// Setup mocks
	ts.mockCategoryService.On("GetCategoryByID", ctx, catID, true).Return(mockCategory, nil)
	ts.mockUserRepo.On("FindByID", ctx, userID).Return(mockUser, nil)
	ts.mockListingRepo.On("CountListingsByUserID", ctx, userID).Return(int64(0), nil) // First post

	// Mock for repo.Create
	// We need to ensure the listing passed to Create has StatusPendingApproval
	ts.mockListingRepo.On("Create", ctx, mock.MatchedBy(func(l *Listing) bool {
		return l.UserID == userID && l.Title == req.Title && l.Status == StatusPendingApproval && !l.IsAdminApproved
	})).Run(func(args mock.Arguments) {
		listingArg := args.Get(1).(*Listing)
		listingArg.ID = listingID // Simulate ID generation by DB
		listingArg.CreatedAt = time.Now()
		listingArg.UpdatedAt = time.Now()
		listingArg.Status = StatusPendingApproval // Ensure status is set correctly
		listingArg.IsAdminApproved = false
	}).Return(nil)

	// Mock for repo.FindByID (reload)
	// The reloaded listing should reflect the state after creation (pending)
	reloadedListing := &Listing{
		BaseModel:       common.BaseModel{ID: listingID, CreatedAt: time.Now(), UpdatedAt: time.Now()},
		UserID:          userID,
		User:            *mockUser, // Make sure User is populated for notification
		Title:           req.Title,
		Status:          StatusPendingApproval,
		IsAdminApproved: false,
		CategoryID:      catID,
		Category:        *mockCategory,
	}
	ts.mockListingRepo.On("FindByID", ctx, listingID, true).Return(reloadedListing, nil)

	// Mock for notificationService.CreateNotification
	expectedNotifType := notification.ListingCreatedPendingApproval
	expectedNotifMsg := "Your listing 'Test Pending Listing' has been submitted and is pending review."
	ts.mockNotifService.On("CreateNotification", ctx, userID, expectedNotifType, expectedNotifMsg, &listingID).Return(&notification.Notification{}, nil)

	createdListing, err := ts.service.CreateListing(ctx, userID, req)

	assert.NoError(t, err)
	assert.NotNil(t, createdListing)
	assert.Equal(t, StatusPendingApproval, createdListing.Status)
	assert.False(t, createdListing.IsAdminApproved)

	ts.mockCategoryService.AssertExpectations(t)
	ts.mockUserRepo.AssertExpectations(t)
	ts.mockListingRepo.AssertExpectations(t)
	ts.mockNotifService.AssertExpectations(t) // Verify notification was called
}

// TODO: Add TestService_CreateListing_AutoApproved_Notification
// TODO: Add TestService_AdminUpdateListingStatus_Approved_Notification

// Example for AdminUpdateListingStatus - Notification Aspect
func TestService_AdminUpdateListingStatus_Approved_Notification(t *testing.T) {
    ts := setupListingServiceTestSuite(t)
    ctx := context.Background()
    listingID := uuid.New()
    userID := uuid.New()

    // Listing before update (pending)
    listingBefore := &Listing{
        BaseModel:       common.BaseModel{ID: listingID, CreatedAt: time.Now(), UpdatedAt: time.Now()},
        UserID:          userID,
        User:            user.User{BaseModel: common.BaseModel{ID: userID}, IsFirstPostApproved: false}, // User whose first post is this one
        Title:           "Test Listing for Approval",
        Status:          StatusPendingApproval,
        IsAdminApproved: false,
        CategoryID:      uuid.New(),
    }

    // Listing after update (active and approved)
    listingAfter := &Listing{
        BaseModel:       common.BaseModel{ID: listingID, CreatedAt: listingBefore.CreatedAt, UpdatedAt: time.Now()},
        UserID:          userID,
        User:            user.User{BaseModel: common.BaseModel{ID: userID}, IsFirstPostApproved: true}, // User now has first post approved
        Title:           "Test Listing for Approval",
        Status:          StatusActive,
        IsAdminApproved: true,
        CategoryID:      listingBefore.CategoryID,
    }

    // Mocks
    ts.mockListingRepo.On("FindByID", ctx, listingID, true).Return(listingBefore, nil).Once() // First call (before update)
    ts.mockUserRepo.On("FindByID", ctx, userID).Return(&listingBefore.User, nil) // For updating IsFirstPostApproved
    ts.mockUserRepo.On("Update", ctx, mock.AnythingOfType("*user.User")).Return(nil) // User update
    ts.mockListingRepo.On("UpdateStatus", ctx, listingID, StatusActive, (*string)(nil)).Return(nil)
    // Mock the explicit update to IsAdminApproved if that path is taken in your service
    // For simplicity, assume UpdateStatus might handle it or the subsequent FindByID reflects it.
    // If a separate repo.Update is called for IsAdminApproved, mock that too.
    ts.mockListingRepo.On("Update", ctx, mock.MatchedBy(func(l *Listing) bool {
        return l.ID == listingID && l.IsAdminApproved == true
    })).Return(nil).Maybe() // Maybe, because it's inside an if newStatus == StatusActive block


    ts.mockListingRepo.On("FindByID", ctx, listingID, true).Return(listingAfter, nil).Once() // Second call (after update, for reload)


    expectedNotifType := notification.ListingApprovedLive
    expectedNotifMsg := "Great news! Your listing 'Test Listing for Approval' has been approved and is now live."
    ts.mockNotifService.On("CreateNotification", ctx, userID, expectedNotifType, expectedNotifMsg, &listingID).Return(&notification.Notification{}, nil)

    updatedListing, err := ts.service.AdminUpdateListingStatus(ctx, listingID, StatusActive, nil)

    assert.NoError(t, err)
    assert.NotNil(t, updatedListing)
    assert.Equal(t, StatusActive, updatedListing.Status)
    assert.True(t, updatedListing.IsAdminApproved)

    ts.mockListingRepo.AssertExpectations(t)
    ts.mockUserRepo.AssertExpectations(t)
    ts.mockNotifService.AssertExpectations(t)
}
