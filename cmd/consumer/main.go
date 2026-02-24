package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"eventmetrics/internal/app"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	base, err := app.Setup(ctx)
	if err != nil {
		log.Fatalf("setup failed: %v", err)
	}

	if base.Config.Strategy != "nats" {
		log.Fatalf("consumer requires strategy=nats, current=%s", base.Config.Strategy)
	}

	// NATS consumer not implemented in this iteration.
	log.Println("consumer for 'nats' strategy is not implemented in this build")

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	base.Shutdown(shutdownCtx)
	log.Println("consumer shutdown complete")
	os.Exit(0)
}
