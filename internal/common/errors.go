// File: internal/common/errors.go
package common

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	// Ensure this is the correct import used by Gin for binding
	"github.com/go-playground/validator/v10"
)

// APIError represents a standard structure for API errors.
type APIError struct {
	StatusCode int         `json:"-"`
	Code       string      `json:"code"`
	Message    string      `json:"message"`
	Details    interface{} `json:"details,omitempty"`
}

func (e *APIError) Error() string {
	return fmt.Sprintf("APIError: StatusCode=%d, Code=%s, Message=%s", e.StatusCode, e.Code, e.Message)
}

func NewAPIError(statusCode int, code, message string) *APIError {
	return &APIError{StatusCode: statusCode, Code: code, Message: message}
}

func (e *APIError) WithDetails(details interface{}) *APIError {
	e.Details = details
	return e
}

var (
	ErrBadRequest          = NewAPIError(http.StatusBadRequest, "BAD_REQUEST", "The request is invalid.")
	ErrUnauthorized        = NewAPIError(http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required and has failed or has not yet been provided.")
	ErrForbidden           = NewAPIError(http.StatusForbidden, "FORBIDDEN", "You do not have permission to access this resource.")
	ErrNotFound            = NewAPIError(http.StatusNotFound, "NOT_FOUND", "The requested resource could not be found.")
	ErrConflict            = NewAPIError(http.StatusConflict, "CONFLICT", "A conflict occurred with the current state of the resource.")
	ErrUnprocessableEntity = NewAPIError(http.StatusUnprocessableEntity, "UNPROCESSABLE_ENTITY", "The request was well-formed but was unable to be followed due to semantic errors.")
	ErrInternalServer      = NewAPIError(http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "An unexpected error occurred on the server.")
	ErrServiceUnavailable  = NewAPIError(http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "The server is currently unable to handle the request.")
)

func IsAPIError(err error) (*APIError, bool) {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr, true
	}
	return nil, false
}

func NewValidationAPIError(details interface{}) *APIError {
	return &APIError{
		StatusCode: http.StatusUnprocessableEntity,
		Code:       "VALIDATION_ERROR",
		Message:    "Input validation failed.",
		Details:    details,
	}
}

// FormatValidationErrors converts validator.ValidationErrors into a map.
// Make sure the import for validator.ValidationErrors is "github.com/go-playground/validator/v10"
func FormatValidationErrors(errs validator.ValidationErrors) map[string]string {
	errorMap := make(map[string]string)
	for _, e := range errs {
		field := e.Field()
		var message string
		switch e.Tag() {
		case "required":
			message = fmt.Sprintf("The %s field is required.", strings.ToLower(field))
		case "email":
			message = fmt.Sprintf("The %s field must be a valid email address.", strings.ToLower(field))
		case "min":
			message = fmt.Sprintf("The %s field must be at least %s characters long.", strings.ToLower(field), e.Param())
		case "max":
			message = fmt.Sprintf("The %s field may not be greater than %s characters.", strings.ToLower(field), e.Param())
		case "alphanumdash":
			message = fmt.Sprintf("The %s field may only contain alphanumeric characters and dashes.", strings.ToLower(field))
		case "oneof":
			message = fmt.Sprintf("The %s field must be one of the following values: %s.", strings.ToLower(field), e.Param())
		case "latitude":
			message = fmt.Sprintf("The %s field must be a valid latitude.", strings.ToLower(field))
		case "longitude":
			message = fmt.Sprintf("The %s field must be a valid longitude.", strings.ToLower(field))
		case "datetime":
			message = fmt.Sprintf("The %s field must be a valid datetime in the format %s.", strings.ToLower(field), e.Param())
		default:
			message = fmt.Sprintf("Field validation for '%s' failed on the '%s' tag.", field, e.Tag())
		}
		errorMap[field] = message
	}
	return errorMap
}
