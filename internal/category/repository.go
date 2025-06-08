// File: internal/category/repository.go
package category

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"seattle_info_backend/internal/common"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Repository defines the interface for category and subcategory data operations.
type Repository interface {
	// Category methods
	CreateCategory(ctx context.Context, category *Category) error
	FindCategoryByID(ctx context.Context, id uuid.UUID, preloadSubcategories bool) (*Category, error)
	FindCategoryBySlug(ctx context.Context, slug string, preloadSubcategories bool) (*Category, error)
	FindAllCategories(ctx context.Context, preloadSubcategories bool) ([]Category, error)
	UpdateCategory(ctx context.Context, category *Category) error
	DeleteCategory(ctx context.Context, id uuid.UUID) error // Deletion might cascade to subcategories

	// SubCategory methods
	CreateSubCategory(ctx context.Context, subCategory *SubCategory) error
	FindSubCategoryByID(ctx context.Context, id uuid.UUID) (*SubCategory, error)
	FindSubCategoriesByCategoryID(ctx context.Context, categoryID uuid.UUID) ([]SubCategory, error)
	UpdateSubCategory(ctx context.Context, subCategory *SubCategory) error
	DeleteSubCategory(ctx context.Context, id uuid.UUID) error
}

// GORMRepository implements the Repository interface using GORM.
type GORMRepository struct {
	db *gorm.DB
}

// NewGORMRepository creates a new GORM category repository.
func NewGORMRepository(db *gorm.DB) Repository {
	return &GORMRepository{db: db}
}

// --- Category Methods ---

// CreateCategory creates a new category.
func (r *GORMRepository) CreateCategory(ctx context.Context, category *Category) error {
	category.Slug = strings.ToLower(strings.TrimSpace(category.Slug)) // Normalize slug
	err := r.db.WithContext(ctx).Create(category).Error
	if err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) || strings.Contains(err.Error(), "unique constraint") {
			return common.ErrConflict.WithDetails("Category with this name or slug already exists.")
		}
		return err
	}
	return nil
}

// FindCategoryByID finds a category by its ID.
func (r *GORMRepository) FindCategoryByID(ctx context.Context, id uuid.UUID, preloadSubcategories bool) (*Category, error) {
	var category Category
	query := r.db.WithContext(ctx)
	if preloadSubcategories {
		query = query.Preload("SubCategories")
	}
	err := query.First(&category, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, common.ErrNotFound.WithDetails("Category not found.")
		}
		return nil, err
	}
	return &category, nil
}

// FindCategoryBySlug finds a category by its slug.
func (r *GORMRepository) FindCategoryBySlug(ctx context.Context, slug string, preloadSubcategories bool) (*Category, error) {
	var category Category
	normalizedSlug := strings.ToLower(strings.TrimSpace(slug))
	query := r.db.WithContext(ctx)
	if preloadSubcategories {
		query = query.Preload("SubCategories")
	}
	err := query.First(&category, "slug = ?", normalizedSlug).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, common.ErrNotFound.WithDetails("Category not found.")
		}
		return nil, err
	}
	return &category, nil
}

// FindAllCategories retrieves all categories, optionally preloading their subcategories.
func (r *GORMRepository) FindAllCategories(ctx context.Context, preloadSubcategories bool) ([]Category, error) {
	var categories []Category
	query := r.db.WithContext(ctx).Model(&Category{})

	subQuery := r.db.Model(&SubCategory{}).
		Select("count(*)").
		Where("sub_categories.category_id = categories.id")

	query = query.Select("categories.*, (?) as sub_category_count", subQuery)

	if preloadSubcategories {
		query = query.Preload("SubCategories", func(db *gorm.DB) *gorm.DB {
			return db.Order("sub_categories.name ASC")
		})
	}

	err := query.Order("categories.name ASC").Find(&categories).Error
	if err != nil {
		return nil, err
	}
	return categories, nil

}

// UpdateCategory updates an existing category.
func (r *GORMRepository) UpdateCategory(ctx context.Context, category *Category) error {
	if category.Slug != "" {
		category.Slug = strings.ToLower(strings.TrimSpace(category.Slug)) // Normalize slug
	}
	err := r.db.WithContext(ctx).Save(category).Error
	if err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) || strings.Contains(err.Error(), "unique constraint") {
			return common.ErrConflict.WithDetails("Category with this name or slug already exists.")
		}
		return err
	}
	return nil
}

// DeleteCategory deletes a category by ID, ensuring no listings are associated.
func (r *GORMRepository) DeleteCategory(ctx context.Context, id uuid.UUID) error {
	// The migration `000001_create_initial_tables.up.sql` has `ON DELETE CASCADE` for sub_categories.
	// So, this should be fine.
	// We also need to consider listings associated with this category. The FK is ON DELETE RESTRICT.
	// So, a category cannot be deleted if it has active listings.
	// We must check for listings first.
	var listingCount int64
	if err := r.db.WithContext(ctx).Table("listings").Where("category_id = ?", id).Count(&listingCount).Error; err != nil {
		return common.ErrInternalServer.WithDetails("Failed to check for associated listings.")
	}
	if listingCount > 0 {
		return common.ErrConflict.WithDetails(
			fmt.Sprintf("Cannot delete category: %d listings are still associated with it.", listingCount),
		)
	}

	// If no listings, proceed to delete category (subcategories will cascade delete due to DB constraint)
	result := r.db.WithContext(ctx).Select(clause.Associations).Delete(&Category{BaseModel: common.BaseModel{ID: id}})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return common.ErrNotFound.WithDetails("Category not found or already deleted.")
	}
	return nil
}

// --- SubCategory Methods ---

// CreateSubCategory creates a new subcategory.
func (r *GORMRepository) CreateSubCategory(ctx context.Context, subCategory *SubCategory) error {
	subCategory.Slug = strings.ToLower(strings.TrimSpace(subCategory.Slug)) // Normalize slug
	err := r.db.WithContext(ctx).Create(subCategory).Error
	if err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) || strings.Contains(err.Error(), "unique constraint") {
			return common.ErrConflict.WithDetails("SubCategory with this name or slug already exists within the parent category.")
		}
		return err
	}
	return nil
}

// FindSubCategoryByID finds a subcategory by its ID.
func (r *GORMRepository) FindSubCategoryByID(ctx context.Context, id uuid.UUID) (*SubCategory, error) {
	var subCategory SubCategory
	err := r.db.WithContext(ctx).Preload("Category").First(&subCategory, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, common.ErrNotFound.WithDetails("SubCategory not found.")
		}
		return nil, err
	}
	return &subCategory, nil
}

// FindSubCategoriesByCategoryID finds all subcategories for a given category ID.
func (r *GORMRepository) FindSubCategoriesByCategoryID(ctx context.Context, categoryID uuid.UUID) ([]SubCategory, error) {
	var subCategories []SubCategory
	err := r.db.WithContext(ctx).Where("category_id = ?", categoryID).Order("name ASC").Find(&subCategories).Error
	return subCategories, err
}

// UpdateSubCategory updates an existing subcategory.
func (r *GORMRepository) UpdateSubCategory(ctx context.Context, subCategory *SubCategory) error {
	if subCategory.Slug != "" {
		subCategory.Slug = strings.ToLower(strings.TrimSpace(subCategory.Slug)) // Normalize slug
	}
	err := r.db.WithContext(ctx).Save(subCategory).Error
	if err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) || strings.Contains(err.Error(), "unique constraint") {
			return common.ErrConflict.WithDetails("SubCategory with this name or slug already exists within the parent category.")
		}
		return err
	}
	return nil
}

// DeleteSubCategory deletes a subcategory by ID.
func (r *GORMRepository) DeleteSubCategory(ctx context.Context, id uuid.UUID) error {
	// Check for associated listings with this subcategory
	// The FK on listings for sub_category_id is ON DELETE SET NULL, so this is safe to delete
	// from a referential integrity perspective for listings. Listings will just lose their sub_category_id.
	var listingCount int64
	if err := r.db.WithContext(ctx).Table("listings").Where("sub_category_id = ?", id).Count(&listingCount).Error; err != nil {
		// Log this error but don't necessarily block deletion because of ON DELETE SET NULL
		// unless business logic dictates otherwise. For now, proceed.
		fmt.Printf("Warning: could not check for listings associated with subcategory %s: %v\n", id, err)
	}
	// if listingCount > 0 {
	// 	// This would be if ON DELETE RESTRICT was used for subcategories in listings table.
	// 	return common.ErrConflict.WithDetails(
	// 		fmt.Sprintf("Cannot delete subcategory: %d listings are still associated with it.", listingCount),
	// 	)
	// }

	result := r.db.WithContext(ctx).Delete(&SubCategory{BaseModel: common.BaseModel{ID: id}})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return common.ErrNotFound.WithDetails("SubCategory not found or already deleted.")
	}
	return nil
}
