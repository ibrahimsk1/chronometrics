package config

import (
	"errors"
	"os"
	"strconv"
	"time"
)

type ServerConfig struct {
	Port            int
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	BodyLimit       int64
	ShutdownTimeout time.Duration
}

type BufferConfig struct {
	Capacity       int
	// backward-compatible integer millisecond fields used by existing code
	FlushInterval  int // milliseconds
	FlushBatchSize int
	FlushRetries   int
	FlushTimeoutMs int
	// TDD-preferred duration fields (populated from env when possible)
	FlushIntervalDuration time.Duration
	FlushTimeout          time.Duration
}

type NATSConfig struct {
	URL            string
	StreamName     string
	Subject        string
	MaxBytes       int64
	Replicas       int
	PublishTimeout time.Duration
}

type ConsumerConfig struct {
	DurableName   string
	AckWait       time.Duration
	MaxDeliver    int
	MaxAckPending int
	BatchSize     int
	BatchTimeout  time.Duration
	FlushTimeout  time.Duration
	HealthPort    int
	// legacy / compatibility
	PullBatchSize int
}

type ClickHouseConfig struct {
	DSN          string // kept for compatibility
	Addr         string
	Database     string
	Username     string
	Password     string
	QueryTimeout time.Duration
}

type ValidationConfig struct {
	// durations preferred, but keep seconds ints for compatibility with earlier tests/code
	MaxFutureDuration time.Duration
	MaxPastDuration   time.Duration
	MaxFutureSeconds  int
	MaxPastSeconds    int
}

type Config struct {
	// Strategy remains for backward compatibility; BufferStrategy is preferred name in TDD
	Strategy       string
	BufferStrategy string

	Server         ServerConfig
	Buffer         BufferConfig
	NATS           NATSConfig
	Consumer       ConsumerConfig
	ClickHouse     ClickHouseConfig
	Validation     ValidationConfig
}

var (
	ErrInvalidRange   = errors.New("invalid range in config")
	ErrMissingNatsURL = errors.New("nats strategy requires EVENTMETRICS_NATS_URL")
)

// Load reads configuration from environment, applies defaults, and validates.
// Returns a value (non-pointer) for minimal churn with existing callers.
func Load() (Config, error) {
	// defaults from TDD
	cfg := Config{
		Strategy:       "memory",
		BufferStrategy: "memory",
		Server: ServerConfig{
			Port:            8080,
			ReadTimeout:     5 * time.Second,
			WriteTimeout:    10 * time.Second,
			IdleTimeout:     60 * time.Second,
			BodyLimit:       1048576, // 1 MB
			ShutdownTimeout: 10 * time.Second,
		},
		Buffer: BufferConfig{
			Capacity:              1000,
			FlushInterval:         100, // ms
			FlushBatchSize:        1000,
			FlushRetries:          3,
			FlushTimeoutMs:        10000,
			FlushIntervalDuration: 100 * time.Millisecond,
			FlushTimeout:          10 * time.Second,
		},
		NATS: NATSConfig{
			URL:            "",
			StreamName:     "EVENTS",
			Subject:        "events.ingest",
			MaxBytes:       10737418240, // 10 GB
			Replicas:       1,
			PublishTimeout: 2 * time.Second,
		},
		Consumer: ConsumerConfig{
			DurableName:   "flusher",
			AckWait:       30 * time.Second,
			MaxDeliver:    5,
			MaxAckPending: 5000,
			BatchSize:     1000,
			BatchTimeout:  100 * time.Millisecond,
			FlushTimeout:  10 * time.Second,
			HealthPort:    8081,
			PullBatchSize: 100,
		},
		ClickHouse: ClickHouseConfig{
			DSN:          "",
			Addr:         "localhost:9000",
			Database:     "default",
			Username:     "default",
			Password:     "",
			QueryTimeout: 30 * time.Second,
		},
		Validation: ValidationConfig{
			MaxFutureDuration: 24 * time.Hour,
			MaxPastDuration:   8760 * time.Hour,
			MaxFutureSeconds:  int((24 * time.Hour).Seconds()),
			MaxPastSeconds:    int((8760 * time.Hour).Seconds()),
		},
	}

	// Helper to override integer env vars
	// Strategy (support legacy and TDD-prefixed env var)
	if v, ok := os.LookupEnv("EVENTMETRICS_BUFFER_STRATEGY"); ok && v != "" {
		cfg.BufferStrategy = v
		cfg.Strategy = v
	}
	if v, ok := os.LookupEnv("BUFFER_STRATEGY"); ok && v != "" {
		cfg.BufferStrategy = v
		cfg.Strategy = v
	}

	// Server port (support TDD and legacy env var)
	if v, ok := os.LookupEnv("EVENTMETRICS_SERVER_PORT"); ok && v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.Server.Port = p
		}
	}
	if v, ok := os.LookupEnv("SERVER_PORT"); ok && v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.Server.Port = p
		}
	}

	if v, ok := os.LookupEnv("EVENTMETRICS_SERVER_READ_TIMEOUT"); ok && v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Server.ReadTimeout = d
		}
	}
	if v, ok := os.LookupEnv("EVENTMETRICS_SERVER_WRITE_TIMEOUT"); ok && v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Server.WriteTimeout = d
		}
	}
	if v, ok := os.LookupEnv("EVENTMETRICS_SERVER_IDLE_TIMEOUT"); ok && v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Server.IdleTimeout = d
		}
	}
	if v, ok := os.LookupEnv("EVENTMETRICS_SERVER_BODY_LIMIT"); ok && v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			cfg.Server.BodyLimit = n
		}
	}
	if v, ok := os.LookupEnv("EVENTMETRICS_SERVER_SHUTDOWN_TIMEOUT"); ok && v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Server.ShutdownTimeout = d
		}
	}

	// Buffer (memory) envs
	// Buffer capacity (support TDD and legacy names)
	if v, ok := os.LookupEnv("EVENTMETRICS_BUFFER_CAPACITY"); ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Buffer.Capacity = n
		}
	}
	if v, ok := os.LookupEnv("BUFFER_CAPACITY"); ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Buffer.Capacity = n
		}
	}
	if v, ok := os.LookupEnv("EVENTMETRICS_BUFFER_FLUSH_INTERVAL"); ok && v != "" {
		// accept either duration string ("100ms") or integer ms ("100")
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Buffer.FlushIntervalDuration = d
			cfg.Buffer.FlushInterval = int(d / time.Millisecond)
		} else if n, err := strconv.Atoi(v); err == nil {
			cfg.Buffer.FlushInterval = n
			cfg.Buffer.FlushIntervalDuration = time.Duration(n) * time.Millisecond
		}
	}
	if v, ok := os.LookupEnv("EVENTMETRICS_BUFFER_FLUSH_BATCH_SIZE"); ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Buffer.FlushBatchSize = n
		}
	}
	if v, ok := os.LookupEnv("EVENTMETRICS_BUFFER_FLUSH_TIMEOUT"); ok && v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Buffer.FlushTimeout = d
			cfg.Buffer.FlushTimeoutMs = int(d / time.Millisecond)
		} else if n, err := strconv.Atoi(v); err == nil {
			cfg.Buffer.FlushTimeoutMs = n
			cfg.Buffer.FlushTimeout = time.Duration(n) * time.Millisecond
		}
	}
	if v, ok := os.LookupEnv("EVENTMETRICS_BUFFER_FLUSH_RETRIES"); ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Buffer.FlushRetries = n
		}
	}

	// NATS
	// NATS URL (support TDD and legacy)
	if v, ok := os.LookupEnv("EVENTMETRICS_NATS_URL"); ok && v != "" {
		cfg.NATS.URL = v
	}
	if v, ok := os.LookupEnv("NATS_URL"); ok && v != "" {
		cfg.NATS.URL = v
	}
	if v, ok := os.LookupEnv("EVENTMETRICS_NATS_STREAM_NAME"); ok && v != "" {
		cfg.NATS.StreamName = v
	}
	if v, ok := os.LookupEnv("EVENTMETRICS_NATS_SUBJECT"); ok && v != "" {
		cfg.NATS.Subject = v
	}
	if v, ok := os.LookupEnv("EVENTMETRICS_NATS_MAX_BYTES"); ok && v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			cfg.NATS.MaxBytes = n
		}
	}
	if v, ok := os.LookupEnv("EVENTMETRICS_NATS_REPLICAS"); ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.NATS.Replicas = n
		}
	}
	if v, ok := os.LookupEnv("EVENTMETRICS_NATS_PUBLISH_TIMEOUT"); ok && v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.NATS.PublishTimeout = d
		}
	}

	// Consumer
	if v, ok := os.LookupEnv("EVENTMETRICS_CONSUMER_DURABLE_NAME"); ok && v != "" {
		cfg.Consumer.DurableName = v
	}
	if v, ok := os.LookupEnv("EVENTMETRICS_CONSUMER_ACK_WAIT"); ok && v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Consumer.AckWait = d
		}
	}
	if v, ok := os.LookupEnv("EVENTMETRICS_CONSUMER_MAX_DELIVER"); ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Consumer.MaxDeliver = n
		}
	}
	if v, ok := os.LookupEnv("EVENTMETRICS_CONSUMER_MAX_ACK_PENDING"); ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Consumer.MaxAckPending = n
		}
	}
	if v, ok := os.LookupEnv("EVENTMETRICS_CONSUMER_BATCH_SIZE"); ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Consumer.BatchSize = n
			cfg.Consumer.PullBatchSize = n
		}
	}
	if v, ok := os.LookupEnv("EVENTMETRICS_CONSUMER_BATCH_TIMEOUT"); ok && v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Consumer.BatchTimeout = d
		}
	}
	if v, ok := os.LookupEnv("EVENTMETRICS_CONSUMER_FLUSH_TIMEOUT"); ok && v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Consumer.FlushTimeout = d
		}
	}
	if v, ok := os.LookupEnv("EVENTMETRICS_CONSUMER_HEALTH_PORT"); ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Consumer.HealthPort = n
		}
	}

	// ClickHouse
	if v, ok := os.LookupEnv("EVENTMETRICS_CLICKHOUSE_ADDR"); ok && v != "" {
		cfg.ClickHouse.Addr = v
	}
	if v, ok := os.LookupEnv("EVENTMETRICS_CLICKHOUSE_DATABASE"); ok && v != "" {
		cfg.ClickHouse.Database = v
	}
	if v, ok := os.LookupEnv("EVENTMETRICS_CLICKHOUSE_USERNAME"); ok && v != "" {
		cfg.ClickHouse.Username = v
	}
	if v, ok := os.LookupEnv("EVENTMETRICS_CLICKHOUSE_PASSWORD"); ok && v != "" {
		cfg.ClickHouse.Password = v
	}
	if v, ok := os.LookupEnv("EVENTMETRICS_CLICKHOUSE_QUERY_TIMEOUT"); ok && v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.ClickHouse.QueryTimeout = d
		}
	}

	// Validation
	if v, ok := os.LookupEnv("EVENTMETRICS_VALIDATION_MAX_FUTURE"); ok && v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Validation.MaxFutureDuration = d
			cfg.Validation.MaxFutureSeconds = int(d / time.Second)
		}
	}
	if v, ok := os.LookupEnv("EVENTMETRICS_VALIDATION_MAX_PAST"); ok && v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Validation.MaxPastDuration = d
			cfg.Validation.MaxPastSeconds = int(d / time.Second)
		}
	}

	// Compatibility: populate seconds-based legacy fields if not explicitly set
	if cfg.Validation.MaxFutureSeconds == 0 {
		cfg.Validation.MaxFutureSeconds = int(cfg.Validation.MaxFutureDuration / time.Second)
	}
	if cfg.Validation.MaxPastSeconds == 0 {
		cfg.Validation.MaxPastSeconds = int(cfg.Validation.MaxPastDuration / time.Second)
	}

	// Validation checks (basic ranges from TDD)
	if cfg.Server.Port < 1 || cfg.Server.Port > 65535 {
		return Config{}, errors.New("server port out of range (1-65535)")
	}
	if cfg.Buffer.Capacity <= 0 {
		return Config{}, ErrInvalidRange
	}
	if cfg.Buffer.FlushBatchSize < 1 || cfg.Buffer.FlushBatchSize > 10000 {
		return Config{}, errors.New("buffer flush_batch_size out of range (1-10000)")
	}
	if cfg.Buffer.FlushRetries < 0 || cfg.Buffer.FlushRetries > 10 {
		return Config{}, errors.New("buffer flush_retries out of range (0-10)")
	}

	// Strategy-specific validations
	if cfg.BufferStrategy == "nats" || cfg.BufferStrategy == "nats-embedded" {
		if cfg.NATS.URL == "" {
			return Config{}, ErrMissingNatsURL
		}
	}

	return cfg, nil
}
