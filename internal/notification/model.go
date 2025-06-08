package notification

import (
	"time"

	"github.com/google/uuid"
	// "seattle_info_backend/internal/common" // Assuming common.BaseModel exists and provides ID, CreatedAt, UpdatedAt
)

// NotificationType defines the type of notification.
type NotificationType string

const (
	ListingCreatedPendingApproval NotificationType = "listing_created_pending_approval"
	ListingCreatedLive            NotificationType = "listing_created_live"
	ListingApprovedLive           NotificationType = "listing_approved_live"
	// ListingRejected             NotificationType = "listing_rejected" // Future
)

// Notification represents a user notification.
type Notification struct {
	ID                 uuid.UUID        `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id"`
	UserID             uuid.UUID        `gorm:"type:uuid;not null;index:idx_notification_user_status" json:"user_id"` // User who receives it
	Type               NotificationType `gorm:"type:varchar(100);not null" json:"type"`
	Message            string           `gorm:"type:text;not null" json:"message"`
	RelatedListingID   *uuid.UUID       `gorm:"type:uuid" json:"related_listing_id,omitempty"` // Nullable
	IsRead             bool             `gorm:"not null;default:false;index:idx_notification_user_status" json:"is_read"`
	CreatedAt          time.Time        `gorm:"not null;default:CURRENT_TIMESTAMP;index:idx_notification_user_status" json:"created_at"`
	// Removed UpdatedAt as notifications are typically immutable once created. If edits are needed, add it back.

	// Associations (optional, depending on query needs)
	// User User `gorm:"foreignKey:UserID" json:"-"` // For eager loading user info if needed
	// Listing Listing `gorm:"foreignKey:RelatedListingID" json:"-"` // For eager loading listing info if needed
}

// TableName specifies the table name for GORM.
func (Notification) TableName() string {
	return "notifications"
}
