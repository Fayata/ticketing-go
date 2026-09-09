package config

import (
	"fmt"
	"time"

	"ticketing/internal/logging"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var (
	DB *gorm.DB
)

func InitDatabase(cfg *Config) error {
	var err error

	slowThresh := time.Duration(cfg.SlowQueryThresholdMs) * time.Millisecond
	if slowThresh <= 0 {
		slowThresh = 150 * time.Millisecond
	}
	dbLogger := logging.NewGORMLogger(slowThresh, cfg.DBLogAllQueries)

	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%d sslmode=%s TimeZone=Asia/Jakarta",
		cfg.DBHost,
		cfg.DBUser,
		cfg.DBPassword,
		cfg.DBName,
		cfg.DBPort,
		cfg.DBSSLMode,
	)

	DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: dbLogger})

	if err != nil {
		logging.DBErrors.Error("Failed to connect database", "error", err.Error(), "host", cfg.DBHost)
		return fmt.Errorf("failed to connect database: %w", err)
	}

	sqlDB, err := DB.DB()
	if err != nil {
		logging.DBErrors.Error("Failed to get database instance", "error", err.Error())
		return fmt.Errorf("failed to get database instance: %w", err)
	}
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)

	logging.DBMigrations.Info("Database connected", "host", cfg.DBHost, "port", cfg.DBPort, "dbname", cfg.DBName)
	return nil
}

func AutoMigrate(models ...interface{}) error {
	if err := DB.AutoMigrate(models...); err != nil {
		logging.DBErrors.Error("Database migration failed", "error", err.Error())
		return fmt.Errorf("failed to migrate database: %w", err)
	}
	logging.DBMigrations.Info("Database migration completed", "models_count", len(models))
	return nil
}
