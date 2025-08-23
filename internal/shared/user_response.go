// File: internal/shared/user_response.go
package shared

import (
	"time"

	"github.com/google/uuid"
)

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
func ToUserResponse(svUser *User) UserResponse {
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
