// File: internal/middleware/logger.go
package middleware

import (
	"seattle_info_backend/internal/config" // For config.Config if needed for logger settings
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	// RequestIDHeader is the header name for request ID
	RequestIDHeader = "X-Request-ID"
	// RequestIDContextKey is the key for storing request ID in Gin context (EXPORTED)
	RequestIDContextKey = "requestID" // WAS: requestIDContextKey
)

// ZapLogger is a Gin middleware that logs requests using Zap.
func ZapLogger(logger *zap.Logger, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		requestID := c.GetHeader(RequestIDHeader)
		if requestID == "" {
			requestID = uuid.NewString()
			c.Header(RequestIDHeader, requestID)
		}
		c.Set(RequestIDContextKey, requestID) // Use exported constant

		c.Next()

		end := time.Now()
		latency := end.Sub(start)
		statusCode := c.Writer.Status()

		fields := []zapcore.Field{
			zap.Int("status_code", statusCode),
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.String("query", query),
			zap.String("ip", c.ClientIP()),
			zap.String("user_agent", c.Request.UserAgent()),
			zap.Duration("latency", latency),
			zap.String("request_id", requestID),
		}

		if len(c.Errors) > 0 {
			for _, e := range c.Errors.ByType(gin.ErrorTypePrivate) {
				fields = append(fields, zap.NamedError("error", e.Err))
			}
		}

		if cfg.GinMode != "release" || (statusCode >= 200 && statusCode < 400) {
			logger.Info("Request handled", fields...)
		} else if statusCode >= 400 && statusCode < 500 {
			logger.Warn("Client error", fields...)
		} else if statusCode >= 500 {
			logger.Error("Server error", fields...)
		}
	}
}
