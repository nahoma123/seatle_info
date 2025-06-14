// File: internal/user/model.go
package user

import (
	"seattle_info_backend/internal/common" // For BaseModel
	"seattle_info_backend/internal/shared" // Added import
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

// UserSearchQuery defines the query parameters for searching users.
type UserSearchQuery struct {
	common.PaginationQuery      // Embeds Page, PageSize, SortBy, SortOrder
	Email                *string `form:"email"` // Pointer to allow empty/nil value
	Name                 *string `form:"name"`  // Pointer to allow empty/nil value, will search FirstName and LastName
	Role                 *string `form:"role"`  // Pointer to allow empty/nil value
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

// UserResponse defines the structure for user data sent in API responses.
type UserResponse struct {
	ID                  uuid.UUID  `json:"id"`
	Email               *string    `json:"email,omitempty"`
	FirstName           *string    `json:"first_name,omitempty"`
	LastName            *string    `json:"last_name,omitempty"`
	ProfilePictureURL   *string    `json:"profile_picture_url,omitempty"`
	AuthProvider        string     `json:"auth_provider"`
	IsEmailVerified     bool       `json:"is_email_verified"`
	Role                string     `json:"role"`
	IsFirstPostApproved bool       `json:"is_first_post_approved"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
	LastLoginAt         *time.Time `json:"last_login_at,omitempty"`
}

// ToUserResponse converts a shared.User to a UserResponse DTO.
func ToUserResponse(svUser *shared.User) UserResponse {
	return UserResponse{
		ID:                  svUser.ID,
		Email:               svUser.Email,
		FirstName:           svUser.FirstName,
		LastName:            svUser.LastName,
		ProfilePictureURL:   svUser.ProfilePictureURL,
		AuthProvider:        svUser.AuthProvider,
		IsEmailVerified:     svUser.IsEmailVerified,
		Role:                svUser.Role,
		IsFirstPostApproved: svUser.IsFirstPostApproved,
		CreatedAt:           svUser.CreatedAt,
		UpdatedAt:           svUser.UpdatedAt,
		LastLoginAt:         svUser.LastLoginAt,
	}
}

func (u *User) GetID() uuid.UUID {
	return u.ID
}

func (u *User) GetEmail() *string { // Return pointer to handle nil
	return u.Email
}

func (u *User) GetRole() string {
	return u.Role
}
