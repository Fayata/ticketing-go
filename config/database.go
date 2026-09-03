package config

import (
	"context"
	"fmt"
	"log"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var (
	DB *gorm.DB
)

// noQueryLogger: tidak log query ke terminal. Hanya error DB yang tetap dicatat.
type noQueryLogger struct{}

func (noQueryLogger) LogMode(logger.LogLevel) logger.Interface { return noQueryLogger{} }
func (noQueryLogger) Info(context.Context, string, ...interface{}) {}
func (noQueryLogger) Warn(context.Context, string, ...interface{}) {}
func (noQueryLogger) Error(ctx context.Context, msg string, args ...interface{}) {
	logger.Default.Error(ctx, msg, args...)
}
func (noQueryLogger) Trace(ctx context.Context, begin time.Time, fc func() (sql string, rows int64), err error) {
	// Jangan log query (SELECT/CUD) ke terminal. Kalau ada error, tetap log.
	if err != nil {
		logger.Default.Trace(ctx, begin, fc, err)
	}
}

func InitDatabase(cfg *Config) error {
	var err error

	// Tidak log query ke terminal (baik SELECT maupun CUD). Error DB tetap bisa dicatat.
	l := noQueryLogger{}

	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%d sslmode=%s TimeZone=Asia/Jakarta",
		cfg.DBHost,
		cfg.DBUser,
		cfg.DBPassword,
		cfg.DBName,
		cfg.DBPort,
		cfg.DBSSLMode,
	)

	DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: l})

	if err != nil {
		return fmt.Errorf("failed to connect database: %w", err)
	}

	sqlDB, err := DB.DB()
	if err != nil {
		return fmt.Errorf("failed to get database instance: %w", err)
	}
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)

	log.Println("Database connected")
	return nil
}

func AutoMigrate(models ...interface{}) error {
	if err := DB.AutoMigrate(models...); err != nil {
		return fmt.Errorf("failed to migrate database: %w", err)
	}
	log.Println("Database migration completed")
	return nil
}
