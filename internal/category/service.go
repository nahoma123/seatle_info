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

type service struct {
	repo   Repository
	logger *zap.Logger
	config *config.Config // If needed for category-specific configs
}

// NewService creates a new category service.
func NewService(repo Repository, logger *zap.Logger, cfg *config.Config) Service {
	return &service{
		repo:   repo,
		logger: logger,
		config: cfg,
	}
}

// --- Admin Methods ---

func (s *service) AdminCreateCategory(ctx context.Context, req AdminCreateCategoryRequest) (*Category, error) {
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

func (s *service) AdminCreateSubCategory(ctx context.Context, categoryID uuid.UUID, req AdminCreateSubCategoryRequest) (*SubCategory, error) {
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

func (s *service) AdminUpdateCategory(ctx context.Context, id uuid.UUID, req AdminCreateCategoryRequest) (*Category, error) {
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

func (s *service) AdminUpdateSubCategory(ctx context.Context, id uuid.UUID, req AdminCreateSubCategoryRequest) (*SubCategory, error) {
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

func (s *service) AdminDeleteCategory(ctx context.Context, id uuid.UUID) error {
	// Repository DeleteCategory already checks for associated listings.
	if err := s.repo.DeleteCategory(ctx, id); err != nil {
		s.logger.Error("Failed to delete category", zap.Error(err), zap.String("id", id.String()))
		return err
	}
	s.logger.Info("Category deleted successfully", zap.String("id", id.String()))
	return nil
}

func (s *service) AdminDeleteSubCategory(ctx context.Context, id uuid.UUID) error {
	// Repository DeleteSubCategory handles associated listings (sets sub_category_id to NULL).
	if err := s.repo.DeleteSubCategory(ctx, id); err != nil {
		s.logger.Error("Failed to delete subcategory", zap.Error(err), zap.String("id", id.String()))
		return err
	}
	s.logger.Info("SubCategory deleted successfully", zap.String("id", id.String()))
	return nil
}

// --- Public Methods ---

func (s *service) GetCategoryByID(ctx context.Context, id uuid.UUID, preloadSubcategories bool) (*Category, error) {
	category, err := s.repo.FindCategoryByID(ctx, id, preloadSubcategories)
	if err != nil {
		// Repo returns common.ErrNotFound if not found
		return nil, err
	}
	return category, nil
}

func (s *service) GetCategoryBySlug(ctx context.Context, slugToFind string, preloadSubcategories bool) (*Category, error) {
	category, err := s.repo.FindCategoryBySlug(ctx, slugToFind, preloadSubcategories)
	if err != nil {
		return nil, err
	}
	return category, nil
}

func (s *service) GetAllCategories(ctx context.Context, preloadSubcategories bool) ([]Category, error) {
	categories, err := s.repo.FindAllCategories(ctx, preloadSubcategories)
	if err != nil {
		s.logger.Error("Failed to get all categories", zap.Error(err))
		// Don't return raw db error to client, wrap if necessary, but repo errors are often fine
		return nil, common.ErrInternalServer.WithDetails("Could not retrieve categories.")
	}
	return categories, nil
}

func (s *service) GetSubCategoryByID(ctx context.Context, id uuid.UUID) (*SubCategory, error) {
	subCategory, err := s.repo.FindSubCategoryByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return subCategory, nil
}
