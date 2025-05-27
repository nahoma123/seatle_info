// File: internal/auth/model.go
package auth

// LoginRequest defines the structure for login requests.
type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

// RefreshTokenRequest defines the structure for refresh token requests.
type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}
