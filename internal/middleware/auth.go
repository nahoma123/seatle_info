// File: internal/middleware/auth.go
package middleware

import (
	"strings"

	"seattle_info_backend/internal/common" // For common.RespondWithError and error types
	"seattle_info_backend/internal/firebase"
	"seattle_info_backend/internal/shared" // For shared.Service (user service)

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
	// FirebaseUIDKey is the context key for storing the Firebase UID
	FirebaseUIDKey = "firebaseUID"
	// UserClaimsKey stores the whole claims object - Note: Re-evaluating its use for Firebase.
	// For now, we will not set this with the full Firebase token to keep context light.
	// UserClaimsKey = "userClaims"
)

// AuthMiddleware creates a Gin middleware for Firebase authentication.
func AuthMiddleware(firebaseService *firebase.FirebaseService, userService shared.Service, logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader(AuthorizationHeader)
		if authHeader == "" {
			logger.Debug("Authorization header missing")
			common.RespondWithError(c, common.ErrUnauthorized.WithDetails("Authorization header is required."))
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || strings.ToLower(parts[0]) != strings.ToLower(AuthorizationTypeBearer) {
			logger.Debug("Authorization header format invalid", zap.String("header", authHeader))
			common.RespondWithError(c, common.ErrUnauthorized.WithDetails("Authorization header format must be 'Bearer <token>'."))
			return
		}

		tokenString := parts[1]
		firebaseToken, err := firebaseService.VerifyIDToken(c.Request.Context(), tokenString)
		if err != nil {
			logger.Warn("Firebase token validation failed", zap.Error(err))
			common.RespondWithError(c, common.ErrUnauthorized.WithDetails("Invalid or expired token: "+err.Error()))
			return
		}

		localUser, wasCreated, err := userService.GetOrCreateUserFromFirebaseClaims(c.Request.Context(), firebaseToken)
		if err != nil {
			logger.Error("Failed to get or create user from Firebase claims", zap.Error(err), zap.String("firebaseUID", firebaseToken.UID))
			common.RespondWithError(c, common.ErrInternalServer.WithDetails("Failed to process user authentication."))
			return
		}

		if wasCreated {
			logger.Info("New local user created from Firebase token", zap.String("userID", localUser.ID.String()), zap.String("firebaseUID", firebaseToken.UID))
		}

		// Set user information in context for downstream handlers
		c.Set(UserIDKey, localUser.ID)
		if localUser.Email != nil {
			c.Set(UserEmailKey, *localUser.Email)
		} else {
			c.Set(UserEmailKey, "") // Handle nil email
		}
		c.Set(UserRoleKey, localUser.Role)
		c.Set(FirebaseUIDKey, firebaseToken.UID)

		logger.Debug("User authenticated via Firebase successfully",
			zap.String("localUserID", localUser.ID.String()),
			zap.String("firebaseUID", firebaseToken.UID),
			zap.Stringp("email", localUser.Email),
			zap.String("role", localUser.Role),
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
