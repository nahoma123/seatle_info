// File: internal/auth/service.go
package auth

import (
	"errors" // For jwt error checking
	"fmt"
	"time"

	"seattle_info_backend/internal/config"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type JWTService struct {
	cfg    *config.Config
	logger *zap.Logger
}

// NewJWTService creates a new JWT service.
// THIS MUST RETURN THE INTERFACE TYPE: shared.TokenService
func NewJWTService(cfg *config.Config, logger *zap.Logger) shared.TokenService {
	return &JWTService{cfg: cfg, logger: logger}
}

func (s *JWTService) GenerateAccessToken(userData shared.UserDataForToken) (string, time.Time, error) {
	expirationTime := time.Now().Add(s.cfg.JWTAccessTokenExpiryMinutes)

	userEmailStr := ""
	if userData.GetEmail() != nil {
		userEmailStr = *userData.GetEmail()
	}

	claims := &shared.Claims{
		UserID: userData.GetID(),
		Email:  userEmailStr,
		Role:   userData.GetRole(),
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "seattle_info_backend",
			Subject:   userData.GetID().String(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(s.cfg.JWTSecretKey))
	if err != nil {
		s.logger.Error("Failed to sign access token", zap.Error(err))
		return "", time.Time{}, fmt.Errorf("could not sign access token: %w", err)
	}
	return tokenString, expirationTime, nil
}

func (s *JWTService) GenerateRefreshToken(userData shared.UserDataForToken) (string, time.Time, error) {
	expirationTime := time.Now().Add(s.cfg.JWTRefreshTokenExpiryDays)
	userEmailStr := ""
	if userData.GetEmail() != nil {
		userEmailStr = *userData.GetEmail()
	}
	claims := &shared.Claims{
		UserID: userData.GetID(),
		Email:  userEmailStr,
		Role:   userData.GetRole(),
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "seattle_info_backend_refresh",
			Subject:   userData.GetID().String(),
			ID:        uuid.NewString(),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(s.cfg.JWTSecretKey))
	if err != nil {
		s.logger.Error("Failed to sign refresh token", zap.Error(err))
		return "", time.Time{}, fmt.Errorf("could not sign refresh token: %w", err)
	}
	return tokenString, expirationTime, nil
}

// ValidateToken validates a JWT token and returns its claims.
func (s *JWTService) ValidateToken(tokenString string) (*shared.Claims, error) {
	claims := &shared.Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		return []byte(s.cfg.JWTSecretKey), nil
	})
	if err != nil {
		s.logger.Error("Failed to validate token", zap.Error(err))
		return nil, fmt.Errorf("invalid token: %w", err)
	}
	if claims, ok := token.Claims.(*shared.Claims); ok && token.Valid {
		return claims, nil
	} else {
		s.logger.Error("Token claims are invalid or token is invalid")
		return nil, errors.New("invalid token claims")
	}
}

func (s *JWTService) ParseRefreshToken(refreshTokenString string) (*shared.Claims, error) {
	return s.ValidateToken(refreshTokenString)
}
}
