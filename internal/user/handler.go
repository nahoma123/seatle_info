// File: internal/user/handler.go
package user

import (
	"errors"
	"seattle_info_backend/internal/common"
	"seattle_info_backend/internal/middleware"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Handler struct holds dependencies for user handlers.
type Handler struct {
	service Service // Depends on user.Service
	logger  *zap.Logger
}

// NewHandler creates a new user handler.
// It does NOT take auth.TokenService.
func NewHandler(service Service, logger *zap.Logger) *Handler {
	return &Handler{
		service: service,
		logger:  logger,
	}
}

// RegisterRoutes sets up the routes for user operations.
// It takes the auth middleware function as a parameter.
func (h *Handler) RegisterRoutes(router *gin.RouterGroup, authMW gin.HandlerFunc) {
	userGroup := router.Group("/users")
	{
		userGroup.POST("/register", h.register)

		authenticatedUserGroup := userGroup.Group("")
		authenticatedUserGroup.Use(authMW)
		{
			authenticatedUserGroup.GET("/me", h.getMe)
			authenticatedUserGroup.GET("/:id", h.getUserByID)
		}
	}
}

func (h *Handler) register(c *gin.Context) {
	var req CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("User registration: Invalid request body", zap.Error(err))
		var ve validator.ValidationErrors
		if errors.As(err, &ve) {
			common.RespondWithError(c, common.NewValidationAPIError(common.FormatValidationErrors(ve)))
			return
		}
		common.RespondWithError(c, common.ErrBadRequest.WithDetails(err.Error()))
		return
	}
	usr, tokenResponse, err := h.service.Register(c.Request.Context(), req)
	if err != nil {
		common.RespondWithError(c, err)
		return
	}
	response := gin.H{"user": ToUserResponse(usr), "token": tokenResponse}
	common.RespondCreated(c, "User registered successfully.", response)
}

func (h *Handler) getMe(c *gin.Context) {
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
	requestingUserID := middleware.GetUserIDFromContext(c)
	requestingUserRole := middleware.GetUserRoleFromContext(c)
	if requestingUserRole != "admin" && requestingUserID != userIDToFetch {
		h.logger.Warn("User attempting to fetch another user's profile without admin rights",
			zap.String("requestingUserID", requestingUserID.String()),
			zap.String("targetUserID", userIDToFetch.String()))
		common.RespondWithError(c, common.ErrForbidden.WithDetails("You are not authorized to view this profile."))
		return
	}
	usr, err := h.service.GetUserByID(c.Request.Context(), userIDToFetch)
	if err != nil {
		common.RespondWithError(c, err)
		return
	}
	common.RespondOK(c, "User retrieved successfully.", ToUserResponse(usr))
}
