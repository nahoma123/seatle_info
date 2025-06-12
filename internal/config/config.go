// File: internal/config/config.go
package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

// Config holds all configuration for the application.
type Config struct {
	// Server Configuration
	GinMode       string        `mapstructure:"GIN_MODE"`
	ServerHost    string        `mapstructure:"SERVER_HOST"`
	ServerPort    string        `mapstructure:"SERVER_PORT"`
	ServerTimeout time.Duration `mapstructure:"SERVER_TIMEOUT_SECONDS"`

	// Database Configuration
	DBHost            string        `mapstructure:"DB_HOST"`
	DBPort            string        `mapstructure:"DB_PORT"`
	DBUser            string        `mapstructure:"DB_USER"`
	DBPassword        string        `mapstructure:"DB_PASSWORD"`
	DBName            string        `mapstructure:"DB_NAME"`
	DBSSLMode         string        `mapstructure:"DB_SSL_MODE"`
	DBTimezone        string        `mapstructure:"DB_TIMEZONE"`
	DBMaxIdleConns    int           `mapstructure:"DB_MAX_IDLE_CONNS"`
	DBMaxOpenConns    int           `mapstructure:"DB_MAX_OPEN_CONNS"`
	DBConnMaxLifetime time.Duration `mapstructure:"DB_CONN_MAX_LIFETIME_MINUTES"`
	DBSource          string        `mapstructure:"DB_SOURCE"`

	// Logging Configuration
	LogLevel  string `mapstructure:"LOG_LEVEL"`
	LogFormat string `mapstructure:"LOG_FORMAT"`

	// Application Specific Configuration
	DefaultListingLifespanDays    int `mapstructure:"DEFAULT_LISTING_LIFESPAN_DAYS"`
	MaxListingDistanceKM          int `mapstructure:"MAX_LISTING_DISTANCE_KM"`
	FirstPostApprovalActiveMonths int `mapstructure:"FIRST_POST_APPROVAL_ACTIVE_MONTHS"`

	// Cron Jobs
	ListingExpiryJobSchedule string `mapstructure:"LISTING_EXPIRY_JOB_SCHEDULE"`

	// Firebase Configuration
	FirebaseServiceAccountKeyPath string `mapstructure:"FIREBASE_SERVICE_ACCOUNT_KEY_PATH"`
	FirebaseProjectID             string `mapstructure:"FIREBASE_PROJECT_ID"`

	// Elasticsearch Configuration
	ElasticsearchURL string `mapstructure:"ELASTICSEARCH_URL"`
}

// Load attempts to load configuration from a .env file (if present) and environment variables.
func Load() (*Config, error) {
	if err := godotenv.Load(); err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("error loading .env file: %w", err)
		}
	}

	v := viper.New()

	// Set default values
	v.SetDefault("GIN_MODE", "debug")
	v.SetDefault("SERVER_HOST", "0.0.0.0")
	v.SetDefault("SERVER_PORT", "8080")
	v.SetDefault("SERVER_TIMEOUT_SECONDS", 30)

	v.SetDefault("DB_HOST", "localhost")
	v.SetDefault("DB_PORT", "5432")
	v.SetDefault("DB_USER", "postgres")
	v.SetDefault("DB_PASSWORD", "password")
	v.SetDefault("DB_NAME", "seattle_info_db")
	v.SetDefault("DB_SSL_MODE", "disable")
	v.SetDefault("DB_TIMEZONE", "UTC")
	v.SetDefault("DB_MAX_IDLE_CONNS", 10)
	v.SetDefault("DB_MAX_OPEN_CONNS", 100)
	v.SetDefault("DB_CONN_MAX_LIFETIME_MINUTES", 60)
	v.SetDefault("DB_SOURCE", "postgresql://postgres:password@localhost:5432/seattle_info_db?sslmode=disable")

	v.SetDefault("LOG_LEVEL", "info")
	v.SetDefault("LOG_FORMAT", "console")

	v.SetDefault("DEFAULT_LISTING_LIFESPAN_DAYS", 10)
	v.SetDefault("MAX_LISTING_DISTANCE_KM", 50)
	v.SetDefault("FIRST_POST_APPROVAL_ACTIVE_MONTHS", 6)
	v.SetDefault("LISTING_EXPIRY_JOB_SCHEDULE", "@daily")

	// Firebase
	v.SetDefault("FIREBASE_PROJECT_ID", "") // Optional
	v.SetDefault("FIREBASE_SERVICE_ACCOUNT_KEY_PATH", "")

	// Elasticsearch
	v.SetDefault("ELASTICSEARCH_URL", "http://localhost:9200")

	v.AutomaticEnv()
	// Optional: v.SetConfigName("config"); v.AddConfigPath("."); v.ReadInConfig()

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("error unmarshalling configuration: %w", err)
	}

	// Convert duration fields
	cfg.ServerTimeout = time.Duration(v.GetInt("SERVER_TIMEOUT_SECONDS")) * time.Second
	cfg.DBConnMaxLifetime = time.Duration(v.GetInt("DB_CONN_MAX_LIFETIME_MINUTES")) * time.Minute

	// Construct DBSource for GORM if not explicitly set by env var DB_SOURCE
	// This ensures GORM DSN is available even if only individual DB params are set.
	// The DB_SOURCE env var is primarily for golang-migrate.
	currentDBSource := v.GetString("DB_SOURCE") // Get what Viper resolved for DB_SOURCE
	constructedDSN := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s TimeZone=%s",
		cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBPassword, cfg.DBName, cfg.DBSSLMode, cfg.DBTimezone)

	if currentDBSource == v.GetString("DB_SOURCE") { // Check if DB_SOURCE was just the default
		// If DBSource is still the default one (not set by env var), prefer the one constructed from parts for GORM.
		cfg.DBSource = constructedDSN
	} else {
		// If DB_SOURCE was set via environment variable, use that for cfg.DBSource as well for consistency.
		// This assumes the env var DB_SOURCE is in GORM DSN format if it's different from the individual params.
		// If DB_SOURCE is a URL (for migrate), GORM might need the param-based DSN.
		// For simplicity: if DB_SOURCE env var is set, assume it's for migrate.
		// GORM will use the DSN constructed from individual DB_* params.
		// So, we ensure cfg.DBSource for GORM is the param-based one.
		// The original DB_SOURCE from env for migrate is still accessible via os.Getenv("DB_SOURCE") in Makefile.
		cfg.DBSource = constructedDSN
	}

	// Basic validation for critical configs
	if strings.TrimSpace(cfg.FirebaseServiceAccountKeyPath) == "" {
		return nil, fmt.Errorf("FATAL: FIREBASE_SERVICE_ACCOUNT_KEY_PATH is not set. This is required for Firebase Admin SDK initialization")
	}
	if _, err := os.Stat(cfg.FirebaseServiceAccountKeyPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("FATAL: Firebase service account key file specified in FIREBASE_SERVICE_ACCOUNT_KEY_PATH (%s) not found", cfg.FirebaseServiceAccountKeyPath)
	}

	return &cfg, nil
}
