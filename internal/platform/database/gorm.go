// File: internal/platform/database/gorm.go
package database

import (
	"fmt"
	"log" // Standard log for critical connection errors
	"time"

	"seattle_info_backend/internal/config"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// NewGORM creates a new GORM database instance.
func NewGORM(cfg *config.Config) (*gorm.DB, error) {
	// DSN (Data Source Name) construction
	// GORM's PostgreSQL driver uses a DSN string like:
	// "host=localhost user=gorm password=gorm dbname=gorm port=9920 sslmode=disable TimeZone=Asia/Shanghai"
	// We already have cfg.DBSource that can be in this format OR a URL format for migrate.
	// Let's ensure we use the GORM format here.
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s TimeZone=%s",
		cfg.DBHost,
		cfg.DBPort,
		cfg.DBUser,
		cfg.DBPassword,
		cfg.DBName,
		cfg.DBSSLMode,
		cfg.DBTimezone,
	)

	// GORM Logger Configuration
	var gormLogLevel gormlogger.LogLevel
	switch cfg.LogLevel {
	case "silent", "fatal", "panic":
		gormLogLevel = gormlogger.Silent
	case "error":
		gormLogLevel = gormlogger.Error
	case "warn", "warning":
		gormLogLevel = gormlogger.Warn
	case "info", "debug": // For info and debug, GORM will log all SQL
		gormLogLevel = gormlogger.Info
	default:
		gormLogLevel = gormlogger.Warn // Default to Warn for GORM
	}

	newLogger := gormlogger.New(
		log.New(log.Writer(), "\r\n", log.LstdFlags), // io writer
		gormlogger.Config{
			SlowThreshold:             200 * time.Millisecond,   // Slow SQL threshold
			LogLevel:                  gormLogLevel,             // Log level
			IgnoreRecordNotFoundError: true,                     // Don't log ErrRecordNotFound
			Colorful:                  cfg.GinMode != "release", // Colorful print only in non-release mode
		},
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: newLogger,
		// NamingStrategy: schema.NamingStrategy{
		// TablePrefix:   "si_", // Optional: if you want a prefix for all tables
		// SingularTable: false, // Use plural table names (e.g., "users" instead of "user")
		// },
		PrepareStmt: true, // Caches compiled statements for performance
	})

	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Connection Pool Settings
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}

	sqlDB.SetMaxIdleConns(cfg.DBMaxIdleConns)
	sqlDB.SetMaxOpenConns(cfg.DBMaxOpenConns)
	sqlDB.SetConnMaxLifetime(cfg.DBConnMaxLifetime)

	// Ping the database to verify connection
	if err = sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	log.Println("Successfully connected to the database.") // Use standard log for this one-time message
	return db, nil
}

// CloseGORMDB closes the GORM database connection.
// This is useful for the cleanup function in main.
func CloseGORMDB(db *gorm.DB) {
	if db != nil {
		sqlDB, err := db.DB()
		if err != nil {
			log.Printf("Error getting underlying SQL DB for closing: %v\n", err)
			return
		}
		log.Println("Closing database connection...")
		if err := sqlDB.Close(); err != nil {
			log.Printf("Error closing database connection: %v\n", err)
		} else {
			log.Println("Database connection closed.")
		}
	}
}
