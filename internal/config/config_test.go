package config

import (
	"os"
	"testing"
)

func unsetEnv(keys ...string) {
	for _, k := range keys {
		_ = os.Unsetenv(k)
	}
}

func TestLoad_Defaults(t *testing.T) {
	unsetEnv("EVENTMETRICS_BUFFER_STRATEGY", "SERVER_PORT", "BUFFER_CAPACITY", "BUFFER_FLUSH_INTERVAL", "NATS_URL")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.Strategy != "memory" {
		t.Fatalf("expected default strategy memory, got %s", cfg.Strategy)
	}
	if cfg.Server.Port != 8080 {
		t.Fatalf("expected default port 8080, got %d", cfg.Server.Port)
	}
	if cfg.Buffer.Capacity != 1000 {
		t.Fatalf("expected default buffer capacity 1000, got %d", cfg.Buffer.Capacity)
	}
}

func TestLoad_EnvOverride(t *testing.T) {
	unsetEnv("EVENTMETRICS_BUFFER_STRATEGY", "SERVER_PORT", "BUFFER_CAPACITY", "BUFFER_FLUSH_INTERVAL", "NATS_URL")
	os.Setenv("BUFFER_CAPACITY", "5")
	os.Setenv("SERVER_PORT", "9090")
	defer unsetEnv("BUFFER_CAPACITY", "SERVER_PORT")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.Buffer.Capacity != 5 {
		t.Fatalf("expected buffer capacity 5, got %d", cfg.Buffer.Capacity)
	}
	if cfg.Server.Port != 9090 {
		t.Fatalf("expected server port 9090, got %d", cfg.Server.Port)
	}
}

func TestLoad_InvalidRange(t *testing.T) {
	unsetEnv("EVENTMETRICS_BUFFER_STRATEGY", "SERVER_PORT", "BUFFER_CAPACITY", "BUFFER_FLUSH_INTERVAL", "NATS_URL")
	os.Setenv("BUFFER_CAPACITY", "0")
	defer unsetEnv("BUFFER_CAPACITY")

	_, err := Load()
	if err == nil {
		t.Fatalf("expected error for invalid buffer capacity, got nil")
	}
	if err != ErrInvalidRange {
		t.Fatalf("expected ErrInvalidRange, got %v", err)
	}
}

func TestLoad_NatsRequiresURL(t *testing.T) {
	unsetEnv("EVENTMETRICS_BUFFER_STRATEGY", "SERVER_PORT", "BUFFER_CAPACITY", "BUFFER_FLUSH_INTERVAL", "NATS_URL")
	os.Setenv("EVENTMETRICS_BUFFER_STRATEGY", "nats")
	defer unsetEnv("EVENTMETRICS_BUFFER_STRATEGY")

	_, err := Load()
	if err == nil {
		t.Fatalf("expected error when nats strategy without NATS_URL, got nil")
	}
	if err != ErrMissingNatsURL {
		t.Fatalf("expected ErrMissingNatsURL, got %v", err)
	}
}
