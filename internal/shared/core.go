package shared

import (
	"context"
	"time"

	"github.com/golang-jwt/jwt/v5"
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

// TokenResponse represents the response containing JWT tokens.
type TokenResponse struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// Service defines the interface for user-related business logic.
type Service interface {
	Register(ctx context.Context, req CreateUserRequest) (*User, *TokenResponse, error)
	Login(ctx context.Context, email, password string) (*User, *TokenResponse, error)
	GetUserByID(ctx context.Context, id uuid.UUID) (*User, error)
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	FindOrCreateOrLinkOAuthUser(ctx context.Context, profile OAuthUserProfile) (usr *User, wasCreated bool, err error)
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
	GenerateAccessToken(userData shared.UserDataForToken) (string, time.Time, error)
	GenerateRefreshToken(userData shared.UserDataForToken) (string, time.Time, error)
	ValidateToken(tokenString string) (*shared.Claims, error)
	ParseRefreshToken(refreshTokenString string) (*shared.Claims, error)
}

// Claims represents the JWT claims structure
type Claims struct {
	UserID uuid.UUID `json:"user_id"`
	Email  string    `json:"email"`
	Role   string    `json:"role"`
}

// OAuthUserProvider defines the user operations needed by the OAuthService.
type OAuthUserProvider interface {
	FindOrCreateOrLinkOAuthUser(ctx context.Context, profile OAuthUserProfile) (usr interface{}, wasCreated bool, err error)
	GetUserByID(ctx context.Context, id uuid.UUID) (interface{}, error)
}
