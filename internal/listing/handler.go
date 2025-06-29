// File: internal/listing/handler.go
package listing

import (
	"errors" // Go standard errors
	// "mime/multipart" // Removed as direct usage isn't present; type is resolved via service interface
	// "strconv" // Removed
	// "seattle_info_backend/internal/auth" // REMOVED
	"seattle_info_backend/internal/common"
	"seattle_info_backend/internal/config" // Added for ImagePublicBaseURL
	"seattle_info_backend/internal/middleware"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Handler struct holds dependencies for listing handlers.
type Handler struct {
	service Service
	logger  *zap.Logger
	cfg     *config.Config // Added to access ImagePublicBaseURL
	// tokenService auth.TokenService // REMOVED
}

// NewHandler creates a new listing handler.
func NewHandler(service Service, logger *zap.Logger, cfg *config.Config) *Handler { // Added cfg
	return &Handler{
		service: service,
		logger:  logger,
		cfg:     cfg, // Added
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

	// Max memory for multipart form (e.g., 10 MB per image, adjust as needed)
	// If allowing multiple images, sum their potential max sizes.
	// Let's assume max 5 images, 10MB each = 50MB total.
	if err := c.Request.ParseMultipartForm(50 << 20); err != nil {
		h.logger.Warn("Create listing: Failed to parse multipart form", zap.Error(err), zap.String("userID", userID.String()))
		common.RespondWithError(c, common.ErrBadRequest.WithDetails("Invalid request format or files too large: "+err.Error()))
		return
	}

	var req CreateListingRequest
	// Bind form fields to the request struct.
	// For JSON fields within a multipart form, custom binding or manual parsing is often needed.
	// Gin's c.ShouldBindWith(&req, binding.Form) or c.ShouldBind(&req) might work if fields are simple form data.
	// If you have nested JSON objects (like BabysittingDetails), you might need to get them as strings
	// and then json.Unmarshal them manually.
	// For simplicity, let's assume form fields map directly or are handled by ShouldBind.
	// This will likely need adjustment if complex JSON parts are sent alongside files.

	// A common pattern is to have JSON data in one form field, and files in others.
	// e.g., form field "data" contains the JSON string for CreateListingRequest.
	// And form field "images" contains the files.

	// Let's try to bind the form values first.
	// Note: This simple binding might not work for nested JSON structures if they are not sent as flattened form fields.
	// You might need to use `c.Request.FormValue("field_name")` for each field and parse them.
	// For complex nested JSON like `BabysittingDetails`, they might need to be sent as JSON strings
	// in a form field and then unmarshalled.
	// For now, let's assume flat structure for simplicity of this step, or that client sends them as form values.
	// The `binding` tags in `CreateListingRequest` might need to be changed to `form:"..."` for this to work well.
	// I will proceed assuming `ShouldBindWith(&req, binding.FormMultipart)` can map the fields.

	if err := c.ShouldBindWith(&req, binding.FormMultipart); err != nil {
		h.logger.Warn("Create listing: Invalid form data", zap.Error(err), zap.String("userID", userID.String()))
		var ve validator.ValidationErrors
		if errors.As(err, &ve) {
			common.RespondWithError(c, common.NewValidationAPIError(common.FormatValidationErrors(ve)))
			return
		}
		common.RespondWithError(c, common.ErrBadRequest.WithDetails("Invalid form data: "+err.Error()))
		return
	}

	// Access uploaded files
	form := c.Request.MultipartForm
	images := form.File["images"] // "images" is the field name for file uploads

	// The service layer will handle the actual file saving and linking.
	// The request to service.CreateListing will need to be updated to accept images.
	listing, err := h.service.CreateListing(c.Request.Context(), userID, req, images)
	if err != nil {
		common.RespondWithError(c, err)
		return
	}
	common.RespondCreated(c, "Listing created successfully.", ToListingResponse(listing, true, h.cfg.ImagePublicBaseURL))
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
	common.RespondOK(c, "Listing retrieved successfully.", ToListingResponse(listing, isAuthenticatedForContact, h.cfg.ImagePublicBaseURL))
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
		listingResponses[i] = ToListingResponse(&l, isAuthenticatedForContact, h.cfg.ImagePublicBaseURL)
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
		listingResponses[i] = ToListingResponse(&l, true, h.cfg.ImagePublicBaseURL)
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

	if err := c.Request.ParseMultipartForm(50 << 20); err != nil { // Same memory limit as create
		h.logger.Warn("Update listing: Failed to parse multipart form", zap.Error(err), zap.String("listingID", listingID.String()))
		common.RespondWithError(c, common.ErrBadRequest.WithDetails("Invalid request format or files too large: "+err.Error()))
		return
	}

	var req UpdateListingRequest
	// Similar to createListing, binding form data.
	// The `RemoveImageIDs` field, if sent as a JSON array string in a form field, will need custom unmarshalling.
	// For now, let's assume `ShouldBindWith` can handle it or it's sent as multiple form values for the array.
	// If `RemoveImageIDs` is sent like `remove_image_ids=id1&remove_image_ids=id2`, Gin can bind it to a slice.
	if err := c.ShouldBindWith(&req, binding.FormMultipart); err != nil {
		h.logger.Warn("Update listing: Invalid form data", zap.Error(err), zap.String("listingID", listingID.String()))
		var ve validator.ValidationErrors
		if errors.As(err, &ve) {
			common.RespondWithError(c, common.NewValidationAPIError(common.FormatValidationErrors(ve)))
			return
		}
		common.RespondWithError(c, common.ErrBadRequest.WithDetails("Invalid form data: "+err.Error()))
		return
	}

	// Access newly uploaded files
	form := c.Request.MultipartForm
	newImages := form.File["images"] // Field name for new images

	// The service layer will handle updating, adding new images, and removing specified old images.
	listing, err := h.service.UpdateListing(c.Request.Context(), listingID, userID, req, newImages)
	if err != nil {
		common.RespondWithError(c, err)
		return
	}
	common.RespondOK(c, "Listing updated successfully.", ToListingResponse(listing, true, h.cfg.ImagePublicBaseURL))
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
	common.RespondOK(c, "Admin: Listing retrieved successfully.", ToListingResponse(listing, true, h.cfg.ImagePublicBaseURL))
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
	common.RespondOK(c, "Admin: Listing status updated successfully.", ToListingResponse(listing, true, h.cfg.ImagePublicBaseURL))
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
	common.RespondOK(c, "Admin: Listing approved successfully.", ToListingResponse(listing, true, h.cfg.ImagePublicBaseURL))
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
