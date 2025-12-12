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
		Server:    ServerConfig{Port: 8080},
		Queue:     QueueConfig{BufferSize: 1000},
		Collector: CollectorConfig{BatchSize: 100},
	}
	if err := config.Validate(); err != nil {
		t.Errorf("Validate() error = %v", err)
	}
}

func TestValidate_InvalidPort(t *testing.T) {
	config := &Config{
		Server:    ServerConfig{Port: -1},
		Queue:     QueueConfig{BufferSize: 1000},
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

// Tests for env.go exported functions
func TestGetEnv_WithValue(t *testing.T) {
	os.Setenv("TEST_ENV_VAR", "test_value")
	defer os.Unsetenv("TEST_ENV_VAR")

	result := GetEnv("TEST_ENV_VAR", "default")
	if result != "test_value" {
		t.Errorf("Expected 'test_value', got '%s'", result)
	}
}

func TestGetEnvInt_WithValue(t *testing.T) {
	os.Setenv("TEST_INT_VAR", "123")
	defer os.Unsetenv("TEST_INT_VAR")

	result := GetEnvInt("TEST_INT_VAR", 42)
	if result != 123 {
		t.Errorf("Expected 123, got %d", result)
	}
}

func TestGetEnvInt_Invalid(t *testing.T) {
	os.Setenv("TEST_INT_INVALID", "not_a_number")
	defer os.Unsetenv("TEST_INT_INVALID")

	result := GetEnvInt("TEST_INT_INVALID", 42)
	if result != 42 {
		t.Errorf("Expected default 42, got %d", result)
	}
}

func TestGetEnvDuration_WithValue(t *testing.T) {
	os.Setenv("TEST_DURATION_VAR", "10s")
	defer os.Unsetenv("TEST_DURATION_VAR")

	result := GetEnvDuration("TEST_DURATION_VAR", 5*time.Second)
	if result != 10*time.Second {
		t.Errorf("Expected 10s, got %v", result)
	}
}

func TestGetEnvDuration_Invalid(t *testing.T) {
	os.Setenv("TEST_DURATION_INVALID", "not_a_duration")
	defer os.Unsetenv("TEST_DURATION_INVALID")

	result := GetEnvDuration("TEST_DURATION_INVALID", 5*time.Second)
	if result != 5*time.Second {
		t.Errorf("Expected default 5s, got %v", result)
	}
}

func TestGetEnvBool_True(t *testing.T) {
	os.Setenv("TEST_BOOL_TRUE", "true")
	defer os.Unsetenv("TEST_BOOL_TRUE")

	result := GetEnvBool("TEST_BOOL_TRUE", false)
	if result != true {
		t.Errorf("Expected true, got %v", result)
	}
}

func TestGetEnvBool_False(t *testing.T) {
	os.Setenv("TEST_BOOL_FALSE", "false")
	defer os.Unsetenv("TEST_BOOL_FALSE")

	result := GetEnvBool("TEST_BOOL_FALSE", true)
	if result != false {
		t.Errorf("Expected false, got %v", result)
	}
}

func TestGetEnvBool_Invalid(t *testing.T) {
	os.Setenv("TEST_BOOL_INVALID", "not_a_bool")
	defer os.Unsetenv("TEST_BOOL_INVALID")

	result := GetEnvBool("TEST_BOOL_INVALID", true)
	if result != true {
		t.Errorf("Expected default true, got %v", result)
	}
}
