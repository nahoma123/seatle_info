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
	PasswordHash        *string `gorm:"type:varchar(255)"`             // Pointer to allow NULL
	FirstName           *string `gorm:"type:varchar(100)"`
	LastName            *string `gorm:"type:varchar(100)"`
	ProfilePictureURL   *string `gorm:"type:text"`
	AuthProvider        string  `gorm:"type:varchar(50);not null;default:'email'"`
	ProviderID          *string `gorm:"type:varchar(255);index:idx_auth_provider_provider_id,unique"` // Part of composite unique index
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

// CreateUserRequest defines the structure for creating a new user.
type CreateUserRequest struct {
	Email     string `json:"email" binding:"required,email"`
	Password  string `json:"password" binding:"required,min=8,max=72"` // bcrypt max is 72 bytes
	FirstName string `json:"first_name,omitempty" binding:"omitempty,max=100"`
	LastName  string `json:"last_name,omitempty" binding:"omitempty,max=100"`
}

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

// ToUserResponse converts a User model to a UserResponse DTO.
func ToUserResponse(user *User) UserResponse {
	return UserResponse{
		ID:                  user.ID,
		Email:               user.Email,
		FirstName:           user.FirstName,
		LastName:            user.LastName,
		ProfilePictureURL:   user.ProfilePictureURL,
		AuthProvider:        user.AuthProvider,
		IsEmailVerified:     user.IsEmailVerified,
		Role:                user.Role,
		IsFirstPostApproved: user.IsFirstPostApproved,
		CreatedAt:           user.CreatedAt,
		UpdatedAt:           user.UpdatedAt,
		LastLoginAt:         user.LastLoginAt,
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
