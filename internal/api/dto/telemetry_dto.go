package dto

import "time"

// TelemetryResponse represents a single telemetry metric in API responses
// Decouples internal domain.TelemetryPoint from API contract
type TelemetryResponse struct {
	GPUUUID    string    `json:"gpu_uuid" example:"GPU-5fd4f087-86f3-7a43-b711-4771313afc50"`
	MetricName string    `json:"metric_name" example:"DCGM_FI_DEV_GPU_UTIL"`
	Value      string    `json:"value" example:"85"`
	Timestamp  time.Time `json:"timestamp" example:"2025-01-18T12:34:56.789Z"`
	LabelsRaw  string    `json:"labels_raw,omitempty" example:"DCGM_FI_DRIVER_VERSION=\"535.129.03\""`
}

// TelemetryListResponse wraps a list of telemetry metrics with metadata
type TelemetryListResponse struct {
	Metrics   []*TelemetryResponse `json:"metrics"`
	Total     int                  `json:"total" example:"150"`
	GPUUUID   string               `json:"gpu_uuid" example:"GPU-5fd4f087-86f3-7a43-b711-4771313afc50"`
	StartTime *time.Time           `json:"start_time,omitempty" example:"2025-01-18T00:00:00Z"`
	EndTime   *time.Time           `json:"end_time,omitempty" example:"2025-01-18T23:59:59Z"`
}
