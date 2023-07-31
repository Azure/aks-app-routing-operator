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

// WithContext returns a new context with the logger
func WithContext(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}

// FromContext returns the logger from the context or the default logger
func FromContext(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(loggerKey).(*slog.Logger); ok {
		return logger
	}

	return def
}

// LoggedError is an error that has been logged
type LoggedError struct {
	err error
}

func (e *LoggedError) Error() string {
	return e.err.Error()
}

// Error logs an error and returns a LoggedError that wraps the error
// to indicate that the error has been logged
func Error(logger *slog.Logger, err error) *LoggedError {
	logger.Error(err.Error())
	return &LoggedError{err: err}
}
