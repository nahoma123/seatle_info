package shared

import (
	"context"
	"time"

	firebaseauth "firebase.google.com/go/v4/auth"
	"github.com/google/uuid"
)

// User represents a user in the system.
type User struct {
	ID                  uuid.UUID
	Email               *string // Changed to pointer
	FirstName           *string // Changed to pointer
	LastName            *string // Changed to pointer
	Role                string
	ProfilePictureURL   *string   // New field
	AuthProvider        string    // New field
	IsEmailVerified     bool      // New field
	IsFirstPostApproved bool      // New field
	CreatedAt           time.Time // New field
	UpdatedAt           time.Time // New field
	LastLoginAt         *time.Time // New field
}

// Service defines the interface for user-related business logic.
// This interface remains as it's used by other parts of the application (e.g., handlers).
type Service interface {
	GetUserByID(ctx context.Context, id uuid.UUID) (*User, error)
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	GetOrCreateUserFromFirebaseClaims(ctx context.Context, firebaseToken *firebaseauth.Token) (usr *User, wasCreated bool, err error)
	GetUserByFirebaseUID(ctx context.Context, firebaseUID string) (*User, error)
}

// Obsolete structs and interfaces related to old JWT/OAuth system are removed below.
// CreateUserRequest (removed)
// TokenResponse (removed)
// OAuthUserProfile (removed)
// UserDataForToken (removed)
// TokenService (removed)
// Claims (removed)
// OAuthUserProvider (removed)
