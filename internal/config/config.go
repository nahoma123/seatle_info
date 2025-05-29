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

	// Deprecated: Replaced by Firebase Auth.
	JWTSecretKey                string        `mapstructure:"JWT_SECRET_KEY"`
	// Deprecated: Replaced by Firebase Auth.
	JWTAccessTokenExpiryMinutes time.Duration `mapstructure:"JWT_ACCESS_TOKEN_EXPIRY_MINUTES"`
	// Deprecated: Replaced by Firebase Auth.
	JWTRefreshTokenExpiryDays   time.Duration `mapstructure:"JWT_REFRESH_TOKEN_EXPIRY_DAYS"`

	// Application Specific Configuration
	DefaultListingLifespanDays    int `mapstructure:"DEFAULT_LISTING_LIFESPAN_DAYS"`
	MaxListingDistanceKM          int `mapstructure:"MAX_LISTING_DISTANCE_KM"`
	FirstPostApprovalActiveMonths int `mapstructure:"FIRST_POST_APPROVAL_ACTIVE_MONTHS"`

	// Cron Jobs
	ListingExpiryJobSchedule string `mapstructure:"LISTING_EXPIRY_JOB_SCHEDULE"`

	// Deprecated: Replaced by Firebase Auth.
	GoogleClientID     string `mapstructure:"GOOGLE_CLIENT_ID"`
	// Deprecated: Replaced by Firebase Auth.
	GoogleClientSecret string `mapstructure:"GOOGLE_CLIENT_SECRET"`
	// Deprecated: Replaced by Firebase Auth.
	GoogleRedirectURI  string `mapstructure:"GOOGLE_REDIRECT_URI"`

	// Deprecated: Replaced by Firebase Auth.
	AppleTeamID         string `mapstructure:"APPLE_TEAM_ID"`
	// Deprecated: Replaced by Firebase Auth.
	AppleClientID       string `mapstructure:"APPLE_CLIENT_ID"`
	// Deprecated: Replaced by Firebase Auth.
	AppleKeyID          string `mapstructure:"APPLE_KEY_ID"`
	// Deprecated: Replaced by Firebase Auth.
	ApplePrivateKeyPath string `mapstructure:"APPLE_PRIVATE_KEY_PATH"`
	// Deprecated: Replaced by Firebase Auth.
	AppleRedirectURI    string `mapstructure:"APPLE_REDIRECT_URI"`

	// Firebase Configuration
	FirebaseServiceAccountKeyPath string `mapstructure:"FIREBASE_SERVICE_ACCOUNT_KEY_PATH"`
	FirebaseProjectID             string `mapstructure:"FIREBASE_PROJECT_ID"`
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

	v.SetDefault("JWT_SECRET_KEY", "default_deprecated_jwt_key")
	v.SetDefault("JWT_ACCESS_TOKEN_EXPIRY_MINUTES", 60)
	v.SetDefault("JWT_REFRESH_TOKEN_EXPIRY_DAYS", 7)

	v.SetDefault("DEFAULT_LISTING_LIFESPAN_DAYS", 10)
	v.SetDefault("MAX_LISTING_DISTANCE_KM", 50)
	v.SetDefault("FIRST_POST_APPROVAL_ACTIVE_MONTHS", 6)
	v.SetDefault("LISTING_EXPIRY_JOB_SCHEDULE", "@daily")

	v.SetDefault("GOOGLE_CLIENT_ID", "deprecated_google_client_id")
	v.SetDefault("GOOGLE_CLIENT_SECRET", "deprecated_google_client_secret")
	v.SetDefault("GOOGLE_REDIRECT_URI", "http://localhost:8080/api/v1/auth/google/callback_deprecated")
	v.SetDefault("APPLE_TEAM_ID", "deprecated_apple_team_id")
	v.SetDefault("APPLE_CLIENT_ID", "deprecated_apple_client_id")
	v.SetDefault("APPLE_KEY_ID", "deprecated_apple_key_id")
	v.SetDefault("APPLE_PRIVATE_KEY_PATH", "/path/to/deprecated_apple_key.p8")
	v.SetDefault("APPLE_REDIRECT_URI", "http://localhost:8080/api/v1/auth/apple/callback_deprecated")

	// Firebase
	v.SetDefault("FIREBASE_PROJECT_ID", "") // Optional

	v.AutomaticEnv()
	// Optional: v.SetConfigName("config"); v.AddConfigPath("."); v.ReadInConfig()

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("error unmarshalling configuration: %w", err)
	}

	// Convert duration fields
	cfg.ServerTimeout = time.Duration(v.GetInt("SERVER_TIMEOUT_SECONDS")) * time.Second
	cfg.DBConnMaxLifetime = time.Duration(v.GetInt("DB_CONN_MAX_LIFETIME_MINUTES")) * time.Minute
	cfg.JWTAccessTokenExpiryMinutes = time.Duration(v.GetInt("JWT_ACCESS_TOKEN_EXPIRY_MINUTES")) * time.Minute
	cfg.JWTRefreshTokenExpiryDays = time.Duration(v.GetInt("JWT_REFRESH_TOKEN_EXPIRY_DAYS")) * 24 * time.Hour

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

	// Commented out validation for deprecated fields
	// if cfg.JWTSecretKey == "default_deprecated_jwt_key" || strings.TrimSpace(cfg.JWTSecretKey) == "" {
	// 	// return nil, fmt.Errorf("FATAL: JWT_SECRET_KEY is not set or is using the default insecure value. Please set a strong secret")
	//  fmt.Println("INFO: JWT_SECRET_KEY is using deprecated default or is empty.")
	// }
	// if cfg.GoogleClientID == "deprecated_google_client_id" || cfg.GoogleClientSecret == "deprecated_google_client_secret" {
	// 	// fmt.Println("WARNING: Google OAuth credentials (GOOGLE_CLIENT_ID, GOOGLE_CLIENT_SECRET) are not fully set or using deprecated defaults. Google Sign-In will not work.")
	// }
	// if cfg.AppleTeamID == "deprecated_apple_team_id" || cfg.AppleClientID == "deprecated_apple_client_id" || cfg.AppleKeyID == "deprecated_apple_key_id" || cfg.ApplePrivateKeyPath == "/path/to/deprecated_apple_key.p8" {
	// 	// fmt.Println("WARNING: Apple Sign-In credentials are not fully set or using deprecated defaults. Sign in with Apple will not work.")
	// }
	// if cfg.ApplePrivateKeyPath != "/path/to/deprecated_apple_key.p8" && cfg.ApplePrivateKeyPath != "" { // only check if it's not the default deprecated path and not empty
	// 	if _, err := os.Stat(cfg.ApplePrivateKeyPath); os.IsNotExist(err) {
	// 		// fmt.Printf("CRITICAL WARNING: Apple private key file specified in APPLE_PRIVATE_KEY_PATH (%s) not found. Sign in with Apple will fail.\n", cfg.ApplePrivateKeyPath)
	// 	}
	// }


	if cfg.DBUser == "your_db_user" || cfg.DBPassword == "your_db_password" {
		// This is just a warning, app might still run if defaults are valid for a local setup.
		fmt.Println("WARNING: Database credentials might be using default example values. Please update them in your .env file if this is not intended.")
	}

	return &cfg, nil
}
