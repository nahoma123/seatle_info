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
// It takes auth and admin role middleware functions as parameters.
func (h *Handler) RegisterRoutes(router *gin.RouterGroup, authMW gin.HandlerFunc, adminRoleMW gin.HandlerFunc) {
	userGroup := router.Group("/users")

	// Publicly accessible user profile
	userGroup.GET("/:id", h.getUserByID)

	// Authenticated route for the user to get their own profile
	// Note: /auth/me is handled by authHandler. This /users/me is an alternative or additional route.
	// If /auth/me is the primary, this specific /users/me might be redundant or serve a slightly different purpose.
	// For now, keeping it as per existing structure, assuming it's desired.
	authenticatedUserGroup := userGroup.Group("/me")
	authenticatedUserGroup.Use(authMW)
	{
		authenticatedUserGroup.GET("", h.getMe) // Responds to GET /users/me
	}

	// Admin-only route for searching/listing users
	// This makes GET /users an admin-only endpoint.
	userGroup.GET("", authMW, adminRoleMW, h.searchUsers)
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

// searchUsers handles GET requests to search for users based on query parameters.
// It supports pagination and filtering by email, name, and role.
func (h *Handler) searchUsers(c *gin.Context) {
	var query UserSearchQuery // This is user.UserSearchQuery from model.go

	// Bind query parameters (e.g., email, name, role)
	if err := c.ShouldBindQuery(&query); err != nil {
		h.logger.Warn("Failed to bind query parameters for user search", zap.Error(err))
		common.RespondWithError(c, common.ErrBadRequest.WithDetails("Invalid search query parameters: "+err.Error()))
		return
	}

	// Get pagination parameters (page, page_size) and set them in the query struct
	// UserSearchQuery embeds common.PaginationQuery, so Page and PageSize fields are directly available.
	query.Page, query.PageSize = common.GetPaginationParams(c)

	h.logger.Debug("Handler: Initiating user search", zap.Any("query", query))

	// Call the service layer to search for users
	sharedUsers, pagination, err := h.service.SearchUsers(c.Request.Context(), query)
	if err != nil {
		// The service layer should return appropriate common.APIError types
		h.logger.Error("Handler: Error searching users via service", zap.Error(err), zap.Any("query", query))
		common.RespondWithError(c, err) // Pass the error directly
		return
	}

	// Convert []*shared.User to []UserResponse
	userResponses := make([]UserResponse, 0, len(sharedUsers))
	for _, sharedUser := range sharedUsers {
		if sharedUser != nil { // Ensure sharedUser is not nil before converting
			userResponses = append(userResponses, ToUserResponse(sharedUser))
		}
	}

	h.logger.Info("Handler: User search successful", zap.Int("count", len(userResponses)), zap.Any("pagination", pagination))
	common.RespondPaginated(c, "Users retrieved successfully.", userResponses, pagination)
}
