package util

import (
	"io"
	"log/slog"
	"os"
)

// LogLevel represents the logging level
type LogLevel string

const (
	LevelDebug LogLevel = "debug"
	LevelInfo  LogLevel = "info"
	LevelWarn  LogLevel = "warn"
	LevelError LogLevel = "error"
)

// LogFormat represents the output format for logs
type LogFormat string

const (
	FormatJSON LogFormat = "json"
	FormatText LogFormat = "text"
)

// LoggerConfig holds the configuration for the logger
type LoggerConfig struct {
	Level  LogLevel
	Format LogFormat
	Output io.Writer
}

// DefaultLoggerConfig returns a logger config with sensible defaults
func DefaultLoggerConfig() LoggerConfig {
	return LoggerConfig{
		Level:  LevelInfo,
		Format: FormatJSON,
		Output: os.Stdout,
	}
}

// NewLogger creates a new structured logger with the given configuration
func NewLogger(config LoggerConfig) *slog.Logger {
	level := parseLevel(config.Level)

	var handler slog.Handler
	opts := &slog.HandlerOptions{
		Level: level,
	}

	switch config.Format {
	case FormatJSON:
		handler = slog.NewJSONHandler(config.Output, opts)
	case FormatText:
		handler = slog.NewTextHandler(config.Output, opts)
	default:
		handler = slog.NewJSONHandler(config.Output, opts)
	}

	return slog.New(handler)
}

// parseLevel converts a LogLevel to slog.Level
func parseLevel(level LogLevel) slog.Level {
	switch level {
	case LevelDebug:
		return slog.LevelDebug
	case LevelInfo:
		return slog.LevelInfo
	case LevelWarn:
		return slog.LevelWarn
	case LevelError:
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// SetDefault sets the default logger for the slog package
func SetDefault(logger *slog.Logger) {
	slog.SetDefault(logger)
}

// Example usage:
//
// Create a logger with default config:
//   logger := util.NewLogger(util.DefaultLoggerConfig())
//
// Create a logger with custom config:
//   config := util.LoggerConfig{
//       Level:  util.LevelDebug,
//       Format: util.FormatJSON,
//       Output: os.Stderr,
//   }
//   logger := util.NewLogger(config)
//
// Use the logger:
//   logger.Info("server starting", "port", 6543)
//   logger.Debug("parsing file", "path", "/path/to/file.go")
//   logger.Error("failed to parse", "error", err, "file", path)
