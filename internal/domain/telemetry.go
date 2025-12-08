package domain

import "time"

// TelemetryPoint represents a single telemetry measurement for a GPU
// Maps to CSV columns: metric_name, value, labels_raw, and references GPU via uuid
// Note: CSV timestamp is IGNORED - we use time.Now() at ingestion time
type TelemetryPoint struct {
	// GPUUUID identifies which GPU this telemetry belongs to (CSV: uuid)
	// This is used to query: /api/v1/gpus/{uuid}/telemetry
	GPUUUID string `json:"gpu_uuid" bson:"gpu_uuid"`

	// MetricName is the type of metric (CSV: metric_name)
	// Examples: "DCGM_FI_DEV_GPU_UTIL", "DCGM_FI_DEV_POWER_USAGE", etc.
	MetricName string `json:"metric_name" bson:"metric_name"`

	// Value is the metric value (CSV: value)
	// Stored as string to preserve precision and handle various metric types
	// Examples: "0", "100", "98.5", "300.794"
	Value string `json:"value" bson:"value"`

	// Timestamp is when the telemetry was INGESTED (NOT from CSV)
	// We use time.Now() when the Streamer publishes the message
	// This allows us to track real-time data flow through the system
	Timestamp time.Time `json:"timestamp" bson:"timestamp"`

	// LabelsRaw contains raw Prometheus-style labels (CSV: labels_raw)
	// Example: 'DCGM_FI_DRIVER_VERSION="535.129.03",Hostname="mtv5-dgx1-hgpu-031",...'
	// Optional metadata for debugging and detailed analysis
	LabelsRaw string `json:"labels_raw,omitempty" bson:"labels_raw,omitempty"`
}

// GetUniqueKey returns a composite key for this telemetry point
// Unique key: GPUUUID + MetricName + Timestamp
// This ensures each telemetry record is uniquely identifiable
func (t *TelemetryPoint) GetUniqueKey() string {
	return t.GPUUUID + ":" + t.MetricName + ":" + t.Timestamp.Format(time.RFC3339Nano)
}

// MetricType represents the various types of GPU metrics we collect
// These correspond to the metric_name values in the CSV
type MetricType string

const (
	// GPU Utilization metrics
	MetricGPUUtil     MetricType = "DCGM_FI_DEV_GPU_UTIL"      // GPU utilization percentage (0-100)
	MetricMemCopyUtil MetricType = "DCGM_FI_DEV_MEM_COPY_UTIL" // Memory copy utilization
	MetricEncUtil     MetricType = "DCGM_FI_DEV_ENC_UTIL"      // Encoder utilization
	MetricDecUtil     MetricType = "DCGM_FI_DEV_DEC_UTIL"      // Decoder utilization

	// Memory metrics
	MetricFBUsed MetricType = "DCGM_FI_DEV_FB_USED" // Framebuffer memory used (bytes)
	MetricFBFree MetricType = "DCGM_FI_DEV_FB_FREE" // Framebuffer memory free (bytes)

	// Clock metrics
	MetricMemClock MetricType = "DCGM_FI_DEV_MEM_CLOCK" // Memory clock speed (MHz)
	MetricSMClock  MetricType = "DCGM_FI_DEV_SM_CLOCK"  // SM clock speed (MHz)

	// Power and temperature metrics
	MetricPowerUsage MetricType = "DCGM_FI_DEV_POWER_USAGE" // Power consumption (Watts)
	MetricGPUTemp    MetricType = "DCGM_FI_DEV_GPU_TEMP"    // GPU temperature (Celsius)
)

// CSVRow represents a raw CSV row from the input file
// This is a helper struct for parsing - not part of the domain model
type CSVRow struct {
	Timestamp  string // CSV column 0 - IGNORED for storage
	MetricName string // CSV column 1
	GPUIndex   string // CSV column 2
	Device     string // CSV column 3
	UUID       string // CSV column 4
	ModelName  string // CSV column 5
	Hostname   string // CSV column 6
	Container  string // CSV column 7
	Pod        string // CSV column 8
	Namespace  string // CSV column 9
	Value      string // CSV column 10
	LabelsRaw  string // CSV column 11
}
