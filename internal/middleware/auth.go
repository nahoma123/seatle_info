// File: internal/middleware/auth.go
package middleware

import (
	"strings"

	"seattle_info_backend/internal/auth"
	"seattle_info_backend/internal/common" // For common.RespondWithError and error types
	"seattle_info_backend/internal/firebase"
	"seattle_info_backend/internal/shared" // For shared.Service (user service)

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// AuthMiddleware creates a Gin middleware for Firebase authentication.
func AuthMiddleware(
	firebaseService *firebase.FirebaseService,
	userService shared.Service,
	blocklistService auth.TokenBlocklistService, // Add blocklist service
	logger *zap.Logger,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader(common.AuthorizationHeader)
		if authHeader == "" {
			logger.Debug("Authorization header missing")
			common.RespondWithError(c, common.ErrUnauthorized.WithDetails("Authorization header is required."))
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || strings.ToLower(parts[0]) != strings.ToLower(common.AuthorizationTypeBearer) {
			logger.Debug("Authorization header format invalid", zap.String("header", authHeader))
			common.RespondWithError(c, common.ErrUnauthorized.WithDetails("Authorization header format must be 'Bearer <token>'."))
			return
		}

		tokenString := parts[1]

		// First, verify the token's signature and expiration with Firebase
		firebaseToken, err := firebaseService.VerifyIDToken(c.Request.Context(), tokenString)
		if err != nil {
			logger.Warn("Firebase token validation failed", zap.Error(err))
			common.RespondWithError(c, common.ErrUnauthorized.WithDetails("Invalid or expired token: "+err.Error()))
			return
		}

		// After verification, check if the token's JTI is in the blocklist.
		// A JTI (JWT ID) is a standard claim in JWTs. Firebase tokens may or may not have it by default.
		// If they don't, we can use the token's signature or another unique, non-revocable identifier.
		// For this implementation, we will assume a 'jti' claim exists.
		// NOTE: Firebase ID tokens do NOT have a standard 'jti' claim.
		// A robust alternative is to use the raw token string itself or its signature as the key.
		// Let's use the raw token string as the identifier to blocklist.
		// This is simple and effective. The blocklist key will be the token itself.
		// A better, more standard approach if we controlled the JWT creation would be to add a 'jti'.
		// Given we are consuming Firebase tokens, we adapt.
		// Let's use the Firebase UID + issued at time as a unique identifier for the token.
		// The most unique identifier for a token is its signature, which is part of the token string itself.
		// So, we will blocklist the entire token string. This is simple and secure.
		isBlocklisted, err := blocklistService.IsBlocklisted(c.Request.Context(), tokenString)
		if err != nil {
			logger.Error("Error checking token blocklist", zap.Error(err))
			common.RespondWithError(c, common.ErrInternalServer.WithDetails("Could not verify token session."))
			return
		}
		if isBlocklisted {
			logger.Warn("Attempted to use a blocklisted token", zap.String("firebaseUID", firebaseToken.UID))
			common.RespondWithError(c, common.ErrUnauthorized.WithDetails("Token has been invalidated. Please log in again."))
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
		c.Set(common.UserIDKey, localUser.ID)
		if localUser.Email != nil {
			c.Set(common.UserEmailKey, *localUser.Email)
		} else {
			c.Set(common.UserEmailKey, "") // Handle nil email
		}
		c.Set(common.UserRoleKey, localUser.Role)
		c.Set(common.FirebaseUIDKey, firebaseToken.UID)

		logger.Debug("User authenticated via Firebase successfully",
			zap.String("localUserID", localUser.ID.String()),
			zap.String("firebaseUID", firebaseToken.UID),
			zap.Stringp("email", localUser.Email),
			zap.String("role", localUser.Role),
		)

		c.Next()
	}
}


// RoleAuthMiddleware creates a middleware to check if the authenticated user has one of the required roles.
func RoleAuthMiddleware(allowedRoles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userRole := common.GetUserRoleFromContext(c)
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
