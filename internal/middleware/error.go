// File: internal/middleware/error.go
package middleware

import (
	"seattle_info_backend/internal/common"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// ErrorHandler creates a Gin middleware for centralized error handling.
func ErrorHandler(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		if len(c.Errors) > 0 {
			for _, ginErr := range c.Errors {
				apiErr, isAPIErr := common.IsAPIError(ginErr.Err)

				if isAPIErr {
					c.AbortWithStatusJSON(apiErr.StatusCode, apiErr)
				} else {
					logger.Error("Unhandled application error",
						zap.Error(ginErr.Err),
						zap.String("path", c.Request.URL.Path),
						zap.Any("meta", ginErr.Meta),
						zap.String("request_id", c.GetString(RequestIDContextKey)), // USE EXPORTED CONSTANT
					)
					genericError := common.ErrInternalServer.WithDetails("An unexpected error occurred.")
					if gin.Mode() == gin.DebugMode && ginErr.Err != nil {
						genericError.Details = ginErr.Err.Error()
					}
					c.AbortWithStatusJSON(genericError.StatusCode, genericError)
				}
				return
			}
		}

		if c.Writer.Status() == 404 && len(c.Errors) == 0 {
			notFoundErr := common.ErrNotFound.WithDetails("The requested endpoint does not exist.")
			c.AbortWithStatusJSON(notFoundErr.StatusCode, notFoundErr)
			return
		}
		if c.Writer.Status() == 405 && len(c.Errors) == 0 {
			methodNotAllowedErr := common.NewAPIError(405, "METHOD_NOT_ALLOWED", "The method is not allowed for the requested URL.")
			c.AbortWithStatusJSON(methodNotAllowedErr.StatusCode, methodNotAllowedErr)
			return
		}
	}
}
