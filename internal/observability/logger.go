// Package observability owns pgsync slog setup.
package observability

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
)

/* Options configures a slog logger. */
type Options struct {
	Level  string
	Format string
	Out    io.Writer
}

/* NewLogger creates a slog logger from Options. */
func NewLogger(opts Options) (*slog.Logger, error) {
	out := opts.Out
	if out == nil {
		out = os.Stderr
	}
	level, err := parseLevel(opts.Level)
	if err != nil {
		return nil, err
	}

	handlerOptions := &slog.HandlerOptions{Level: level}
	var handler slog.Handler
	switch opts.Format {
	case "text":
		handler = slog.NewTextHandler(out, handlerOptions)
	case "json":
		handler = slog.NewJSONHandler(out, handlerOptions)
	default:
		return nil, fmt.Errorf("unknown format %q", opts.Format)
	}
	return slog.New(handler), nil
}

func parseLevel(raw string) (slog.Level, error) {
	switch raw {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, errors.New("level must be debug|info|warn|error")
	}
}
