package storage

import (
	"time"

	"github.com/arnabghosh/gpu-metrics-streamer/internal/domain"
)

// TimeFilter represents optional time range filters for querying telemetry
type TimeFilter struct {
	StartTime *time.Time
	EndTime   *time.Time
}

// TelemetryRepository defines the interface for telemetry data storage
type TelemetryRepository interface {
	// Store persists a telemetry point
	Store(telemetry *domain.TelemetryPoint) error

	// GetByGPU retrieves all telemetry for a specific GPU, ordered by timestamp
	// Supports optional time filtering
	GetByGPU(gpuUUID string, filter TimeFilter) ([]*domain.TelemetryPoint, error)

	// Count returns the total number of telemetry points stored
	Count() int64
}

// GPURepository defines the interface for GPU data storage
type GPURepository interface {
	// Store persists or updates a GPU record
	Store(gpu *domain.GPU) error

	// GetByUUID retrieves a GPU by its UUID
	GetByUUID(uuid string) (*domain.GPU, error)

	// List returns all GPUs that have telemetry data
	List() ([]*domain.GPU, error)

	// Count returns the total number of GPUs
	Count() int64
}
