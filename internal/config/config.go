package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
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
	// BufferStrategy is the active buffer strategy; kept for compatibility but only "memory" is supported.
	BufferStrategy string

	Server     ServerConfig
	Buffer     BufferConfig
	ClickHouse ClickHouseConfig
	Validation ValidationConfig
}

var (
	ErrInvalidRange   = errors.New("invalid range in config")
)

// envAny returns the first non-empty environment variable value for the given keys.
// Usage: if v, ok := envAny("CHRONOMETRICS_FOO", "NATS_URL"); ok { ... }
func envAny(keys ...string) (string, bool) {
	for _, k := range keys {
		if v, ok := os.LookupEnv(k); ok && v != "" {
			return v, true
		}
	}
	return "", false
}

// Load reads configuration from environment, applies defaults, and validates.
// Returns a value (non-pointer) for minimal churn with existing callers.
func Load() (Config, error) {
	// Load environment file (if present) to populate env vars for local dev.
	// Do not override already-set environment variables.
	if err := loadDotEnv(".env"); err != nil {
		// Non-fatal if .env is missing; return error only for parse failures.
		if !os.IsNotExist(err) {
			return Config{}, fmt.Errorf("load .env: %w", err)
		}
	}
		// defaults from TDD
	cfg := Config{
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
		// NATS and consumer configuration removed — this build supports memory-only strategy.
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
	if v, ok := envAny("CHRONOMETRICS_BUFFER_STRATEGY"); ok {
		cfg.BufferStrategy = v
	}
	if v, ok := os.LookupEnv("BUFFER_STRATEGY"); ok && v != "" {
		cfg.BufferStrategy = v
	}

	// Server port (support TDD and legacy env var)
	if v, ok := envAny("CHRONOMETRICS_SERVER_PORT"); ok && v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.Server.Port = p
		}
	}
	if v, ok := os.LookupEnv("SERVER_PORT"); ok && v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.Server.Port = p
		}
	}

	if v, ok := envAny("CHRONOMETRICS_SERVER_READ_TIMEOUT"); ok && v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Server.ReadTimeout = d
		}
	}
	if v, ok := envAny("CHRONOMETRICS_SERVER_WRITE_TIMEOUT"); ok && v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Server.WriteTimeout = d
		}
	}
	if v, ok := envAny("CHRONOMETRICS_SERVER_IDLE_TIMEOUT"); ok && v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Server.IdleTimeout = d
		}
	}
	if v, ok := envAny("CHRONOMETRICS_SERVER_BODY_LIMIT"); ok && v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			cfg.Server.BodyLimit = n
		}
	}
	if v, ok := envAny("CHRONOMETRICS_SERVER_SHUTDOWN_TIMEOUT"); ok && v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Server.ShutdownTimeout = d
		}
	}

	// Buffer (memory) envs
	// Buffer capacity (support TDD and legacy names)
	if v, ok := envAny("CHRONOMETRICS_BUFFER_CAPACITY"); ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Buffer.Capacity = n
		}
	}
	if v, ok := os.LookupEnv("BUFFER_CAPACITY"); ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Buffer.Capacity = n
		}
	}
	if v, ok := envAny("CHRONOMETRICS_BUFFER_FLUSH_INTERVAL"); ok && v != "" {
		// accept either duration string ("100ms") or integer ms ("100")
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Buffer.FlushIntervalDuration = d
			cfg.Buffer.FlushInterval = int(d / time.Millisecond)
		} else if n, err := strconv.Atoi(v); err == nil {
			cfg.Buffer.FlushInterval = n
			cfg.Buffer.FlushIntervalDuration = time.Duration(n) * time.Millisecond
		}
	}
	if v, ok := envAny("CHRONOMETRICS_BUFFER_FLUSH_BATCH_SIZE"); ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Buffer.FlushBatchSize = n
		}
	}
	if v, ok := envAny("CHRONOMETRICS_BUFFER_FLUSH_TIMEOUT"); ok && v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Buffer.FlushTimeout = d
			cfg.Buffer.FlushTimeoutMs = int(d / time.Millisecond)
		} else if n, err := strconv.Atoi(v); err == nil {
			cfg.Buffer.FlushTimeoutMs = n
			cfg.Buffer.FlushTimeout = time.Duration(n) * time.Millisecond
		}
	}
	if v, ok := envAny("CHRONOMETRICS_BUFFER_FLUSH_RETRIES"); ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Buffer.FlushRetries = n
		}
	}

	// NATS and consumer env parsing removed (memory-only).

	// ClickHouse
	if v, ok := envAny("CHRONOMETRICS_CLICKHOUSE_ADDR"); ok && v != "" {
		cfg.ClickHouse.Addr = v
	}
	if v, ok := envAny("CHRONOMETRICS_CLICKHOUSE_DATABASE"); ok && v != "" {
		cfg.ClickHouse.Database = v
	}
	if v, ok := envAny("CHRONOMETRICS_CLICKHOUSE_USERNAME"); ok && v != "" {
		cfg.ClickHouse.Username = v
	}
	if v, ok := envAny("CHRONOMETRICS_CLICKHOUSE_PASSWORD"); ok && v != "" {
		cfg.ClickHouse.Password = v
	}
	if v, ok := envAny("CHRONOMETRICS_CLICKHOUSE_QUERY_TIMEOUT"); ok && v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.ClickHouse.QueryTimeout = d
		}
	}

	// Validation
	if v, ok := envAny("CHRONOMETRICS_VALIDATION_MAX_FUTURE"); ok && v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Validation.MaxFutureDuration = d
			cfg.Validation.MaxFutureSeconds = int(d / time.Second)
		}
	}
	if v, ok := envAny("CHRONOMETRICS_VALIDATION_MAX_PAST"); ok && v != "" {
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

	// Required environment variables: fail fast if missing so app init cannot proceed with implicit defaults.
	required := []string{
		"CHRONOMETRICS_CLICKHOUSE_ADDR",
		"CHRONOMETRICS_CLICKHOUSE_DATABASE",
		"CHRONOMETRICS_CLICKHOUSE_USERNAME",
	}
	var missing []string
	for _, k := range required {
		if v, _ := os.LookupEnv(k); v == "" {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
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

	// Only memory strategy is supported in this codebase iteration. Other strategies removed.

	return cfg, nil
}

// loadDotEnv reads a simple KEY=VALUE file and sets any variables that are not already present in the environment.
// Lines starting with '#' are comments. Blank lines are ignored. Keys and values are trimmed of surrounding whitespace.
func loadDotEnv(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Split at first '='
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid line in %s: %q", path, line)
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		// Remove surrounding quotes if present
		if len(val) >= 2 && ((val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'')) {
			val = val[1 : len(val)-1]
		}
		if key == "" {
			return fmt.Errorf("invalid key in %s: %q", path, line)
		}
		if cur, ok := os.LookupEnv(key); !ok || cur == "" {
			_ = os.Setenv(key, val)
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	return nil
}
