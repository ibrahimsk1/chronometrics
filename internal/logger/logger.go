package logger

import (
	"log/slog"
	"os"
)

type Options struct {
	Level slog.Level
}

func New(opts Options) *slog.Logger {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: opts.Level,
	})
	return slog.New(handler)
}
