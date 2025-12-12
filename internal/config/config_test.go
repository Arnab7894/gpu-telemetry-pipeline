package config

import (
	"os"
	"testing"
	"time"
)

func TestLoad_Defaults(t *testing.T) {
	os.Clearenv()
	config, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if config.Server.Port != 8080 {
		t.Errorf("Expected port 8080, got %d", config.Server.Port)
	}
}

func TestValidate_ValidConfig(t *testing.T) {
	config := &Config{
		Server: ServerConfig{Port: 8080},
		Queue: QueueConfig{BufferSize: 1000},
		Collector: CollectorConfig{BatchSize: 100},
	}
	if err := config.Validate(); err != nil {
		t.Errorf("Validate() error = %v", err)
	}
}

func TestValidate_InvalidPort(t *testing.T) {
	config := &Config{
		Server: ServerConfig{Port: -1},
		Queue: QueueConfig{BufferSize: 1000},
		Collector: CollectorConfig{BatchSize: 100},
	}
	if err := config.Validate(); err == nil {
		t.Error("Expected error for invalid port")
	}
}

func TestGetEnv(t *testing.T) {
	os.Clearenv()
	result := getEnv("TEST_KEY", "default")
	if result != "default" {
		t.Errorf("Expected 'default', got '%s'", result)
	}
}

func TestGetEnvAsInt(t *testing.T) {
	os.Clearenv()
	result := getEnvAsInt("TEST_INT", 42)
	if result != 42 {
		t.Errorf("Expected 42, got %d", result)
	}
}

func TestGetEnvAsBool(t *testing.T) {
	os.Clearenv()
	result := getEnvAsBool("TEST_BOOL", true)
	if result != true {
		t.Errorf("Expected true, got %v", result)
	}
}

func TestGetEnvAsDuration(t *testing.T) {
	os.Clearenv()
	result := getEnvAsDuration("TEST_DUR", 5*time.Second)
	if result != 5*time.Second {
		t.Errorf("Expected 5s, got %v", result)
	}
}
