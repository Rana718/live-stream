package logger

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

var defaultLogger *slog.Logger

// Init configures the process-wide structured logger.
func Init(level, format string) *slog.Logger {
	lvl := parseLevel(level)
	opts := &slog.HandlerOptions{Level: lvl}

	var handler slog.Handler
	var w io.Writer = os.Stdout
	if strings.EqualFold(format, "text") {
		handler = slog.NewTextHandler(w, opts)
	} else {
		handler = slog.NewJSONHandler(w, opts)
	}

	l := slog.New(handler)
	slog.SetDefault(l)
	defaultLogger = l
	return l
}

// L returns the default logger (falls back to slog.Default).
func L() *slog.Logger {
	if defaultLogger == nil {
		return slog.Default()
	}
	return defaultLogger
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
