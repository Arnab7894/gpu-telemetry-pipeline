package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTelemetryPoint_GetUniqueKey(t *testing.T) {
	timestamp := time.Date(2025, 1, 18, 12, 30, 45, 123456789, time.UTC)
	telemetry := &TelemetryPoint{
		GPUUUID:    "GPU-001",
		MetricName: "DCGM_FI_DEV_GPU_UTIL",
		Value:      "85.5",
		Timestamp:  timestamp,
	}

	key := telemetry.GetUniqueKey()
	expected := "GPU-001:DCGM_FI_DEV_GPU_UTIL:" + timestamp.Format(time.RFC3339Nano)
	assert.Equal(t, expected, key)
	assert.Contains(t, key, "GPU-001")
	assert.Contains(t, key, "DCGM_FI_DEV_GPU_UTIL")
	assert.Contains(t, key, "2025-01-18")
}

func TestTelemetryPoint_GetUniqueKey_DifferentMetrics(t *testing.T) {
	timestamp := time.Now()
	
	telemetry1 := &TelemetryPoint{
		GPUUUID:    "GPU-001",
		MetricName: "DCGM_FI_DEV_GPU_UTIL",
		Timestamp:  timestamp,
	}
	
	telemetry2 := &TelemetryPoint{
		GPUUUID:    "GPU-001",
		MetricName: "DCGM_FI_DEV_MEM_UTIL",
		Timestamp:  timestamp,
	}

	key1 := telemetry1.GetUniqueKey()
	key2 := telemetry2.GetUniqueKey()
	
	assert.NotEqual(t, key1, key2, "Different metrics should have different keys")
}
