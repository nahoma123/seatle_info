package notification

import (
	"context"
	"errors"
	"fmt"
	"seattle_info_backend/internal/common" // For Pagination

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Repository interface {
	Create(ctx context.Context, notification *Notification) error
	GetByUserID(ctx context.Context, userID uuid.UUID, page, pageSize int) ([]Notification, *common.Pagination, error)
	FindByID(ctx context.Context, notificationID uuid.UUID, userID uuid.UUID) (*Notification, error) // userID for ownership check
	MarkAsRead(ctx context.Context, notificationID uuid.UUID, userID uuid.UUID) error
	MarkAllAsRead(ctx context.Context, userID uuid.UUID) (int64, error) // Return count of marked notifications
}

// GORMRepository implements the Repository interface using GORM.
type GORMRepository struct {
	db *gorm.DB
}

// NewGORMRepository creates a new GORM notification repository.
func NewGORMRepository(db *gorm.DB) Repository {
	return &GORMRepository{db: db}
}

// Create inserts a new notification into the database.
func (r *GORMRepository) Create(ctx context.Context, notification *Notification) error {
	if err := r.db.WithContext(ctx).Create(notification).Error; err != nil {
		return fmt.Errorf("failed to create notification: %w", err)
	}
	return nil
}

// GetByUserID retrieves a paginated list of notifications for a specific user, ordered by creation date.
func (r *GORMRepository) GetByUserID(ctx context.Context, userID uuid.UUID, page, pageSize int) ([]Notification, *common.Pagination, error) {
	var notifications []Notification
	var total int64

	query := r.db.WithContext(ctx).Model(&Notification{}).Where("user_id = ?", userID)

	countQuery := r.db.WithContext(ctx).Model(&Notification{}).Where("user_id = ?", userID)
	if err := countQuery.Count(&total).Error; err != nil {
		return nil, nil, fmt.Errorf("counting notifications for user %s failed: %w", userID, err)
	}

	pagination := common.NewPagination(total, page, pageSize)

	offset := (page - 1) * pageSize
	if page <= 0 {
		offset = 0
	}

	err := query.Order("created_at DESC").
		Limit(pageSize).
		Offset(offset).
		Find(&notifications).Error
	if err != nil {
		return nil, nil, fmt.Errorf("fetching notifications for user %s failed: %w", userID, err)
	}
	return notifications, pagination, nil
}

// FindByID retrieves a specific notification by its ID, ensuring it belongs to the provided userID.
func (r *GORMRepository) FindByID(ctx context.Context, notificationID uuid.UUID, userID uuid.UUID) (*Notification, error) {
	var notification Notification
	err := r.db.WithContext(ctx).Where("id = ? AND user_id = ?", notificationID, userID).First(&notification).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, common.ErrNotFound.WithDetails("Notification not found or not owned by user.")
		}
		return nil, fmt.Errorf("failed to find notification %s for user %s: %w", notificationID, userID, err)
	}
	return &notification, nil
}

// MarkAsRead marks a specific notification as read for a user.
// It first verifies ownership using FindByID.
func (r *GORMRepository) MarkAsRead(ctx context.Context, notificationID uuid.UUID, userID uuid.UUID) error {
	_, err := r.FindByID(ctx, notificationID, userID)
	if err != nil {
		return err
	}

	result := r.db.WithContext(ctx).Model(&Notification{}).
		Where("id = ? AND user_id = ?", notificationID, userID).
		Update("is_read", true)

	if result.Error != nil {
		return fmt.Errorf("failed to mark notification %s as read for user %s: %w", notificationID, userID, result.Error)
	}
	if result.RowsAffected == 0 {
		return common.ErrNotFound.WithDetails("Notification not found, not owned by user, or already marked as read.")
	}
	return nil
}

// MarkAllAsRead marks all unread notifications for a user as read.
// It returns the count of notifications that were updated.
func (r *GORMRepository) MarkAllAsRead(ctx context.Context, userID uuid.UUID) (int64, error) {
	result := r.db.WithContext(ctx).Model(&Notification{}).
		Where("user_id = ? AND is_read = ?", userID, false).
		Updates(map[string]interface{}{"is_read": true})

	if result.Error != nil {
		return 0, fmt.Errorf("failed to mark all notifications as read for user %s: %w", userID, result.Error)
	}
	return result.RowsAffected, nil
}
