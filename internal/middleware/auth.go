// File: internal/middleware/auth.go
package middleware

import (
	"seattle_info_backend/internal/common" // For common.RespondWithError and error types
	"seattle_info_backend/internal/shared" // For shared.TokenService and shared.Claims
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	// AuthorizationHeader is the header name for authorization token
	AuthorizationHeader = "Authorization"
	// AuthorizationTypeBearer is the prefix for Bearer tokens
	AuthorizationTypeBearer = "Bearer"
	// UserIDKey is the context key for storing the authenticated user's ID
	UserIDKey = "userID"
	// UserEmailKey is the context key for storing the authenticated user's email
	UserEmailKey = "userEmail"
	// UserRoleKey is the context key for storing the authenticated user's role
	UserRoleKey = "userRole"
	// UserClaimsKey stores the whole claims object
	UserClaimsKey = "userClaims"
)

// AuthMiddleware creates a Gin middleware for JWT authentication.
func AuthMiddleware(tokenService shared.TokenService, logger *zap.Logger) gin.HandlerFunc { // Changed to shared.TokenService
	return func(c *gin.Context) {
		authHeader := c.GetHeader(AuthorizationHeader)
		if authHeader == "" {
			logger.Debug("Authorization header missing")
			common.RespondWithError(c, common.ErrUnauthorized.WithDetails("Authorization header is required."))
			// c.Abort() is handled by RespondWithError's AbortWithStatusJSON
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || strings.ToLower(parts[0]) != strings.ToLower(AuthorizationTypeBearer) {
			logger.Debug("Authorization header format invalid", zap.String("header", authHeader))
			common.RespondWithError(c, common.ErrUnauthorized.WithDetails("Authorization header format must be 'Bearer <token>'."))
			return
		}

		tokenString := parts[1]
		claims, err := tokenService.ValidateToken(tokenString)
		if err != nil {
			logger.Warn("Token validation failed", zap.Error(err))
			common.RespondWithError(c, common.ErrUnauthorized.WithDetails(err.Error())) // Use specific error from validation
			return
		}

		// Set user information in context for downstream handlers
		c.Set(UserIDKey, claims.UserID)
		c.Set(UserEmailKey, claims.Email)
		c.Set(UserRoleKey, claims.Role)
		c.Set(UserClaimsKey, claims) // Store full claims if needed

		logger.Debug("User authenticated successfully",
			zap.String("userID", claims.UserID.String()),
			zap.String("email", claims.Email),
			zap.String("role", claims.Role),
		)

		c.Next()
	}
}

// GetUserIDFromContext retrieves the user ID from the Gin context.
// Returns uuid.Nil if not found or not a UUID.
func GetUserIDFromContext(c *gin.Context) uuid.UUID {
	val, exists := c.Get(UserIDKey)
	if !exists {
		return uuid.Nil
	}
	userID, ok := val.(uuid.UUID)
	if !ok {
		return uuid.Nil
	}
	return userID
}

// GetUserRoleFromContext retrieves the user role from the Gin context.
func GetUserRoleFromContext(c *gin.Context) string {
	val, exists := c.Get(UserRoleKey)
	if !exists {
		return ""
	}
	role, ok := val.(string)
	if !ok {
		return ""
	}
	return role
}

// GetUserClaimsFromContext retrieves the full claims object from the Gin context.
func GetUserClaimsFromContext(c *gin.Context) *shared.Claims { // Changed to *shared.Claims
	val, exists := c.Get(UserClaimsKey)
	if !exists {
		return nil
	}
	claims, ok := val.(*shared.Claims) // Changed to *shared.Claims
	if !ok {
		return nil
	}
	return claims
}

// RoleAuthMiddleware creates a middleware to check if the authenticated user has one of the required roles.
func RoleAuthMiddleware(allowedRoles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userRole := GetUserRoleFromContext(c)
		if userRole == "" {
			// This should ideally not happen if AuthMiddleware ran successfully
			common.RespondWithError(c, common.ErrForbidden.WithDetails("User role not found in context."))
			return
		}

		isAllowed := false
		for _, role := range allowedRoles {
			if userRole == role {
				isAllowed = true
				break
			}
		}

		if !isAllowed {
			common.RespondWithError(c, common.ErrForbidden.WithDetails("You do not have sufficient permissions for this resource."))
			return
		}
		c.Next()
	}
}
