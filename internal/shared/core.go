package shared

import (
	"context"
	"seattle_info_backend/internal/common" // For PaginationQuery
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

// UserSearchQuery defines the query parameters for searching users.
// Moved from internal/user/model.go to break import cycle.
type UserSearchQuery struct {
	common.PaginationQuery      // Embeds Page, PageSize, SortBy, SortOrder
	Email                *string `form:"email"` // Pointer to allow empty/nil value
	Name                 *string `form:"name"`  // Pointer to allow empty/nil value, will search FirstName and LastName
	Role                 *string `form:"role"`  // Pointer to allow empty/nil value
}

// Service defines the interface for user-related business logic.
// This interface remains as it's used by other parts of the application (e.g., handlers).
type Service interface {
	GetUserByID(ctx context.Context, id uuid.UUID) (*User, error)
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	GetOrCreateUserFromFirebaseClaims(ctx context.Context, firebaseToken *firebaseauth.Token) (usr *User, wasCreated bool, err error)
	GetUserByFirebaseUID(ctx context.Context, firebaseUID string) (*User, error)
	SearchUsers(ctx context.Context, query UserSearchQuery) ([]*User, *common.Pagination, error) // Now uses shared.UserSearchQuery
}

// Obsolete structs and interfaces related to old JWT/OAuth system are removed below.
// CreateUserRequest (removed)
// TokenResponse (removed)
// OAuthUserProfile (removed)
// UserDataForToken (removed)
// TokenService (removed)
// Claims (removed)
// OAuthUserProvider (removed)
