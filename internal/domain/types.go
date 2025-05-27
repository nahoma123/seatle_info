package domain

import (
	"github.com/google/uuid"
)

// User represents a user in the system.
type User struct {
	ID        uuid.UUID
	Email     string
	Password  string
	FirstName string
	LastName  string
	Role      string
}

// CreateUserRequest represents a request to create a new user.
type CreateUserRequest struct {
	Email     string
	Password  string
	FirstName string
	LastName  string
}

// OAuthUserProfile holds common profile data from OAuth providers.
type OAuthUserProfile struct {
	Provider      string
	ProviderID    string
	Email         string
	FirstName     string
	LastName      string
	PictureURL    string
	EmailVerified bool
}
