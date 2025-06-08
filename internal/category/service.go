// File: internal/category/service.go
package category

import (
	"context"
	"fmt"
	"strings"

	"seattle_info_backend/internal/common"
	"seattle_info_backend/internal/config"

	"github.com/google/uuid"
	"github.com/gosimple/slug" // For robust slug generation
	"go.uber.org/zap"
)

// Service defines the interface for category-related business logic.
type Service interface {
	// Admin methods
	AdminCreateCategory(ctx context.Context, req AdminCreateCategoryRequest) (*Category, error)
	AdminCreateSubCategory(ctx context.Context, categoryID uuid.UUID, req AdminCreateSubCategoryRequest) (*SubCategory, error)
	AdminUpdateCategory(ctx context.Context, id uuid.UUID, req AdminCreateCategoryRequest) (*Category, error)
	AdminUpdateSubCategory(ctx context.Context, id uuid.UUID, req AdminCreateSubCategoryRequest) (*SubCategory, error)
	AdminDeleteCategory(ctx context.Context, id uuid.UUID) error
	AdminDeleteSubCategory(ctx context.Context, id uuid.UUID) error

	// Public methods
	GetCategoryByID(ctx context.Context, id uuid.UUID, preloadSubcategories bool) (*Category, error)
	GetCategoryBySlug(ctx context.Context, slug string, preloadSubcategories bool) (*Category, error)
	GetAllCategories(ctx context.Context, preloadSubcategories bool) ([]Category, error)
	GetSubCategoryByID(ctx context.Context, id uuid.UUID) (*SubCategory, error)
}

// ServiceImplementation implements the category Service interface.
type ServiceImplementation struct {
	repo   Repository
	logger *zap.Logger
	config *config.Config // If needed for category-specific configs
}

// NewService creates a new category service.
func NewService(repo Repository, logger *zap.Logger, cfg *config.Config) Service {
	return &ServiceImplementation{
		repo:   repo,
		logger: logger,
		config: cfg,
	}
}

// --- Admin Methods ---

// AdminCreateCategory creates a new category.
func (s *ServiceImplementation) AdminCreateCategory(ctx context.Context, req AdminCreateCategoryRequest) (*Category, error) {
	finalSlug := strings.TrimSpace(req.Slug)
	if finalSlug == "" {
		finalSlug = slug.Make(req.Name) // Generate slug if not provided
	} else {
		finalSlug = slug.Make(finalSlug) // Ensure provided slug is clean
	}

	category := &Category{
		Name:        strings.TrimSpace(req.Name),
		Slug:        finalSlug,
		Description: req.Description,
	}

	if err := s.repo.CreateCategory(ctx, category); err != nil {
		s.logger.Error("Failed to create category", zap.Error(err), zap.String("name", req.Name))
		return nil, err // Repo should return specific common.APIError
	}
	s.logger.Info("Category created successfully", zap.String("id", category.ID.String()), zap.String("name", category.Name))
	return category, nil
}

// AdminCreateSubCategory creates a new subcategory under a given parent category.
func (s *ServiceImplementation) AdminCreateSubCategory(ctx context.Context, categoryID uuid.UUID, req AdminCreateSubCategoryRequest) (*SubCategory, error) {
	// Check if parent category exists
	_, err := s.repo.FindCategoryByID(ctx, categoryID, false)
	if err != nil {
		s.logger.Warn("Parent category not found for subcategory creation", zap.String("categoryID", categoryID.String()))
		return nil, common.ErrBadRequest.WithDetails(fmt.Sprintf("Parent category with ID %s not found.", categoryID))
	}

	finalSlug := strings.TrimSpace(req.Slug)
	if finalSlug == "" {
		finalSlug = slug.Make(req.Name)
	} else {
		finalSlug = slug.Make(finalSlug)
	}

	subCategory := &SubCategory{
		CategoryID:  categoryID,
		Name:        strings.TrimSpace(req.Name),
		Slug:        finalSlug,
		Description: req.Description,
	}

	if err := s.repo.CreateSubCategory(ctx, subCategory); err != nil {
		s.logger.Error("Failed to create subcategory", zap.Error(err),
			zap.String("name", req.Name), zap.String("parentCategoryID", categoryID.String()))
		return nil, err
	}
	s.logger.Info("SubCategory created successfully", zap.String("id", subCategory.ID.String()), zap.String("name", subCategory.Name))
	return subCategory, nil
}

// AdminUpdateCategory updates an existing category.
func (s *ServiceImplementation) AdminUpdateCategory(ctx context.Context, id uuid.UUID, req AdminCreateCategoryRequest) (*Category, error) {
	category, err := s.repo.FindCategoryByID(ctx, id, false)
	if err != nil {
		return nil, err // ErrNotFound or other DB error
	}

	category.Name = strings.TrimSpace(req.Name)
	if req.Slug != "" {
		category.Slug = slug.Make(req.Slug)
	} else {
		category.Slug = slug.Make(req.Name) // Regenerate slug if slug field is empty, based on new name
	}
	category.Description = req.Description

	if err := s.repo.UpdateCategory(ctx, category); err != nil {
		s.logger.Error("Failed to update category", zap.Error(err), zap.String("id", id.String()))
		return nil, err
	}
	s.logger.Info("Category updated successfully", zap.String("id", category.ID.String()))
	return category, nil
}

// AdminUpdateSubCategory updates an existing subcategory.
func (s *ServiceImplementation) AdminUpdateSubCategory(ctx context.Context, id uuid.UUID, req AdminCreateSubCategoryRequest) (*SubCategory, error) {
	subCategory, err := s.repo.FindSubCategoryByID(ctx, id)
	if err != nil {
		return nil, err
	}

	subCategory.Name = strings.TrimSpace(req.Name)
	if req.Slug != "" {
		subCategory.Slug = slug.Make(req.Slug)
	} else {
		subCategory.Slug = slug.Make(req.Name)
	}
	subCategory.Description = req.Description

	if err := s.repo.UpdateSubCategory(ctx, subCategory); err != nil {
		s.logger.Error("Failed to update subcategory", zap.Error(err), zap.String("id", id.String()))
		return nil, err
	}
	s.logger.Info("SubCategory updated successfully", zap.String("id", subCategory.ID.String()))
	return subCategory, nil
}

// AdminDeleteCategory deletes a category by its ID.
func (s *ServiceImplementation) AdminDeleteCategory(ctx context.Context, id uuid.UUID) error {
	if err := s.repo.DeleteCategory(ctx, id); err != nil {
		s.logger.Error("Failed to delete category", zap.Error(err), zap.String("id", id.String()))
		return err
	}
	s.logger.Info("Category deleted successfully", zap.String("id", id.String()))
	return nil
}

// AdminDeleteSubCategory deletes a subcategory by its ID.
func (s *ServiceImplementation) AdminDeleteSubCategory(ctx context.Context, id uuid.UUID) error {
	if err := s.repo.DeleteSubCategory(ctx, id); err != nil {
		s.logger.Error("Failed to delete subcategory", zap.Error(err), zap.String("id", id.String()))
		return err
	}
	s.logger.Info("SubCategory deleted successfully", zap.String("id", id.String()))
	return nil
}

// --- Public Methods ---

// GetCategoryByID retrieves a category by its ID.
func (s *ServiceImplementation) GetCategoryByID(ctx context.Context, id uuid.UUID, preloadSubcategories bool) (*Category, error) {
	category, err := s.repo.FindCategoryByID(ctx, id, preloadSubcategories)
	if err != nil {
		return nil, err
	}
	return category, nil
}

// GetCategoryBySlug retrieves a category by its slug.
func (s *ServiceImplementation) GetCategoryBySlug(ctx context.Context, slugToFind string, preloadSubcategories bool) (*Category, error) {
	category, err := s.repo.FindCategoryBySlug(ctx, slugToFind, preloadSubcategories)
	if err != nil {
		return nil, err
	}
	return category, nil
}

// GetAllCategories retrieves all categories, optionally preloading subcategories.
func (s *ServiceImplementation) GetAllCategories(ctx context.Context, preloadSubcategories bool) ([]Category, error) {
	categories, err := s.repo.FindAllCategories(ctx, preloadSubcategories)
	if err != nil {
		s.logger.Error("Failed to get all categories", zap.Error(err))
		return nil, common.ErrInternalServer.WithDetails("Could not retrieve categories.")
	}
	return categories, nil
}

// GetSubCategoryByID retrieves a subcategory by its ID.
func (s *ServiceImplementation) GetSubCategoryByID(ctx context.Context, id uuid.UUID) (*SubCategory, error) {
	subCategory, err := s.repo.FindSubCategoryByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return subCategory, nil
}
