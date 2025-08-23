// File: internal/user/model.go
package user

import (
	"seattle_info_backend/internal/common" // For BaseModel
	"time"

	"github.com/google/uuid"
)

// User represents the user model in the database.
type User struct {
	common.BaseModel            // Embeds ID, CreatedAt, UpdatedAt
	Email               *string `gorm:"type:varchar(255);uniqueIndex"` // Pointer to allow NULL
	PasswordHash        *string `gorm:"type:varchar(255)"`             // Deprecated: Passwords will be managed by Firebase
	FirstName           *string `gorm:"type:varchar(100)"`
	LastName            *string `gorm:"type:varchar(100)"`
	ProfilePictureURL   *string `gorm:"type:text"`
	AuthProvider        string  `gorm:"type:varchar(50);not null;default:'email'"`
	ProviderID          *string `gorm:"type:varchar(255);index:idx_auth_provider_provider_id,unique"` // Deprecated: For Firebase auth, FirebaseUID is the primary identifier. This might be used for migrating old OAuth users or specific non-Firebase OAuth if ever re-added.
	FirebaseUID         *string `gorm:"type:varchar(255);uniqueIndex;comment:Firebase User ID"`
	IsEmailVerified     bool    `gorm:"not null;default:false"`
	Role                string  `gorm:"type:varchar(50);not null;default:'user'"` // e.g., "user", "admin"
	IsFirstPostApproved bool    `gorm:"not null;default:false"`
	LastLoginAt         *time.Time
	// Listings            []listing.Listing `gorm:"foreignKey:UserID"` // This will cause import cycle if listing imports user
}

// TableName specifies the table name for the User model.
func (User) TableName() string {
	return "users"
}

// Sanitize removes sensitive information like password hash.
func (u *User) Sanitize() {
	u.PasswordHash = nil
}

// --- DTOs (Data Transfer Objects) for API requests/responses ---

func (u *User) GetID() uuid.UUID {
	return u.ID
}

func (u *User) GetEmail() *string { // Return pointer to handle nil
	return u.Email
}

func (u *User) GetRole() string {
	return u.Role
}
