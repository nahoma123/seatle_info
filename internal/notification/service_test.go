package notification

import (
	"context"
	"errors"
	"seattle_info_backend/internal/common"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

// MockNotificationRepository is a mock type for notification.Repository
type MockNotificationRepository struct {
	mock.Mock
}

func (m *MockNotificationRepository) Create(ctx context.Context, notification *Notification) error {
	args := m.Called(ctx, notification)
	// Simulate ID generation if needed by the test (e.g., assign to notification.ID)
	if args.Error(0) == nil && notification.ID == uuid.Nil {
		notification.ID = uuid.New() // Simulate DB generating ID
	}
	return args.Error(0)
}

func (m *MockNotificationRepository) GetByUserID(ctx context.Context, userID uuid.UUID, page, pageSize int) ([]Notification, *common.Pagination, error) {
	args := m.Called(ctx, userID, page, pageSize)
	var notifications []Notification
	if args.Get(0) != nil {
		notifications = args.Get(0).([]Notification)
	}
	var pagination *common.Pagination
	if args.Get(1) != nil {
		pagination = args.Get(1).(*common.Pagination)
	}
	return notifications, pagination, args.Error(2)
}

func (m *MockNotificationRepository) FindByID(ctx context.Context, notificationID uuid.UUID, userID uuid.UUID) (*Notification, error) {
	args := m.Called(ctx, notificationID, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Notification), args.Error(1)
}

func (m *MockNotificationRepository) MarkAsRead(ctx context.Context, notificationID uuid.UUID, userID uuid.UUID) error {
	args := m.Called(ctx, notificationID, userID)
	return args.Error(0)
}

func (m *MockNotificationRepository) MarkAllAsRead(ctx context.Context, userID uuid.UUID) (int64, error) {
	args := m.Called(ctx, userID)
	return args.Get(0).(int64), args.Error(1)
}

// Test Suite Setup
type NotificationServiceTestSuite struct {
	service        Service // notification.Service (the one we are testing)
	mockNotifRepo  *MockNotificationRepository
	logger         *zap.Logger
}

func setupNotificationServiceTestSuite(t *testing.T) *NotificationServiceTestSuite {
	ts := &NotificationServiceTestSuite{}
	ts.mockNotifRepo = new(MockNotificationRepository)
	ts.logger = zap.NewNop()

	ts.service = NewService(
		ts.mockNotifRepo,
		ts.logger,
	)
	return ts
}

// --- Test Cases ---

func TestNotificationService_CreateNotification_Success(t *testing.T) {
	ts := setupNotificationServiceTestSuite(t)
	ctx := context.Background()
	userID := uuid.New()
	listingID := uuid.New()
	notifType := ListingApprovedLive
	message := "Your listing is live!"

	// Mock the repository's Create method
	// The mock will assign an ID to the notification object passed to it
	ts.mockNotifRepo.On("Create", ctx, mock.AnythingOfType("*notification.Notification")).Run(func(args mock.Arguments) {
		notifArg := args.Get(1).(*Notification)
		notifArg.ID = uuid.New() // Simulate DB generating an ID
		assert.Equal(t, userID, notifArg.UserID)
		assert.Equal(t, notifType, notifArg.Type)
		assert.Equal(t, message, notifArg.Message)
		assert.Equal(t, &listingID, notifArg.RelatedListingID)
		assert.False(t, notifArg.IsRead)
	}).Return(nil)

	createdNotif, err := ts.service.CreateNotification(ctx, userID, notifType, message, &listingID)

	assert.NoError(t, err)
	assert.NotNil(t, createdNotif)
	assert.NotEqual(t, uuid.Nil, createdNotif.ID, "Expected notification ID to be set") // Check that ID was set
	assert.Equal(t, userID, createdNotif.UserID)
	// ... other assertions ...
	ts.mockNotifRepo.AssertExpectations(t)
}

func TestNotificationService_CreateNotification_Error(t *testing.T) {
	ts := setupNotificationServiceTestSuite(t)
	ctx := context.Background()
	userID := uuid.New()
	listingID := uuid.New()
	expectedError := common.ErrInternalServer.WithDetails("Could not create notification.")


	ts.mockNotifRepo.On("Create", ctx, mock.AnythingOfType("*notification.Notification")).Return(errors.New("repo error"))

	createdNotif, err := ts.service.CreateNotification(ctx, userID, ListingCreatedLive, "test", &listingID)

	assert.Error(t, err)
	assert.Nil(t, createdNotif)
	apiErr, ok := err.(*common.APIError)
	assert.True(t, ok)
	assert.Equal(t, expectedError.Code, apiErr.Code)
	ts.mockNotifRepo.AssertExpectations(t)
}


func TestNotificationService_GetNotificationsForUser_Success(t *testing.T) {
	ts := setupNotificationServiceTestSuite(t)
	ctx := context.Background()
	userID := uuid.New()
	page, pageSize := 1, 5

	mockNotifications := []Notification{
		{ID: uuid.New(), UserID: userID, Message: "Notif 1"},
		{ID: uuid.New(), UserID: userID, Message: "Notif 2"},
	}
	mockPagination := &common.Pagination{CurrentPage: page, PageSize: pageSize, TotalItems: 2, TotalPages: 1}

	ts.mockNotifRepo.On("GetByUserID", ctx, userID, page, pageSize).Return(mockNotifications, mockPagination, nil)

	notifications, pagination, err := ts.service.GetNotificationsForUser(ctx, userID, page, pageSize)

	assert.NoError(t, err)
	assert.NotNil(t, notifications)
	assert.NotNil(t, pagination)
	assert.Len(t, notifications, 2)
	assert.Equal(t, mockPagination, pagination)
	ts.mockNotifRepo.AssertExpectations(t)
}

func TestNotificationService_GetNotificationsForUser_Error(t *testing.T) {
	ts := setupNotificationServiceTestSuite(t)
	ctx := context.Background()
	userID := uuid.New()
	page, pageSize := 1, 5
	expectedError := common.ErrInternalServer.WithDetails("Could not retrieve notifications.")

	ts.mockNotifRepo.On("GetByUserID", ctx, userID, page, pageSize).Return(nil, nil, errors.New("repo error"))

	notifications, pagination, err := ts.service.GetNotificationsForUser(ctx, userID, page, pageSize)

	assert.Error(t, err)
	assert.Nil(t, notifications)
	assert.Nil(t, pagination)
	apiErr, ok := err.(*common.APIError)
	assert.True(t, ok)
	assert.Equal(t, expectedError.Code, apiErr.Code)
	ts.mockNotifRepo.AssertExpectations(t)
}


func TestNotificationService_MarkNotificationAsRead_Success(t *testing.T) {
	ts := setupNotificationServiceTestSuite(t)
	ctx := context.Background()
	userID := uuid.New()
	notificationID := uuid.New()

	ts.mockNotifRepo.On("MarkAsRead", ctx, notificationID, userID).Return(nil)

	err := ts.service.MarkNotificationAsRead(ctx, notificationID, userID)

	assert.NoError(t, err)
	ts.mockNotifRepo.AssertExpectations(t)
}

func TestNotificationService_MarkNotificationAsRead_NotFound(t *testing.T) {
	ts := setupNotificationServiceTestSuite(t)
	ctx := context.Background()
	userID := uuid.New()
	notificationID := uuid.New()
	expectedError := common.ErrNotFound.WithDetails("Notification not found or not owned by user.") // Example detail from repo

	// Simulate the repository returning a common.ErrNotFound
	ts.mockNotifRepo.On("MarkAsRead", ctx, notificationID, userID).Return(expectedError)

	err := ts.service.MarkNotificationAsRead(ctx, notificationID, userID)

	assert.Error(t, err)
	apiErr, ok := err.(*common.APIError)
	assert.True(t, ok, "Error should be an APIError")
	assert.Equal(t, common.ErrNotFound.Code, apiErr.Code, "Error code should be NOT_FOUND")
	// Optionally check details if they are preserved and important
	// assert.Equal(t, expectedError.Details, apiErr.Details)
	ts.mockNotifRepo.AssertExpectations(t)
}


func TestNotificationService_MarkAllUserNotificationsAsRead_Success(t *testing.T) {
	ts := setupNotificationServiceTestSuite(t)
	ctx := context.Background()
	userID := uuid.New()
	expectedCount := int64(5)

	ts.mockNotifRepo.On("MarkAllAsRead", ctx, userID).Return(expectedCount, nil)

	count, err := ts.service.MarkAllUserNotificationsAsRead(ctx, userID)

	assert.NoError(t, err)
	assert.Equal(t, expectedCount, count)
	ts.mockNotifRepo.AssertExpectations(t)
}

func TestNotificationService_MarkAllUserNotificationsAsRead_Error(t *testing.T) {
	ts := setupNotificationServiceTestSuite(t)
	ctx := context.Background()
	userID := uuid.New()
	expectedError := common.ErrInternalServer.WithDetails("Could not mark all notifications as read.")


	ts.mockNotifRepo.On("MarkAllAsRead", ctx, userID).Return(int64(0), errors.New("repo error"))

	count, err := ts.service.MarkAllUserNotificationsAsRead(ctx, userID)

	assert.Error(t, err)
	assert.Equal(t, int64(0), count)
	apiErr, ok := err.(*common.APIError)
	assert.True(t, ok)
	assert.Equal(t, expectedError.Code, apiErr.Code)
	ts.mockNotifRepo.AssertExpectations(t)
}
