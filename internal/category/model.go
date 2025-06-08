// File: internal/category/model.go
package category

import (
	"seattle_info_backend/internal/common"
	"time"

	"github.com/google/uuid"
)

// Category represents the category model in the database.
type Category struct {
	common.BaseModel
	Name             string        `gorm:"type:varchar(100);not null;uniqueIndex:idx_categories_name,unique"`
	Slug             string        `gorm:"type:varchar(100);not null;uniqueIndex:idx_categories_slug,unique"`
	Description      *string       `gorm:"type:text"`
	SubCategories    []SubCategory `gorm:"foreignKey:CategoryID;constraint:OnDelete:CASCADE;"`
	SubCategoryCount int           `gorm:"column:sub_category_count;->"` // read-only, no writes
}

// TableName specifies the table name for the Category model.
func (Category) TableName() string {
	return "categories"
}

// SubCategory represents the sub_category model in the database.
type SubCategory struct {
	common.BaseModel
	CategoryID  uuid.UUID `gorm:"type:uuid;not null"`
	Category    Category  `gorm:"foreignKey:CategoryID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	Name        string    `gorm:"type:varchar(100);not null;uniqueIndex:idx_sub_categories_category_id_name,unique,composite:unique_name_in_category"`
	Slug        string    `gorm:"type:varchar(100);not null;uniqueIndex:idx_sub_categories_category_id_slug,unique,composite:unique_slug_in_category"`
	Description *string   `gorm:"type:text"`
}

// TableName specifies the table name for the SubCategory model.
func (SubCategory) TableName() string {
	return "sub_categories"
}

// --- DTOs ---

// CategoryResponse defines the structure for category data sent in API responses.
type CategoryResponse struct {
	ID               uuid.UUID             `json:"id"`
	Name             string                `json:"name"`
	Slug             string                `json:"slug"`
	Description      *string               `json:"description,omitempty"`
	SubCategoryCount int                   `json:"sub_category_count"`
	SubCategories    []SubCategoryResponse `json:"sub_categories,omitempty"`
	CreatedAt        time.Time             `json:"created_at"`
	UpdatedAt        time.Time             `json:"updated_at"`
}

// SubCategoryResponse defines the structure for sub_category data.
type SubCategoryResponse struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	Description *string   `json:"description,omitempty"`
	CategoryID  uuid.UUID `json:"category_id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ToCategoryResponse converts a Category model to a CategoryResponse DTO.
func ToCategoryResponse(category *Category) CategoryResponse {
	subCategoryDTOs := make([]SubCategoryResponse, len(category.SubCategories))
	for i, sc := range category.SubCategories {
		subCategoryDTOs[i] = ToSubCategoryResponse(&sc)
	}
	return CategoryResponse{
		ID:               category.ID,
		Name:             category.Name,
		Slug:             category.Slug,
		Description:      category.Description,
		SubCategoryCount: category.SubCategoryCount,
		SubCategories:    subCategoryDTOs,
		CreatedAt:        category.CreatedAt,
		UpdatedAt:        category.UpdatedAt,
	}
}

// ToSubCategoryResponse converts a SubCategory model to a SubCategoryResponse DTO.
func ToSubCategoryResponse(subCategory *SubCategory) SubCategoryResponse {
	return SubCategoryResponse{
		ID:          subCategory.ID,
		Name:        subCategory.Name,
		Slug:        subCategory.Slug,
		Description: subCategory.Description,
		CategoryID:  subCategory.CategoryID,
		CreatedAt:   subCategory.CreatedAt,
		UpdatedAt:   subCategory.UpdatedAt,
	}
}

// AdminCreateCategoryRequest for admin creating categories
type AdminCreateCategoryRequest struct {
	Name        string  `json:"name" binding:"required,max=100"`
	Slug        string  `json:"slug" binding:"required,max=100,alphanumdash"`
	Description *string `json:"description,omitempty"`
}

// AdminCreateSubCategoryRequest for admin creating subcategories
type AdminCreateSubCategoryRequest struct {
	Name        string  `json:"name" binding:"required,max=100"`
	Slug        string  `json:"slug" binding:"required,max=100,alphanumdash"`
	Description *string `json:"description,omitempty"`
}
