// File: internal/user/handler.go
package user

import (
	"seattle_info_backend/internal/common"
	"seattle_info_backend/internal/middleware"
	"seattle_info_backend/internal/shared" // Added import

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Handler struct holds dependencies for user handlers.
type Handler struct {
	service shared.Service // Changed to shared.Service
	logger  *zap.Logger
}

// NewHandler creates a new user handler.
// It does NOT take auth.TokenService.
func NewHandler(service shared.Service, logger *zap.Logger) *Handler { // Changed to shared.Service
	return &Handler{
		service: service,
		logger:  logger,
	}
}

// RegisterRoutes sets up the routes for user operations.
// It takes the auth middleware function as a parameter.
func (h *Handler) RegisterRoutes(router *gin.RouterGroup, authMW gin.HandlerFunc) {
	userGroup := router.Group("/users") // This base group is passed from app/server.go
	// No specific middleware applied directly here, assuming /users base is public or auth is handled by caller.
	// If /users itself needs auth, authMW should be applied by the caller to the group passed to RegisterRoutes.

	// Public routes (if any) for users could be registered on userGroup directly.
	// Example: userGroup.GET("/:id", h.getUserByID) // If GET /users/:id is public

	// Authenticated routes
	authenticatedUserGroup := userGroup.Group("") // Create a new subgroup for authenticated routes
	authenticatedUserGroup.Use(authMW)            // Apply auth middleware to this subgroup
	{
		authenticatedUserGroup.GET("/me", h.getMe)
		// If GET /users/:id is private and needs auth:
		// authenticatedUserGroup.GET("/:id", h.getUserByID)
	}

	// For the current setup, where /auth/me is handled by authHandler,
	// and userHandler is expected to handle /users/me (private) and /users/:id (public).
	// The userGroup itself should be public for /:id.
	// A specific authenticated group is needed for /me.

	// Corrected structure based on typical needs:
	// Public routes on userGroup
	userGroup.GET("/:id", h.getUserByID) // Publicly accessible user profile

	// Authenticated specific routes
	// The /me route is already registered by authHandler using authMW.
	// If there are other user-specific authenticated routes, they'd go into a group like this:
	// privateUserRoutes := userGroup.Group("") // Or some other path like /profile
	// privateUserRoutes.Use(authMW)
	// {
	//    // e.g., privateUserRoutes.PUT("/me/profile", h.updateMyProfile)
	// }
}

func (h *Handler) getMe(c *gin.Context) {
	// This /me handler in user.Handler might be redundant if /auth/me already serves user profiles.
	// However, if it's intended for user-specific profile management (e.g., PUT /users/me), it's fine.
	// For now, assuming it's the primary way to get the authenticated user's own profile.
	userID := middleware.GetUserIDFromContext(c)
	if userID == uuid.Nil {
		h.logger.Error("User ID not found in context for /me", zap.String("path", c.Request.URL.Path))
		common.RespondWithError(c, common.ErrInternalServer.WithDetails("User identifier missing."))
		return
	}
	usr, err := h.service.GetUserByID(c.Request.Context(), userID)
	if err != nil {
		common.RespondWithError(c, err)
		return
	}
	common.RespondOK(c, "User profile retrieved successfully.", ToUserResponse(usr))
}

func (h *Handler) getUserByID(c *gin.Context) {
	paramID := c.Param("id")
	userIDToFetch, err := uuid.Parse(paramID)
	if err != nil {
		h.logger.Warn("Invalid user ID format in URL parameter", zap.String("paramID", paramID), zap.Error(err))
		common.RespondWithError(c, common.ErrBadRequest.WithDetails("Invalid user ID format."))
		return
	}

	// For a public /users/:id endpoint, we don't check requesting user's identity or role here.
	// If this endpoint were to be private or have role-based access,
	// it should be part of an authenticated group and have checks like:
	// requestingUserID := middleware.GetUserIDFromContext(c)
	// requestingUserRole := middleware.GetUserRoleFromContext(c)
	// if requestingUserRole != common.RoleAdmin && requestingUserID != userIDToFetch {
	// 	common.RespondWithError(c, common.ErrForbidden.WithDetails("You are not authorized to view this profile."))
	// 	return
	// }

	usr, err := h.service.GetUserByID(c.Request.Context(), userIDToFetch)
	if err != nil {
		common.RespondWithError(c, err) // Handles common.ErrNotFound appropriately
		return
	}
	common.RespondOK(c, "User retrieved successfully.", ToUserResponse(usr))
}
