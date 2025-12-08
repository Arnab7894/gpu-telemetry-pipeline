package dto

import (
	"testing"
	"time"

	"github.com/arnabghosh/gpu-metrics-streamer/internal/domain"
	"github.com/stretchr/testify/assert"
)

func TestToGPUResponse(t *testing.T) {
	gpu := &domain.GPU{
		UUID:      "GPU-001",
		DeviceID:  "nvidia0",
		GPUIndex:  "0",
		ModelName: "Tesla V100",
		Hostname:  "host-001",
		Container: "my-container",
		Pod:       "my-pod",
		Namespace: "ml-team",
	}

	response := ToGPUResponse(gpu)

	assert.Equal(t, "GPU-001", response.UUID)
	assert.Equal(t, "nvidia0", response.DeviceID)
	assert.Equal(t, "0", response.GPUIndex)
	assert.Equal(t, "Tesla V100", response.ModelName)
	assert.Equal(t, "host-001", response.Hostname)
	assert.Equal(t, "my-container", response.Container)
	assert.Equal(t, "my-pod", response.Pod)
	assert.Equal(t, "ml-team", response.Namespace)
}

func TestToGPUListResponse(t *testing.T) {
	gpus := []*domain.GPU{
		{
			UUID:      "GPU-001",
			DeviceID:  "nvidia0",
			ModelName: "Tesla V100",
			Hostname:  "host-001",
		},
		{
			UUID:      "GPU-002",
			DeviceID:  "nvidia1",
			ModelName: "Tesla P100",
			Hostname:  "host-002",
		},
	}

	response := ToGPUListResponse(gpus)

	assert.Equal(t, 2, response.Total)
	assert.Len(t, response.GPUs, 2)
	assert.Equal(t, "GPU-001", response.GPUs[0].UUID)
	assert.Equal(t, "GPU-002", response.GPUs[1].UUID)
}

func TestToGPUListResponse_Empty(t *testing.T) {
	gpus := []*domain.GPU{}
	response := ToGPUListResponse(gpus)

	assert.Equal(t, 0, response.Total)
	assert.Len(t, response.GPUs, 0)
}

func TestToTelemetryResponse(t *testing.T) {
	timestamp := time.Date(2025, 1, 18, 12, 0, 0, 0, time.UTC)
	telemetry := &domain.TelemetryPoint{
		GPUUUID:    "GPU-001",
		MetricName: "DCGM_FI_DEV_GPU_UTIL",
		Value:      "85.5",
		Timestamp:  timestamp,
		LabelsRaw:  "Hostname=\"host-001\"",
	}

	response := ToTelemetryResponse(telemetry)

	assert.Equal(t, "GPU-001", response.GPUUUID)
	assert.Equal(t, "DCGM_FI_DEV_GPU_UTIL", response.MetricName)
	assert.Equal(t, "85.5", response.Value)
	assert.Equal(t, timestamp, response.Timestamp)
	assert.Equal(t, "Hostname=\"host-001\"", response.LabelsRaw)
}

func TestToTelemetryListResponse(t *testing.T) {
	baseTime := time.Date(2025, 1, 18, 12, 0, 0, 0, time.UTC)

	telemetry := []*domain.TelemetryPoint{
		{
			GPUUUID:    "GPU-001",
			MetricName: "DCGM_FI_DEV_GPU_UTIL",
			Value:      "85.5",
			Timestamp:  baseTime,
		},
		{
			GPUUUID:    "GPU-001",
			MetricName: "DCGM_FI_DEV_MEM_UTIL",
			Value:      "70.0",
			Timestamp:  baseTime.Add(30 * time.Second),
		},
	}

	response := ToTelemetryListResponse(telemetry, "GPU-001", nil)

	assert.Equal(t, "GPU-001", response.GPUUUID)
	assert.Equal(t, 2, response.Total)
	assert.Len(t, response.Metrics, 2)
	assert.Equal(t, "85.5", response.Metrics[0].Value)
	assert.Equal(t, "70.0", response.Metrics[1].Value)
}

func TestToTelemetryListResponse_NoTimeFilter(t *testing.T) {
	telemetry := []*domain.TelemetryPoint{
		{
			GPUUUID:    "GPU-001",
			MetricName: "DCGM_FI_DEV_GPU_UTIL",
			Value:      "85.5",
			Timestamp:  time.Now(),
		},
	}

	response := ToTelemetryListResponse(telemetry, "GPU-001", nil)

	assert.Equal(t, "GPU-001", response.GPUUUID)
	assert.Equal(t, 1, response.Total)
}

func TestToTelemetryListResponse_Empty(t *testing.T) {
	telemetry := []*domain.TelemetryPoint{}
	response := ToTelemetryListResponse(telemetry, "GPU-001", nil)

	assert.Equal(t, "GPU-001", response.GPUUUID)
	assert.Equal(t, 0, response.Total)
	assert.Len(t, response.Metrics, 0)
}
