package main

import (
	"context"
	"log/slog"
	"os/signal"
	"syscall"

	"eventmetrics/internal/app"
	"eventmetrics/internal/logger"
)

func main() {
	log := logger.New(logger.Options{Level: slog.LevelInfo})

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	a, err := app.Build(ctx, log)
	if err != nil {
		log.Error("setup failed", "error", err)
		return
	}

	a.Run(ctx)
}
