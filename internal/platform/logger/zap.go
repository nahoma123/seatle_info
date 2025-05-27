// File: internal/platform/logger/zap.go
package logger

import (
	"seattle_info_backend/internal/config" // Import your config package
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// New initializes a new Zap logger based on the application configuration.
// It takes the ginMode and logLevel from the config.
func New(cfg *config.Config) (*zap.Logger, error) {
	var zapConfig zap.Config

	// Set log level
	logLevel := strings.ToLower(cfg.LogLevel)
	level := zapcore.InfoLevel // Default level
	switch logLevel {
	case "debug":
		level = zapcore.DebugLevel
	case "info":
		level = zapcore.InfoLevel
	case "warn", "warning":
		level = zapcore.WarnLevel
	case "error":
		level = zapcore.ErrorLevel
	case "dpanic":
		level = zapcore.DPanicLevel
	case "panic":
		level = zapcore.PanicLevel
	case "fatal":
		level = zapcore.FatalLevel
	}

	// Configure based on Gin mode (similar to how Gin sets up its logger)
	if cfg.GinMode == "release" {
		zapConfig = zap.NewProductionConfig()
		zapConfig.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	} else { // "debug" or "test"
		zapConfig = zap.NewDevelopmentConfig()
		// More human-readable output for development
		zapConfig.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		zapConfig.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder // Or zapcore.RFC3339TimeEncoder
	}

	// Override level from config
	zapConfig.Level = zap.NewAtomicLevelAt(level)

	// Output format from config
	logFormat := strings.ToLower(cfg.LogFormat)
	if logFormat == "json" {
		zapConfig.Encoding = "json"
	} else { // "console" or any other value defaults to console for dev-friendly logs
		zapConfig.Encoding = "console"
		if cfg.GinMode == "release" && logFormat != "json" {
			// In release mode, if not json, still prefer json for machine readability
			// unless explicitly console was requested. But production defaults to json.
			// For simplicity, if logFormat is not 'json', it's 'console'.
			// ProductionConfig already sets Encoding to "json".
			// DevelopmentConfig sets Encoding to "console".
			// So, this mostly affects if someone sets GinMode=release and LogFormat=console.
		}
	}

	// Build the logger
	logger, err := zapConfig.Build(zap.AddCallerSkip(1)) // AddCallerSkip to show correct caller
	if err != nil {
		return nil, err
	}

	// Redirect standard log output to Zap
	// This is optional but can be useful to capture logs from libraries that use the standard `log` package.
	// zap.RedirectStdLog(logger) // Be cautious with this, can have performance implications or duplicate logs in some cases.

	return logger, nil
}

// NewSugaredLogger provides a SugaredLogger for convenience.
// It's often easier to use for simple, less performance-critical logging.
func NewSugaredLogger(cfg *config.Config) (*zap.SugaredLogger, error) {
	logger, err := New(cfg)
	if err != nil {
		return nil, err
	}
	return logger.Sugar(), nil
}

// For testing or simple scenarios where config might not be fully loaded
func NewDefaultLogger() *zap.Logger {
	logger, _ := zap.NewDevelopment() // Errors are ignored for simplicity here
	return logger
}
