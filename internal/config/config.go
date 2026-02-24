package config

import (
	"errors"
	"os"
	"strconv"
)

type ServerConfig struct {
	Port int
}

type BufferConfig struct {
	Capacity      int
	FlushInterval int // seconds
}

type NATSConfig struct {
	URL string
}

type ConsumerConfig struct {
	// placeholder for consumer-related settings
	PullBatchSize int
}

type ClickHouseConfig struct {
	DSN string
}

type ValidationConfig struct {
	MaxFutureSeconds int
	MaxPastSeconds   int
}

type Config struct {
	Strategy   string
	Server     ServerConfig
	Buffer     BufferConfig
	NATS       NATSConfig
	Consumer   ConsumerConfig
	ClickHouse ClickHouseConfig
	Validation ValidationConfig
}

var (
	ErrInvalidRange   = errors.New("invalid range in config")
	ErrMissingNatsURL = errors.New("nats strategy requires NATS_URL")
)

// Load reads configuration from environment, applies defaults, and validates.
func Load() (Config, error) {
	cfg := Config{
		Strategy: "memory",
		Server: ServerConfig{
			Port: 8080,
		},
		Buffer: BufferConfig{
			Capacity:      1000,
			FlushInterval: 5,
		},
		NATS: NATSConfig{
			URL: "",
		},
		Consumer: ConsumerConfig{
			PullBatchSize: 100,
		},
		ClickHouse: ClickHouseConfig{
			DSN: "",
		},
		Validation: ValidationConfig{
			MaxFutureSeconds: 300,
			MaxPastSeconds:   60 * 60 * 24 * 30, // 30 days
		},
	}

	if v, ok := os.LookupEnv("EVENTMETRICS_BUFFER_STRATEGY"); ok && v != "" {
		cfg.Strategy = v
	}

	if v, ok := os.LookupEnv("SERVER_PORT"); ok && v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.Server.Port = p
		}
	}

	if v, ok := os.LookupEnv("BUFFER_CAPACITY"); ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Buffer.Capacity = n
		}
	}

	if v, ok := os.LookupEnv("BUFFER_FLUSH_INTERVAL"); ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Buffer.FlushInterval = n
		}
	}

	if v, ok := os.LookupEnv("NATS_URL"); ok && v != "" {
		cfg.NATS.URL = v
	}

	// Validation
	if cfg.Buffer.Capacity <= 0 {
		return Config{}, ErrInvalidRange
	}

	if cfg.Strategy == "nats" || cfg.Strategy == "nats-embedded" {
		if cfg.NATS.URL == "" {
			return Config{}, ErrMissingNatsURL
		}
	}

	return cfg, nil
}

