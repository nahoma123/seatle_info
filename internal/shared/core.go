package shared

import (
	"context"
	"time"

	firebaseauth "firebase.google.com/go/v4/auth"
	"github.com/golang-jwt/jwt/v5"
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

// CreateUserRequest represents a request to create a new user.
type CreateUserRequest struct {
	Email     string
	Password  string
	FirstName string
	LastName  string
}

// TokenResponse represents the response containing JWT tokens.
type TokenResponse struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	TokenType    string    `json:"token_type"`
}

// Service defines the interface for user-related business logic.
type Service interface {
	GetUserByID(ctx context.Context, id uuid.UUID) (*User, error)
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	GetOrCreateUserFromFirebaseClaims(ctx context.Context, firebaseToken *firebaseauth.Token) (usr *User, wasCreated bool, err error)
	GetUserByFirebaseUID(ctx context.Context, firebaseUID string) (*User, error)
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

// UserDataForToken is an interface to abstract the user data needed for token generation.
type UserDataForToken interface {
	GetID() uuid.UUID
	GetEmail() *string
	GetRole() string
}

// TokenService defines the interface for JWT operations.
type TokenService interface {
	GenerateAccessToken(userData UserDataForToken) (string, time.Time, error)
	GenerateRefreshToken(userData UserDataForToken) (string, time.Time, error)
	ValidateToken(tokenString string) (*Claims, error)
	ParseRefreshToken(refreshTokenString string) (*Claims, error)
}

// Claims represents the JWT claims structure
type Claims struct {
	UserID uuid.UUID `json:"user_id"`
	Email  string    `json:"email"`
	Role   string    `json:"role"`
	jwt.RegisteredClaims
}

// OAuthUserProvider defines the user operations needed by the OAuthService.
type OAuthUserProvider interface {
	// FindOrCreateOrLinkOAuthUser(ctx context.Context, profile OAuthUserProfile) (usr *User, wasCreated bool, err error) // Removed as per instructions
	GetUserByID(ctx context.Context, id uuid.UUID) (*User, error)
}
