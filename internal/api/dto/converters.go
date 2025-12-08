package dto

import (
	"github.com/arnabghosh/gpu-metrics-streamer/internal/domain"
)

// ToGPUResponse converts domain.GPU to dto.GPUResponse
func ToGPUResponse(gpu *domain.GPU) *GPUResponse {
	if gpu == nil {
		return nil
	}

	return &GPUResponse{
		UUID:      gpu.UUID,
		DeviceID:  gpu.DeviceID,
		GPUIndex:  gpu.GPUIndex,
		ModelName: gpu.ModelName,
		Hostname:  gpu.Hostname,
		Container: gpu.Container,
		Pod:       gpu.Pod,
		Namespace: gpu.Namespace,
	}
}

// ToGPUListResponse converts a slice of domain.GPU to dto.GPUListResponse
func ToGPUListResponse(gpus []*domain.GPU) *GPUListResponse {
	responses := make([]*GPUResponse, 0, len(gpus))
	for _, gpu := range gpus {
		responses = append(responses, ToGPUResponse(gpu))
	}

	return &GPUListResponse{
		GPUs:  responses,
		Total: len(responses),
	}
}

// ToTelemetryResponse converts domain.TelemetryPoint to dto.TelemetryResponse
func ToTelemetryResponse(point *domain.TelemetryPoint) *TelemetryResponse {
	if point == nil {
		return nil
	}

	return &TelemetryResponse{
		GPUUUID:    point.GPUUUID,
		MetricName: point.MetricName,
		Value:      point.Value,
		Timestamp:  point.Timestamp,
		LabelsRaw:  point.LabelsRaw,
	}
}

// ToTelemetryListResponse converts a slice of domain.TelemetryPoint to dto.TelemetryListResponse
func ToTelemetryListResponse(points []*domain.TelemetryPoint, gpuUUID string, filter interface{}) *TelemetryListResponse {
	responses := make([]*TelemetryResponse, 0, len(points))
	for _, point := range points {
		responses = append(responses, ToTelemetryResponse(point))
	}

	response := &TelemetryListResponse{
		Metrics: responses,
		Total:   len(responses),
		GPUUUID: gpuUUID,
	}

	// Add filter information if available
	// Note: filter parameter can be extended to include time range info

	return response
}
