// File: internal/auth/model.go
package auth

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// LoginRequest defines the structure for login requests.
type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

// TokenResponse defines the structure for token responses.
type TokenResponse struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"` // Optional based on flow
	TokenType    string    `json:"token_type"`
	ExpiresAt    time.Time `json:"expires_at"` // Expiry of the access token
}

// Claims defines the custom JWT claims.
type Claims struct {
	UserID uuid.UUID `json:"user_id"`
	Email  string    `json:"email"`
	Role   string    `json:"role"`
	jwt.RegisteredClaims
}

// RefreshTokenRequest defines the structure for refresh token requests.
type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}
