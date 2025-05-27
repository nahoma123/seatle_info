// File: internal/common/pagination.go
package common

import (
	"strconv"

	"github.com/gin-gonic/gin"
)

const (
	DefaultPage     = 1
	DefaultPageSize = 10
	MaxPageSize     = 100
)

// Pagination struct for paginated API responses (already defined in common/model.go)
// type Pagination struct { ... } from common/model.go

// PaginationQuery holds pagination parameters from request query.
type PaginationQuery struct {
	Page     int `form:"page"`
	PageSize int `form:"page_size"`
}

// GetPaginationParams extracts pagination parameters from Gin context.
func GetPaginationParams(c *gin.Context) (page, pageSize int) {
	page, err := strconv.Atoi(c.DefaultQuery("page", strconv.Itoa(DefaultPage)))
	if err != nil || page <= 0 {
		page = DefaultPage
	}

	pageSize, err = strconv.Atoi(c.DefaultQuery("page_size", strconv.Itoa(DefaultPageSize)))
	if err != nil || pageSize <= 0 {
		pageSize = DefaultPageSize
	}
	if pageSize > MaxPageSize {
		pageSize = MaxPageSize
	}
	return page, pageSize
}

// Offset calculates the offset for database queries.
func (pq *PaginationQuery) Offset() int {
	if pq.Page <= 0 {
		pq.Page = DefaultPage
	}
	return (pq.Page - 1) * pq.Limit()
}

// Limit calculates the limit for database queries.
func (pq *PaginationQuery) Limit() int {
	if pq.PageSize <= 0 {
		pq.PageSize = DefaultPageSize
	}
	if pq.PageSize > MaxPageSize {
		pq.PageSize = MaxPageSize
	}
	return pq.PageSize
}
