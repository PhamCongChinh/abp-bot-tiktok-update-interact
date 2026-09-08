// Package logger provides a zap-based structured logger with log rotation
// via lumberjack and session correlation IDs for distributed tracing.
package logger

import (
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Config holds logger configuration for rotation and level.
type Config struct {
	Level      string // debug, info, warn, error
	FilePath   string // path to the log file (e.g., "./logs/bot.log")
	MaxSizeMB  int    // max megabytes before rotation
	MaxAgeDays int    // max days to retain old log files
	MaxBackups int    // max number of old log files to keep
}

// New creates a zap.Logger that writes to both console (colored, human-readable)
// and a rotated log file (JSON format). It returns an error only when the log
// directory cannot be created; level parse failures silently fall back to Info.
func New(cfg Config) (*zap.Logger, error) {
	lvl, err := zapcore.ParseLevel(cfg.Level)
	if err != nil {
		lvl = zapcore.InfoLevel
	}

	// Ensure log directory exists
	if cfg.FilePath != "" {
		if err := os.MkdirAll(filepath.Dir(cfg.FilePath), 0755); err != nil {
			return nil, err
		}
	}

	// Console encoder — colored, human-readable timestamps
	consoleCfg := zap.NewDevelopmentEncoderConfig()
	consoleCfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
	consoleCfg.EncodeTime = zapcore.TimeEncoderOfLayout("2006-01-02 15:04:05")
	consoleEncoder := zapcore.NewConsoleEncoder(consoleCfg)

	// File encoder — JSON for machine ingestion
	fileCfg := zap.NewProductionEncoderConfig()
	fileCfg.EncodeTime = zapcore.ISO8601TimeEncoder
	fileEncoder := zapcore.NewJSONEncoder(fileCfg)

	// Lumberjack for log rotation
	fileWriter := zapcore.AddSync(&lumberjack.Logger{
		Filename:   cfg.FilePath,
		MaxSize:    cfg.MaxSizeMB,
		MaxAge:     cfg.MaxAgeDays,
		MaxBackups: cfg.MaxBackups,
		Compress:   true,
	})

	// Tee core: writes to both console and file
	core := zapcore.NewTee(
		zapcore.NewCore(consoleEncoder, zapcore.AddSync(os.Stdout), lvl),
		zapcore.NewCore(fileEncoder, fileWriter, lvl),
	)

	return zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel)), nil
}

// NewSessionID generates a new UUID v4 string for use as a crawl session
// correlation identifier.
func NewSessionID() string {
	return uuid.New().String()
}

// WithSession returns a child logger that includes the given sessionID in
// every log entry under the "session_id" field.
func WithSession(log *zap.Logger, sessionID string) *zap.Logger {
	return log.With(zap.String("session_id", sessionID))
}
