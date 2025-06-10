// File: internal/listing/handler.go
package listing

import (
	"errors" // Go standard errors
	// "seattle_info_backend/internal/auth" // REMOVED
	"seattle_info_backend/internal/common"
	"seattle_info_backend/internal/middleware"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Handler struct holds dependencies for listing handlers.
type Handler struct {
	service Service
	logger  *zap.Logger
	// tokenService auth.TokenService // REMOVED
}

// NewHandler creates a new listing handler.
// func NewHandler(service Service, tokenService auth.TokenService, logger *zap.Logger) *Handler { // OLD
func NewHandler(service Service, logger *zap.Logger) *Handler { // NEW
	return &Handler{
		service: service,
		logger:  logger,
		// tokenService: tokenService, // REMOVED
	}
}

// RegisterRoutes sets up the routes for listing operations.
func (h *Handler) RegisterRoutes(router *gin.RouterGroup, authMW gin.HandlerFunc, adminRoleMW gin.HandlerFunc) { // Pass middlewares
	listingGroup := router.Group("/listings")
	{
		listingGroup.GET("", h.searchListings)
		listingGroup.GET("/:id", h.getListingByID)
		listingGroup.GET("/recent", h.getRecentListings) // New Public Route

		authedListingGroup := listingGroup.Group("")
		authedListingGroup.Use(authMW) // Apply general auth
		{
			authedListingGroup.POST("", h.createListing)
			authedListingGroup.PUT("/:id", h.updateListing)
			authedListingGroup.DELETE("/:id", h.deleteListing)
			authedListingGroup.GET("/my-listings", h.getMyListings) // New route for user's own listings
		}

		adminListingGroup := listingGroup.Group("/admin")
		adminListingGroup.Use(authMW)
		adminListingGroup.Use(adminRoleMW) // Apply admin role check
		{
			adminListingGroup.GET("/:id", h.adminGetListingByID)
			adminListingGroup.PATCH("/:id/status", h.adminUpdateListingStatus)
			adminListingGroup.POST("/:id/approve", h.adminApproveListing)
		}
	}
}

func (h *Handler) createListing(c *gin.Context) {
	userID := middleware.GetUserIDFromContext(c)
	if userID == uuid.Nil {
		common.RespondWithError(c, common.ErrInternalServer.WithDetails("User ID not found."))
		return
	}
	var req CreateListingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("Create listing: Invalid request body", zap.Error(err), zap.String("userID", userID.String()))
		var ve validator.ValidationErrors
		if errors.As(err, &ve) {
			common.RespondWithError(c, common.NewValidationAPIError(common.FormatValidationErrors(ve)))
			return
		}
		common.RespondWithError(c, common.ErrBadRequest.WithDetails(err.Error()))
		return
	}
	listing, err := h.service.CreateListing(c.Request.Context(), userID, req)
	if err != nil {
		common.RespondWithError(c, err)
		return
	}
	common.RespondCreated(c, "Listing created successfully.", ToListingResponse(listing, true))
}

func (h *Handler) getListingByID(c *gin.Context) {
	listingID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		common.RespondWithError(c, common.ErrBadRequest.WithDetails("Invalid listing ID format."))
		return
	}

	var authenticatedUserID *uuid.UUID
	// Check if X-User-ID header is set by AuthMiddleware (if it runs for this route implicitly or explicitly)
	userIDFromCtx := middleware.GetUserIDFromContext(c)
	if userIDFromCtx != uuid.Nil {
		authenticatedUserID = &userIDFromCtx
	}

	listing, err := h.service.GetListingByID(c.Request.Context(), listingID, authenticatedUserID)
	if err != nil {
		common.RespondWithError(c, err)
		return
	}
	isAuthenticatedForContact := authenticatedUserID != nil
	common.RespondOK(c, "Listing retrieved successfully.", ToListingResponse(listing, isAuthenticatedForContact))
}

func (h *Handler) searchListings(c *gin.Context) {
	var query ListingSearchQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		h.logger.Warn("Search listings: Invalid query parameters", zap.Error(err))
		common.RespondWithError(c, common.ErrBadRequest.WithDetails("Invalid query parameters: "+err.Error()))
		return
	}
	query.Page, query.PageSize = common.GetPaginationParams(c)

	var authenticatedUserID *uuid.UUID
	userIDFromCtx := middleware.GetUserIDFromContext(c)
	if userIDFromCtx != uuid.Nil {
		authenticatedUserID = &userIDFromCtx
	}

	listings, pagination, err := h.service.SearchListings(c.Request.Context(), query, authenticatedUserID)
	if err != nil {
		common.RespondWithError(c, err)
		return
	}
	listingResponses := make([]ListingResponse, len(listings))
	isAuthenticatedForContact := authenticatedUserID != nil
	for i, l := range listings {
		listingResponses[i] = ToListingResponse(&l, isAuthenticatedForContact)
		// If distance needs to be added from a gorm:"-" field:
		// distanceVal, ok := c.Get(fmt.Sprintf("distance_listing_%s", l.ID.String())) // Example of how service might pass it
		// if ok {
		//     if distFloat, okFloat := distanceVal.(float64); okFloat {
		//         listingResponses[i].Distance = &distFloat
		//     }
		// }
	}
	common.RespondPaginated(c, "Listings retrieved successfully.", listingResponses, pagination)
}

func (h *Handler) getMyListings(c *gin.Context) {
	userID := middleware.GetUserIDFromContext(c)
	if userID == uuid.Nil {
		// This should ideally not happen if auth middleware is effective
		common.RespondWithError(c, common.ErrUnauthorized.WithDetails("User not authenticated."))
		return
	}

	var query UserListingsQuery
	// Bind query parameters like status, category_slug
	if err := c.ShouldBindQuery(&query); err != nil {
		h.logger.Warn("Get my listings: Invalid query parameters", zap.Error(err), zap.String("userID", userID.String()))
		common.RespondWithError(c, common.ErrBadRequest.WithDetails("Invalid query parameters: "+err.Error()))
		return
	}

	// Populate pagination parameters
	query.Page, query.PageSize = common.GetPaginationParams(c)

	listings, pagination, err := h.service.GetUserListings(c.Request.Context(), userID, query)
	if err != nil {
		// Service layer is responsible for logging the error details
		common.RespondWithError(c, err) // Respond with the error passed from the service
		return
	}

	listingResponses := make([]ListingResponse, len(listings))
	for i, l := range listings {
		// For "my listings", the user is authenticated and is the owner, so they should see full details.
		listingResponses[i] = ToListingResponse(&l, true)
	}

	common.RespondPaginated(c, "Successfully retrieved your listings.", listingResponses, pagination)
}

func (h *Handler) updateListing(c *gin.Context) {
	listingID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		common.RespondWithError(c, common.ErrBadRequest.WithDetails("Invalid listing ID format."))
		return
	}
	userID := middleware.GetUserIDFromContext(c)
	if userID == uuid.Nil {
		common.RespondWithError(c, common.ErrInternalServer.WithDetails("User ID not found."))
		return
	}
	var req UpdateListingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("Update listing: Invalid request body", zap.Error(err), zap.String("listingID", listingID.String()))
		var ve validator.ValidationErrors
		if errors.As(err, &ve) {
			common.RespondWithError(c, common.NewValidationAPIError(common.FormatValidationErrors(ve)))
			return
		}
		common.RespondWithError(c, common.ErrBadRequest.WithDetails(err.Error()))
		return
	}
	listing, err := h.service.UpdateListing(c.Request.Context(), listingID, userID, req)
	if err != nil {
		common.RespondWithError(c, err)
		return
	}
	common.RespondOK(c, "Listing updated successfully.", ToListingResponse(listing, true))
}

func (h *Handler) deleteListing(c *gin.Context) {
	listingID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		common.RespondWithError(c, common.ErrBadRequest.WithDetails("Invalid listing ID format."))
		return
	}
	userID := middleware.GetUserIDFromContext(c)
	if userID == uuid.Nil {
		common.RespondWithError(c, common.ErrInternalServer.WithDetails("User ID not found."))
		return
	}
	if err := h.service.DeleteListing(c.Request.Context(), listingID, userID); err != nil {
		common.RespondWithError(c, err)
		return
	}
	common.RespondNoContent(c)
}

// --- Admin Handlers ---
func (h *Handler) adminGetListingByID(c *gin.Context) {
	listingID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		common.RespondWithError(c, common.ErrBadRequest.WithDetails("Invalid listing ID format."))
		return
	}
	listing, err := h.service.AdminGetListingByID(c.Request.Context(), listingID)
	if err != nil {
		common.RespondWithError(c, err)
		return
	}
	common.RespondOK(c, "Admin: Listing retrieved successfully.", ToListingResponse(listing, true))
}

func (h *Handler) adminUpdateListingStatus(c *gin.Context) {
	listingID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		common.RespondWithError(c, common.ErrBadRequest.WithDetails("Invalid listing ID format."))
		return
	}
	var req AdminUpdateListingStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("Admin update listing status: Invalid request body", zap.Error(err), zap.String("listingID", listingID.String()))
		var ve validator.ValidationErrors
		if errors.As(err, &ve) {
			common.RespondWithError(c, common.NewValidationAPIError(common.FormatValidationErrors(ve)))
			return
		}
		common.RespondWithError(c, common.ErrBadRequest.WithDetails(err.Error()))
		return
	}
	listing, err := h.service.AdminUpdateListingStatus(c.Request.Context(), listingID, req.Status, req.AdminNotes)
	if err != nil {
		common.RespondWithError(c, err)
		return
	}
	common.RespondOK(c, "Admin: Listing status updated successfully.", ToListingResponse(listing, true))
}

func (h *Handler) adminApproveListing(c *gin.Context) {
	listingID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		common.RespondWithError(c, common.ErrBadRequest.WithDetails("Invalid listing ID format."))
		return
	}
	listing, err := h.service.AdminApproveListing(c.Request.Context(), listingID)
	if err != nil {
		common.RespondWithError(c, err)
		return
	}
	common.RespondOK(c, "Admin: Listing approved successfully.", ToListingResponse(listing, true))
}

func (h *Handler) getRecentListings(c *gin.Context) {
	page, pageSize := common.GetPaginationParams(c)

	listings, pagination, err := h.service.GetRecentListings(c.Request.Context(), page, pageSize)
	if err != nil {
		common.RespondWithError(c, err) // Service layer should return appropriate common.APIError
		return
	}
	// For public recent listings, contact info is hidden by the service layer (ToListingResponse called with false)
	common.RespondPaginated(c, "Recent listings retrieved successfully.", listings, pagination)
}

// RegisterEventRoutes sets up the routes for event specific listing operations.
func (h *Handler) RegisterEventRoutes(router *gin.RouterGroup) {
	// The router group passed here is expected to be something like /api/v1/events
	router.GET("/upcoming", h.getUpcomingEvents)
}

func (h *Handler) getUpcomingEvents(c *gin.Context) {
	page, pageSize := common.GetPaginationParams(c)
	// Default page_size for events as per issue is 10.
	// common.GetPaginationParams uses 10 if 'page_size' is not provided or invalid, so this should be fine.

	events, pagination, err := h.service.GetUpcomingEvents(c.Request.Context(), page, pageSize)
	if err != nil {
		common.RespondWithError(c, err) // Service layer should return appropriate common.APIError
		return
	}
	// Contact info is hidden by the service layer (ToListingResponse called with false)
	common.RespondPaginated(c, "Upcoming events retrieved successfully.", events, pagination)
}
