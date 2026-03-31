package logging

import (
	"log/slog"
	"os"
	"strings"
)

const defaultBufferSize = 1000

// New creates and returns a configured slog.Logger backed by a TeeHandler.
// The returned RingBuffer captures the last 1000 log entries for the API.
func New(level, format string) (*slog.Logger, *RingBuffer) {
	lvl := parseLevel(level)
	opts := &slog.HandlerOptions{
		Level: lvl,
	}

	var output slog.Handler
	if strings.ToLower(format) == "text" {
		output = slog.NewTextHandler(os.Stdout, opts)
	} else {
		output = slog.NewJSONHandler(os.Stdout, opts)
	}

	buf := NewRingBuffer(defaultBufferSize)
	handler := NewTeeHandler(output, buf)

	return slog.New(handler), buf
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(level) {
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
