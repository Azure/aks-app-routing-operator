package clients

import (
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"golang.org/x/exp/slog"
)

type logWriter struct {
	lgr    *slog.Logger
	prefix string
	level  slog.Level
}

func newLogWriter(lgr *slog.Logger, prefix string, level *slog.Level) *logWriter {
	if level == nil {
		level = to.Ptr(slog.LevelInfo)
	}

	return &logWriter{lgr: lgr, prefix: prefix, level: *level}
}

func (lw *logWriter) Write(p []byte) (n int, err error) {
	lw.lgr.Log(nil, lw.level, lw.prefix+string(p))
	return len(p), nil
}
