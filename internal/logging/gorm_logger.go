package logging

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// GORMLogger bridges GORM database operations to our structured JSON logging system.
type GORMLogger struct {
	LogLevel                  gormlogger.LogLevel
	SlowThreshold             time.Duration
	LogAllQueries             bool
	IgnoreRecordNotFoundError bool
}

// NewGORMLogger creates a new GORMLogger.
func NewGORMLogger(slowThreshold time.Duration, logAllQueries bool) *GORMLogger {
	if slowThreshold <= 0 {
		slowThreshold = 150 * time.Millisecond
	}
	return &GORMLogger{
		LogLevel:                  gormlogger.Info,
		SlowThreshold:             slowThreshold,
		LogAllQueries:             logAllQueries,
		IgnoreRecordNotFoundError: true,
	}
}

// LogMode sets the logging level.
func (l *GORMLogger) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	newLogger := *l
	newLogger.LogLevel = level
	return &newLogger
}

// Info logs database informational events (e.g. migration, connection).
func (l *GORMLogger) Info(ctx context.Context, msg string, data ...interface{}) {
	if l.LogLevel >= gormlogger.Info {
		formattedMsg := fmt.Sprintf(msg, data...)
		corrID := GetCorrelationID(ctx)
		attrs := []any{slog.String("event", "db_info")}
		if corrID != "" {
			attrs = append(attrs, slog.String("correlation_id", corrID))
		}
		DBMigrations.Info(formattedMsg, attrs...)
	}
}

// Warn logs database warnings.
func (l *GORMLogger) Warn(ctx context.Context, msg string, data ...interface{}) {
	if l.LogLevel >= gormlogger.Warn {
		formattedMsg := fmt.Sprintf(msg, data...)
		corrID := GetCorrelationID(ctx)
		attrs := []any{slog.String("event", "db_warn")}
		if corrID != "" {
			attrs = append(attrs, slog.String("correlation_id", corrID))
		}
		DBErrors.Warn(formattedMsg, attrs...)
	}
}

// Error logs database errors.
func (l *GORMLogger) Error(ctx context.Context, msg string, data ...interface{}) {
	if l.LogLevel >= gormlogger.Error {
		formattedMsg := fmt.Sprintf(msg, data...)
		corrID := GetCorrelationID(ctx)
		attrs := []any{slog.String("event", "db_error")}
		if corrID != "" {
			attrs = append(attrs, slog.String("correlation_id", corrID))
		}
		DBErrors.Error(formattedMsg, attrs...)
	}
}

// Trace logs SQL queries, slow queries, and database query errors.
func (l *GORMLogger) Trace(ctx context.Context, begin time.Time, fc func() (sql string, rows int64), err error) {
	if l.LogLevel <= gormlogger.Silent {
		return
	}

	elapsed := time.Since(begin)
	elapsedMs := float64(elapsed.Microseconds()) / 1000.0
	sql, rows := fc()
	corrID := GetCorrelationID(ctx)

	isSlow := elapsed >= l.SlowThreshold
	hasErr := err != nil && (!l.IgnoreRecordNotFoundError || !errors.Is(err, gorm.ErrRecordNotFound))

	commonAttrs := []any{
		slog.String("sql", sql),
		slog.Float64("elapsed_ms", elapsedMs),
		slog.Int64("rows_affected", rows),
	}
	if corrID != "" {
		commonAttrs = append(commonAttrs, slog.String("correlation_id", corrID))
	}

	// 1. Log query errors to logs/db/errors.json.log
	if hasErr {
		errAttrs := append(commonAttrs, slog.String("error", err.Error()))
		DBErrors.Error("Database query failed", errAttrs...)
	}

	// 2. Log slow queries to logs/db/slow_queries.json.log
	if isSlow {
		slowAttrs := append(commonAttrs,
			slog.Float64("threshold_ms", float64(l.SlowThreshold.Milliseconds())),
			slog.String("anomaly_flag", "SLOW_QUERY"),
		)
		DBSlowQueries.Warn("Slow database query detected", slowAttrs...)
	}

	// 3. Log all queries to logs/db/queries.json.log (when enabled in dev, or if slow/error)
	if l.LogAllQueries || isSlow || hasErr {
		logAttrs := append(commonAttrs,
			slog.Bool("is_slow", isSlow),
		)
		if err != nil {
			logAttrs = append(logAttrs, slog.String("error", err.Error()))
		}
		DBQueries.Debug("SQL execution", logAttrs...)
	}
}
