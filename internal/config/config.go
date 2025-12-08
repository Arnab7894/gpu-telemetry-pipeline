package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds the application configuration
type Config struct {
	Server    ServerConfig
	Queue     QueueConfig
	Storage   StorageConfig
	Collector CollectorConfig
	Telemetry TelemetryConfig
}

// ServerConfig holds server configuration
type ServerConfig struct {
	Host         string
	Port         int
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

// QueueConfig holds queue configuration
type QueueConfig struct {
	Type       string
	BufferSize int
}

// StorageConfig holds storage configuration
type StorageConfig struct {
	Type string
}

// CollectorConfig holds collector configuration
type CollectorConfig struct {
	CSVPath        string
	PollInterval   time.Duration
	BatchSize      int
}

// TelemetryConfig holds telemetry configuration
type TelemetryConfig struct {
	Enabled bool
	Topic   string
}

// Load loads configuration from environment variables
func Load() (*Config, error) {
	config := &Config{
		Server: ServerConfig{
			Host:         getEnv("SERVER_HOST", "0.0.0.0"),
			Port:         getEnvAsInt("SERVER_PORT", 8080),
			ReadTimeout:  getEnvAsDuration("SERVER_READ_TIMEOUT", 10*time.Second),
			WriteTimeout: getEnvAsDuration("SERVER_WRITE_TIMEOUT", 10*time.Second),
		},
		Queue: QueueConfig{
			Type:       getEnv("QUEUE_TYPE", "inmemory"),
			BufferSize: getEnvAsInt("QUEUE_BUFFER_SIZE", 1000),
		},
		Storage: StorageConfig{
			Type: getEnv("STORAGE_TYPE", "inmemory"),
		},
		Collector: CollectorConfig{
			CSVPath:      getEnv("CSV_PATH", "data/metrics.csv"),
			PollInterval: getEnvAsDuration("POLL_INTERVAL", 5*time.Second),
			BatchSize:    getEnvAsInt("BATCH_SIZE", 100),
		},
		Telemetry: TelemetryConfig{
			Enabled: getEnvAsBool("TELEMETRY_ENABLED", true),
			Topic:   getEnv("TELEMETRY_TOPIC", "gpu.metrics"),
		},
	}

	return config, nil
}

// getEnv gets an environment variable or returns a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvAsInt gets an environment variable as int or returns a default value
func getEnvAsInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

// getEnvAsBool gets an environment variable as bool or returns a default value
func getEnvAsBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}

// getEnvAsDuration gets an environment variable as duration or returns a default value
func getEnvAsDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", c.Server.Port)
	}

	if c.Queue.BufferSize <= 0 {
		return fmt.Errorf("invalid queue buffer size: %d", c.Queue.BufferSize)
	}

	if c.Collector.BatchSize <= 0 {
		return fmt.Errorf("invalid collector batch size: %d", c.Collector.BatchSize)
	}

	return nil
}
