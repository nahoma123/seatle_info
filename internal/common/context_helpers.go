// File: internal/common/context_helpers.go
package common

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// GetTokenFromContext retrieves the JWT token string from the Authorization header.
// Returns an empty string if not found.
func GetTokenFromContext(c *gin.Context) string {
	authHeader := c.GetHeader(AuthorizationHeader)
	if authHeader == "" {
		return ""
	}
	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || strings.ToLower(parts[0]) != strings.ToLower(AuthorizationTypeBearer) {
		return ""
	}
	return parts[1]
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

// GetFirebaseUIDFromContext retrieves the Firebase UID from the Gin context.
func GetFirebaseUIDFromContext(c *gin.Context) string {
	val, exists := c.Get(FirebaseUIDKey)
	if !exists {
		return ""
	}
	uid, ok := val.(string)
	if !ok {
		return ""
	}
	return uid
}
