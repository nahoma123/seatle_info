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

	// JWT Configuration
	JWTSecretKey                string        `mapstructure:"JWT_SECRET_KEY"`
	JWTAccessTokenExpiryMinutes time.Duration `mapstructure:"JWT_ACCESS_TOKEN_EXPIRY_MINUTES"`
	JWTRefreshTokenExpiryDays   time.Duration `mapstructure:"JWT_REFRESH_TOKEN_EXPIRY_DAYS"`

	// Logging Configuration
	LogLevel  string `mapstructure:"LOG_LEVEL"`
	LogFormat string `mapstructure:"LOG_FORMAT"`

	// Application Specific Configuration
	DefaultListingLifespanDays    int `mapstructure:"DEFAULT_LISTING_LIFESPAN_DAYS"`
	MaxListingDistanceKM          int `mapstructure:"MAX_LISTING_DISTANCE_KM"`
	FirstPostApprovalActiveMonths int `mapstructure:"FIRST_POST_APPROVAL_ACTIVE_MONTHS"`

	// Cron Jobs
	ListingExpiryJobSchedule string `mapstructure:"LISTING_EXPIRY_JOB_SCHEDULE"`

	// OAuth - Google
	GoogleClientID     string `mapstructure:"GOOGLE_CLIENT_ID"`
	GoogleClientSecret string `mapstructure:"GOOGLE_CLIENT_SECRET"`
	GoogleRedirectURI  string `mapstructure:"GOOGLE_REDIRECT_URI"`

	// OAuth - Apple
	AppleTeamID         string `mapstructure:"APPLE_TEAM_ID"`
	AppleClientID       string `mapstructure:"APPLE_CLIENT_ID"`
	AppleKeyID          string `mapstructure:"APPLE_KEY_ID"`
	ApplePrivateKeyPath string `mapstructure:"APPLE_PRIVATE_KEY_PATH"`
	AppleRedirectURI    string `mapstructure:"APPLE_REDIRECT_URI"`

	// OAuth Cookie settings
	OAuthCookieDomain        string `mapstructure:"OAUTH_COOKIE_DOMAIN"`
	OAuthCookieSecure        bool   `mapstructure:"OAUTH_COOKIE_SECURE"`
	OAuthCookieHTTPOnly      bool   `mapstructure:"OAUTH_COOKIE_HTTP_ONLY"`
	OAuthCookieSameSite      string `mapstructure:"OAUTH_COOKIE_SAME_SITE"`
	OAuthStateCookieName     string `mapstructure:"OAUTH_STATE_COOKIE_NAME"`
	OAuthNonceCookieName     string `mapstructure:"OAUTH_NONCE_COOKIE_NAME"`
	OAuthCookieMaxAgeMinutes int    `mapstructure:"OAUTH_COOKIE_MAX_AGE_MINUTES"`
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

	v.SetDefault("JWT_SECRET_KEY", "default_secret_key_please_change")
	v.SetDefault("JWT_ACCESS_TOKEN_EXPIRY_MINUTES", 60)
	v.SetDefault("JWT_REFRESH_TOKEN_EXPIRY_DAYS", 7)

	v.SetDefault("LOG_LEVEL", "info")
	v.SetDefault("LOG_FORMAT", "console")

	v.SetDefault("DEFAULT_LISTING_LIFESPAN_DAYS", 10)
	v.SetDefault("MAX_LISTING_DISTANCE_KM", 50)
	v.SetDefault("FIRST_POST_APPROVAL_ACTIVE_MONTHS", 6)
	v.SetDefault("LISTING_EXPIRY_JOB_SCHEDULE", "@daily")

	v.SetDefault("GOOGLE_REDIRECT_URI", "http://localhost:8080/api/v1/auth/google/callback")
	v.SetDefault("APPLE_REDIRECT_URI", "http://localhost:8080/api/v1/auth/apple/callback")

	v.SetDefault("OAUTH_COOKIE_DOMAIN", "localhost")
	v.SetDefault("OAUTH_COOKIE_SECURE", false)
	v.SetDefault("OAUTH_COOKIE_HTTP_ONLY", true)
	v.SetDefault("OAUTH_COOKIE_SAME_SITE", "Lax")
	v.SetDefault("OAUTH_STATE_COOKIE_NAME", "oauth_state")
	v.SetDefault("OAUTH_NONCE_COOKIE_NAME", "oauth_nonce")
	v.SetDefault("OAUTH_COOKIE_MAX_AGE_MINUTES", 10)

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
	if cfg.JWTSecretKey == "default_secret_key_please_change" || strings.TrimSpace(cfg.JWTSecretKey) == "" {
		return nil, fmt.Errorf("FATAL: JWT_SECRET_KEY is not set or is using the default insecure value. Please set a strong secret")
	}
	if cfg.DBUser == "your_db_user" || cfg.DBPassword == "your_db_password" {
		// This is just a warning, app might still run if defaults are valid for a local setup.
		fmt.Println("WARNING: Database credentials might be using default example values. Please update them in your .env file if this is not intended.")
	}

	if cfg.GoogleClientID == "" || cfg.GoogleClientSecret == "" {
		fmt.Println("WARNING: Google OAuth credentials (GOOGLE_CLIENT_ID, GOOGLE_CLIENT_SECRET) are not fully set. Google Sign-In will not work.")
	}
	if cfg.AppleTeamID == "" || cfg.AppleClientID == "" || cfg.AppleKeyID == "" || cfg.ApplePrivateKeyPath == "" {
		fmt.Println("WARNING: Apple Sign-In credentials (APPLE_TEAM_ID, APPLE_CLIENT_ID, APPLE_KEY_ID, APPLE_PRIVATE_KEY_PATH) are not fully set. Sign in with Apple will not work.")
	}
	if cfg.ApplePrivateKeyPath != "" {
		if _, err := os.Stat(cfg.ApplePrivateKeyPath); os.IsNotExist(err) {
			// This is a critical failure if Apple Sign In is intended.
			// Could return an error, or just log a strong warning.
			// For now, strong warning, service will fail later.
			fmt.Printf("CRITICAL WARNING: Apple private key file specified in APPLE_PRIVATE_KEY_PATH (%s) not found. Sign in with Apple will fail.\n", cfg.ApplePrivateKeyPath)
		}
	}

	return &cfg, nil
}
