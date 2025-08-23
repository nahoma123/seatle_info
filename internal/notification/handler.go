package notification

import (
	"seattle_info_backend/internal/common"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type Handler struct {
	service Service
	logger  *zap.Logger
}

func NewHandler(service Service, logger *zap.Logger) *Handler {
	return &Handler{
		service: service,
		logger:  logger,
	}
}

// RegisterRoutes sets up the routes for notification operations.
// All routes in this group should be authenticated.
func (h *Handler) RegisterRoutes(router *gin.RouterGroup) {
	router.GET("", h.getNotifications)
	router.POST("/:notification_id/mark-read", h.markNotificationAsRead)
	router.POST("/mark-all-read", h.markAllNotificationsAsRead)
}

func (h *Handler) getNotifications(c *gin.Context) {
	userID := common.GetUserIDFromContext(c)
	if userID == uuid.Nil {
		common.RespondWithError(c, common.ErrUnauthorized.WithDetails("User ID not found in token."))
		return
	}

	page, pageSize := common.GetPaginationParams(c)

	notifications, pagination, err := h.service.GetNotificationsForUser(c.Request.Context(), userID, page, pageSize)
	if err != nil {
		// Service layer should return appropriate common.APIError
		common.RespondWithError(c, err)
		return
	}
	common.RespondPaginated(c, "Notifications retrieved successfully.", notifications, pagination)
}

func (h *Handler) markNotificationAsRead(c *gin.Context) {
	userID := common.GetUserIDFromContext(c)
	if userID == uuid.Nil {
		common.RespondWithError(c, common.ErrUnauthorized.WithDetails("User ID not found in token."))
		return
	}

	notificationIDStr := c.Param("notification_id")
	notificationID, err := uuid.Parse(notificationIDStr)
	if err != nil {
		common.RespondWithError(c, common.ErrBadRequest.WithDetails("Invalid notification ID format."))
		return
	}

	err = h.service.MarkNotificationAsRead(c.Request.Context(), notificationID, userID)
	if err != nil {
		// Service should return common.ErrNotFound if not found/owned, or other errors
		common.RespondWithError(c, err)
		return
	}
	common.RespondSuccess(c, 200, "Notification marked as read successfully.", nil) // Or 204 No Content
}

func (h *Handler) markAllNotificationsAsRead(c *gin.Context) {
	userID := common.GetUserIDFromContext(c)
	if userID == uuid.Nil {
		common.RespondWithError(c, common.ErrUnauthorized.WithDetails("User ID not found in token."))
		return
	}

	_, err := h.service.MarkAllUserNotificationsAsRead(c.Request.Context(), userID) // Count is ignored here
	if err != nil {
		common.RespondWithError(c, err)
		return
	}
	// Consider returning 204 No Content if there's no body, or if count is 0.
	// For now, 200 OK with a message is fine.
	common.RespondSuccess(c, 200, "All notifications marked as read successfully.", nil)
}
