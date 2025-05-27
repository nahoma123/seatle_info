// File: internal/common/response.go
package common

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap" // Ensure zap is imported if logger is used directly here
)

// SuccessResponse wraps successful API responses.
type SuccessResponse struct {
	Status  string      `json:"status"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

// RespondWithError sends a JSON error response.
func RespondWithError(c *gin.Context, err error) {
	apiErr, ok := IsAPIError(err) // This function must be defined in common/errors.go
	if !ok {
		// If logger is guaranteed to be in context (e.g., from middleware)
		if l, exists := c.Get("logger"); exists {
			if logger, ok := l.(*zap.Logger); ok {
				logger.Error("Unhandled internal error being wrapped", zap.Error(err))
			}
		}
		// Wrap it as a generic internal server error
		apiErr = ErrInternalServer.WithDetails(err.Error()) // ErrInternalServer must be defined in common/errors.go
	}

	c.AbortWithStatusJSON(apiErr.StatusCode, apiErr)
}

// RespondSuccess sends a JSON success response.
func RespondSuccess(c *gin.Context, statusCode int, message string, data interface{}) {
	response := SuccessResponse{
		Status:  "success",
		Message: message,
		Data:    data,
	}
	c.JSON(statusCode, response)
}

// RespondOK sends a 200 OK response.
func RespondOK(c *gin.Context, message string, data interface{}) {
	RespondSuccess(c, http.StatusOK, message, data)
}

// RespondCreated sends a 201 Created response.
func RespondCreated(c *gin.Context, message string, data interface{}) {
	RespondSuccess(c, http.StatusCreated, message, data)
}

// RespondNoContent sends a 204 No Content response.
func RespondNoContent(c *gin.Context) {
	c.Status(http.StatusNoContent)
}

// PaginatedResponse structure for paginated data
type PaginatedResponse struct {
	Status     string      `json:"status"`
	Message    string      `json:"message,omitempty"`
	Data       interface{} `json:"data"`
	Pagination *Pagination `json:"pagination"` // Pagination must be defined in common/model.go or common/pagination.go
}

// RespondPaginated sends a JSON response for paginated data.
func RespondPaginated(c *gin.Context, message string, data interface{}, pagination *Pagination) {
	response := PaginatedResponse{
		Status:     "success",
		Message:    message,
		Data:       data,
		Pagination: pagination,
	}
	c.JSON(http.StatusOK, response)
}
