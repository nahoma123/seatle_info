// File: internal/common/model.go
package common

import (
	"time"

	"github.com/google/uuid"
	// "gorm.io/gorm" // Not strictly needed if BeforeCreate is commented out
)

// BaseModel defines common fields for GORM models.
type BaseModel struct {
	ID        uuid.UUID `gorm:"type:uuid;primary_key;default:uuid_generate_v4()"`
	CreatedAt time.Time `gorm:"column:created_at;not null;default:current_timestamp"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null;default:current_timestamp"`
}

// Pagination struct for paginated API responses
type Pagination struct {
	TotalItems  int64 `json:"total_items"`
	TotalPages  int   `json:"total_pages"`
	CurrentPage int   `json:"current_page"`
	PageSize    int   `json:"page_size"`
	HasNext     bool  `json:"has_next"`
	HasPrev     bool  `json:"has_prev"`
}

// NewPagination creates a pagination object.
func NewPagination(totalItems int64, page, pageSize int) *Pagination {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 10 // Default page size
	}

	totalPages := int((totalItems + int64(pageSize) - 1) / int64(pageSize))
	if totalPages == 0 && totalItems > 0 {
		totalPages = 1
	}
	if totalItems == 0 { // If no items, total pages should be 0 or 1 depending on preference
		totalPages = 0 // Or 1 if you always want at least one page in response
	}

	return &Pagination{
		TotalItems:  totalItems,
		TotalPages:  totalPages,
		CurrentPage: page,
		PageSize:    pageSize,
		HasNext:     page < totalPages,
		HasPrev:     page > 1,
	}
}
