// File: internal/auth/handler.go
package auth

import (
	"errors" // Kept for common.IsAPIError if used, or can be removed if not
	"seattle_info_backend/internal/common"
	"seattle_info_backend/internal/shared"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid" // For uuid.Nil
	"go.uber.org/zap"
)

// Handler struct holds dependencies for auth handlers.
type Handler struct {
	userService shared.Service // Interface type
	logger      *zap.Logger
}

// NewHandler creates a new auth handler.
func NewHandler(
	userService shared.Service,
	logger *zap.Logger,
) *Handler {
	return &Handler{
		userService: userService,
		logger:      logger,
	}
}

// RegisterRoutes sets up the routes for authentication operations.
func (h *Handler) RegisterRoutes(router *gin.RouterGroup) {
	authGroup := router.Group("/")
	{
		authGroup.GET("/me", h.me)
		// Middleware is applied in server.go where this router group is passed
	}
}

// me handler retrieves the authenticated user's profile.
func (h *Handler) me(c *gin.Context) {
	userID := common.GetUserIDFromContext(c)
	if userID == uuid.Nil {
		h.logger.Warn("/me: UserID not found in context or is Nil")
		common.RespondWithError(c, common.ErrUnauthorized.WithDetails("User ID not found in context."))
		return
	}

	sharedUser, err := h.userService.GetUserByID(c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			h.logger.Warn("/me: User not found in DB with ID from context", zap.String("userID", userID.String()), zap.Error(err))
			common.RespondWithError(c, common.ErrNotFound.WithDetails("Authenticated user not found."))
			return
		}
		h.logger.Error("/me: Error fetching user by ID", zap.String("userID", userID.String()), zap.Error(err))
		common.RespondWithError(c, common.ErrInternalServer.WithDetails("Failed to retrieve user profile."))
		return
	}

	userResponse := shared.ToUserResponse(sharedUser)
	common.RespondOK(c, "User profile retrieved successfully.", userResponse)
}
