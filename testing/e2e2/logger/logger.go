package logger

import (
	"context"

	"golang.org/x/exp/slog"
)

type ctxKey struct{}

var (
	def       = slog.Default()
	loggerKey = ctxKey{}
)

func WithContext(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}

func FromContext(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(loggerKey).(*slog.Logger); ok {
		return logger
	}

	return def
}
