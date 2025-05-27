// File: internal/category/handler.go
package category

import (
	"errors"
	"seattle_info_backend/internal/common"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Handler struct holds dependencies for category handlers.
type Handler struct {
	service Service // Depends on category.Service
	logger  *zap.Logger
}

// NewHandler creates a new category handler.
// It does NOT take auth.TokenService.
func NewHandler(service Service, logger *zap.Logger) *Handler {
	return &Handler{
		service: service,
		logger:  logger,
	}
}

// RegisterRoutes sets up the routes for category operations.
// It takes auth and admin middleware functions as parameters.
func (h *Handler) RegisterRoutes(router *gin.RouterGroup, authMW gin.HandlerFunc, adminRoleMW gin.HandlerFunc) {
	categoryGroup := router.Group("/categories")
	{
		categoryGroup.GET("", h.getAllCategories)
		categoryGroup.GET("/:idOrSlug", h.getCategory)

		adminCategoryGroup := categoryGroup.Group("/admin")
		adminCategoryGroup.Use(authMW)
		adminCategoryGroup.Use(adminRoleMW)
		{
			adminCategoryGroup.POST("", h.adminCreateCategory)
			adminCategoryGroup.PUT("/:id", h.adminUpdateCategory)
			adminCategoryGroup.DELETE("/:id", h.adminDeleteCategory)
			adminCategoryGroup.POST("/:categoryId/subcategories", h.adminCreateSubCategory)
		}
	}
	subCategoryAdminGroup := router.Group("/subcategories/admin")
	subCategoryAdminGroup.Use(authMW)
	subCategoryAdminGroup.Use(adminRoleMW)
	{
		subCategoryAdminGroup.PUT("/:id", h.adminUpdateSubCategory)
		subCategoryAdminGroup.DELETE("/:id", h.adminDeleteSubCategory)
	}
}

func (h *Handler) getAllCategories(c *gin.Context) {
	preloadSubcategories := c.Query("include_subcategories") == "true"
	categories, err := h.service.GetAllCategories(c.Request.Context(), preloadSubcategories)
	if err != nil {
		common.RespondWithError(c, err)
		return
	}
	categoryResponses := make([]CategoryResponse, len(categories))
	for i, cat := range categories {
		categoryResponses[i] = ToCategoryResponse(&cat)
	}
	common.RespondOK(c, "Categories retrieved successfully.", categoryResponses)
}

func (h *Handler) getCategory(c *gin.Context) {
	idOrSlug := c.Param("idOrSlug")
	preloadSubcategories := c.Query("include_subcategories") == "true"
	var catModel *Category // Changed from category to catModel to avoid conflict
	var err error
	catID, parseErr := uuid.Parse(idOrSlug)
	if parseErr == nil {
		catModel, err = h.service.GetCategoryByID(c.Request.Context(), catID, preloadSubcategories)
	} else {
		catModel, err = h.service.GetCategoryBySlug(c.Request.Context(), idOrSlug, preloadSubcategories)
	}
	if err != nil {
		common.RespondWithError(c, err)
		return
	}
	common.RespondOK(c, "Category retrieved successfully.", ToCategoryResponse(catModel))
}

func (h *Handler) adminCreateCategory(c *gin.Context) {
	var req AdminCreateCategoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("Admin create category: Invalid request body", zap.Error(err))
		var ve validator.ValidationErrors
		if errors.As(err, &ve) {
			common.RespondWithError(c, common.NewValidationAPIError(common.FormatValidationErrors(ve)))
			return
		}
		common.RespondWithError(c, common.ErrBadRequest.WithDetails(err.Error()))
		return
	}
	catModel, err := h.service.AdminCreateCategory(c.Request.Context(), req)
	if err != nil {
		common.RespondWithError(c, err)
		return
	}
	common.RespondCreated(c, "Category created successfully.", ToCategoryResponse(catModel))
}

func (h *Handler) adminUpdateCategory(c *gin.Context) {
	categoryID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		common.RespondWithError(c, common.ErrBadRequest.WithDetails("Invalid category ID format."))
		return
	}
	var req AdminCreateCategoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("Admin update category: Invalid request body", zap.Error(err), zap.String("categoryID", categoryID.String()))
		var ve validator.ValidationErrors
		if errors.As(err, &ve) {
			common.RespondWithError(c, common.NewValidationAPIError(common.FormatValidationErrors(ve)))
			return
		}
		common.RespondWithError(c, common.ErrBadRequest.WithDetails(err.Error()))
		return
	}
	catModel, err := h.service.AdminUpdateCategory(c.Request.Context(), categoryID, req)
	if err != nil {
		common.RespondWithError(c, err)
		return
	}
	common.RespondOK(c, "Category updated successfully.", ToCategoryResponse(catModel))
}

func (h *Handler) adminDeleteCategory(c *gin.Context) {
	categoryID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		common.RespondWithError(c, common.ErrBadRequest.WithDetails("Invalid category ID format."))
		return
	}
	if err := h.service.AdminDeleteCategory(c.Request.Context(), categoryID); err != nil {
		common.RespondWithError(c, err)
		return
	}
	common.RespondNoContent(c)
}

func (h *Handler) adminCreateSubCategory(c *gin.Context) {
	categoryID, err := uuid.Parse(c.Param("categoryId"))
	if err != nil {
		common.RespondWithError(c, common.ErrBadRequest.WithDetails("Invalid parent category ID format."))
		return
	}
	var req AdminCreateSubCategoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("Admin create subcategory: Invalid request body", zap.Error(err), zap.String("categoryID", categoryID.String()))
		var ve validator.ValidationErrors
		if errors.As(err, &ve) {
			common.RespondWithError(c, common.NewValidationAPIError(common.FormatValidationErrors(ve)))
			return
		}
		common.RespondWithError(c, common.ErrBadRequest.WithDetails(err.Error()))
		return
	}
	subCatModel, err := h.service.AdminCreateSubCategory(c.Request.Context(), categoryID, req)
	if err != nil {
		common.RespondWithError(c, err)
		return
	}
	common.RespondCreated(c, "SubCategory created successfully.", ToSubCategoryResponse(subCatModel))
}

func (h *Handler) adminUpdateSubCategory(c *gin.Context) {
	subCategoryID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		common.RespondWithError(c, common.ErrBadRequest.WithDetails("Invalid subcategory ID format."))
		return
	}
	var req AdminCreateSubCategoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("Admin update subcategory: Invalid request body", zap.Error(err), zap.String("subCategoryID", subCategoryID.String()))
		var ve validator.ValidationErrors
		if errors.As(err, &ve) {
			common.RespondWithError(c, common.NewValidationAPIError(common.FormatValidationErrors(ve)))
			return
		}
		common.RespondWithError(c, common.ErrBadRequest.WithDetails(err.Error()))
		return
	}
	subCatModel, err := h.service.AdminUpdateSubCategory(c.Request.Context(), subCategoryID, req)
	if err != nil {
		common.RespondWithError(c, err)
		return
	}
	common.RespondOK(c, "SubCategory updated successfully.", ToSubCategoryResponse(subCatModel))
}

func (h *Handler) adminDeleteSubCategory(c *gin.Context) {
	subCategoryID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		common.RespondWithError(c, common.ErrBadRequest.WithDetails("Invalid subcategory ID format."))
		return
	}
	if err := h.service.AdminDeleteSubCategory(c.Request.Context(), subCategoryID); err != nil {
		common.RespondWithError(c, err)
		return
	}
	common.RespondNoContent(c)
}
