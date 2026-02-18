package logger

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"

	"gopkg.in/natefinch/lumberjack.v2"
)

// MultiHandler fans out records to multiple handlers.
type MultiHandler struct {
	handlers []slog.Handler
}

func NewMultiHandler(handlers ...slog.Handler) *MultiHandler {
	return &MultiHandler{handlers: handlers}
}

func (h *MultiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (h *MultiHandler) Handle(ctx context.Context, r slog.Record) error {
	for _, handler := range h.handlers {
		if err := handler.Handle(ctx, r); err != nil {
			return err
		}
	}
	return nil
}

func (h *MultiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	handlers := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		handlers[i] = handler.WithAttrs(attrs)
	}
	return &MultiHandler{handlers: handlers}
}

func (h *MultiHandler) WithGroup(name string) slog.Handler {
	handlers := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		handlers[i] = handler.WithGroup(name)
	}
	return &MultiHandler{handlers: handlers}
}

// Setup configures the default slog logger to write:
// 1. JSON logs to a rotating file in <stateDir>/logs/goclaw.log
// 2. Text (pretty) logs to os.Stdout
func Setup(stateDir string) {
	logDir := filepath.Join(stateDir, "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		// Fallback to stderr if we can't create log dir
		slog.Error("failed to create log directory", "error", err)
	}

	// 1. File Handler (JSON, Rotating)
	fileLogger := &lumberjack.Logger{
		Filename:   filepath.Join(logDir, "goclaw.log"),
		MaxSize:    10,   // megabytes
		MaxBackups: 3,    // files
		MaxAge:     28,   // days
		Compress:   true, // disabled by default
	}

	jsonHandler := slog.NewJSONHandler(fileLogger, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})

	// 2. Console Handler (Text, Pretty)
	// We use TextHandler for now. For "pretty" colors, typically need a custom handler,
	// but standard TextHandler is good enough for dev.
	consoleHandler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug, // Show debug in console during dev? Or Info? Let's use Info for now to match.
		// Actually, usually console is for Devs, so Debug might be nice if we had a flag.
		// For now default to Info.
	})

	// Combine
	multi := NewMultiHandler(jsonHandler, consoleHandler)
	slog.SetDefault(slog.New(multi))
}
