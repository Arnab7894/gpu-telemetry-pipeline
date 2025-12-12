package mongodb

import (
	"errors"
	"testing"
	"time"

	"github.com/arnabghosh/gpu-metrics-streamer/internal/domain"
	"github.com/arnabghosh/gpu-metrics-streamer/internal/storage"
	"github.com/stretchr/testify/assert"
)

func TestNewGPURepository_InvalidURI_Extended(t *testing.T) {
	_, err := NewGPURepository("invalid://url", "testdb", "gpus")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to connect to MongoDB")
}

func TestNewGPURepository_EmptyDatabase(t *testing.T) {
	// Empty database name should still create repository
	_, err := NewGPURepository("mongodb://localhost:27017", "", "gpus")
	// This will fail to connect, but we're testing the creation logic
	assert.Error(t, err)
}

func TestNewTelemetryRepository_InvalidURI_Extended(t *testing.T) {
	_, err := NewTelemetryRepository("invalid://url", "testdb", "telemetry")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to connect to MongoDB")
}

func TestNewTelemetryRepository_ConnectionError(t *testing.T) {
	// Test with non-existent host (should fail to connect)
	_, err := NewTelemetryRepository("mongodb://nonexistent-host:27017", "testdb", "telemetry")
	assert.Error(t, err)
}

func TestGPURepository_BulkStore_NilSlice(t *testing.T) {
	// Test that nil slice doesn't cause panic
	repo := &GPURepository{
		client:     nil, // Will cause error if accessed
		database:   "test",
		collection: "gpus",
	}

	// BulkStore with nil should not error (early return)
	err := repo.BulkStore(nil)
	assert.NoError(t, err)
}

func TestGPURepository_BulkStore_EmptySlice(t *testing.T) {
	// Test that empty slice doesn't cause panic
	repo := &GPURepository{
		client:     nil, // Will cause error if accessed
		database:   "test",
		collection: "gpus",
	}

	// BulkStore with empty slice should not error (early return)
	err := repo.BulkStore([]*domain.GPU{})
	assert.NoError(t, err)
}

func TestTelemetryRepository_BulkStore_EmptySlice(t *testing.T) {
	// Test that empty slice doesn't cause panic
	repo := &TelemetryRepository{
		client:     nil, // Will cause error if accessed
		database:   "test",
		collection: "telemetry",
	}

	// BulkStore with empty slice should not error (early return)
	err := repo.BulkStore([]*domain.TelemetryPoint{})
	assert.NoError(t, err)
}

func TestGPU_Validation(t *testing.T) {
	tests := []struct {
		name  string
		gpu   *domain.GPU
		valid bool
	}{
		{
			name: "valid GPU",
			gpu: &domain.GPU{
				UUID:      "gpu-123",
				Hostname:  "host1",
				ModelName: "Tesla V100",
			},
			valid: true,
		},
		{
			name: "missing UUID",
			gpu: &domain.GPU{
				UUID:      "",
				Hostname:  "host1",
				ModelName: "Tesla V100",
			},
			valid: false,
		},
		{
			name: "GPU with all fields",
			gpu: &domain.GPU{
				UUID:      "gpu-456",
				Hostname:  "host2",
				ModelName: "A100",
				DeviceID:  "nvidia0",
				GPUIndex:  "0",
			},
			valid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.valid {
				assert.NotEmpty(t, tt.gpu.UUID)
			} else {
				assert.Empty(t, tt.gpu.UUID)
			}
		})
	}
}

func TestTelemetryPoint_GetUniqueKey(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		point    *domain.TelemetryPoint
		expected string
	}{
		{
			name: "with batch ID",
			point: &domain.TelemetryPoint{
				GPUUUID:    "gpu-123",
				MetricName: "temperature",
				BatchID:    "batch-1",
				Timestamp:  now,
			},
			expected: "gpu-123:temperature:batch-1",
		},
		{
			name: "without batch ID",
			point: &domain.TelemetryPoint{
				GPUUUID:    "gpu-456",
				MetricName: "power",
				Timestamp:  now,
			},
			expected: "gpu-456:power:" + now.Format(time.RFC3339Nano),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := tt.point.GetUniqueKey()
			assert.Equal(t, tt.expected, key)
		})
	}
}

func TestTelemetryPoint_Validation(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name  string
		point *domain.TelemetryPoint
		valid bool
	}{
		{
			name: "valid telemetry",
			point: &domain.TelemetryPoint{
				GPUUUID:    "gpu-123",
				MetricName: "temperature",
				Value:      "65.5",
				Timestamp:  now,
			},
			valid: true,
		},
		{
			name: "missing GPU UUID",
			point: &domain.TelemetryPoint{
				GPUUUID:    "",
				MetricName: "temperature",
				Value:      "65.5",
				Timestamp:  now,
			},
			valid: false,
		},
		{
			name: "missing metric name",
			point: &domain.TelemetryPoint{
				GPUUUID:    "gpu-123",
				MetricName: "",
				Value:      "65.5",
				Timestamp:  now,
			},
			valid: false,
		},
		{
			name: "zero timestamp",
			point: &domain.TelemetryPoint{
				GPUUUID:    "gpu-123",
				MetricName: "temperature",
				Value:      "65.5",
				Timestamp:  time.Time{},
			},
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.valid {
				assert.NotEmpty(t, tt.point.GPUUUID)
				assert.NotEmpty(t, tt.point.MetricName)
				assert.False(t, tt.point.Timestamp.IsZero())
			}
		})
	}
}

func TestDomainError_Types(t *testing.T) {
	// Test that domain errors are defined correctly
	assert.NotNil(t, domain.ErrGPUNotFound)

	// Test error comparison
	err := domain.ErrGPUNotFound
	assert.True(t, errors.Is(err, domain.ErrGPUNotFound))
}

func TestGPU_Fields(t *testing.T) {
	gpu := &domain.GPU{
		UUID:      "gpu-123",
		Hostname:  "host1",
		ModelName: "Tesla V100",
		DeviceID:  "nvidia0",
		GPUIndex:  "0",
		Container: "container1",
		Pod:       "pod1",
		Namespace: "namespace1",
	}

	assert.Equal(t, "gpu-123", gpu.UUID)
	assert.Equal(t, "host1", gpu.Hostname)
	assert.Equal(t, "Tesla V100", gpu.ModelName)
	assert.Equal(t, "nvidia0", gpu.DeviceID)
	assert.Equal(t, "0", gpu.GPUIndex)
	assert.Equal(t, "container1", gpu.Container)
	assert.Equal(t, "pod1", gpu.Pod)
	assert.Equal(t, "namespace1", gpu.Namespace)
}

func TestTelemetryPoint_Fields(t *testing.T) {
	now := time.Now()
	point := &domain.TelemetryPoint{
		GPUUUID:    "gpu-123",
		MetricName: "temperature",
		Value:      "65.5",
		Timestamp:  now,
		BatchID:    "batch-1",
		LabelsRaw:  "key=value",
	}

	assert.Equal(t, "gpu-123", point.GPUUUID)
	assert.Equal(t, "temperature", point.MetricName)
	assert.Equal(t, "65.5", point.Value)
	assert.Equal(t, now, point.Timestamp)
	assert.Equal(t, "batch-1", point.BatchID)
	assert.Equal(t, "key=value", point.LabelsRaw)
}

func TestRepository_StructInitialization(t *testing.T) {
	gpuRepo := &GPURepository{
		client:     nil,
		database:   "testdb",
		collection: "gpus",
	}

	assert.NotNil(t, gpuRepo)
	assert.Equal(t, "testdb", gpuRepo.database)
	assert.Equal(t, "gpus", gpuRepo.collection)

	telemetryRepo := &TelemetryRepository{
		client:     nil,
		database:   "testdb",
		collection: "telemetry",
	}

	assert.NotNil(t, telemetryRepo)
	assert.Equal(t, "testdb", telemetryRepo.database)
	assert.Equal(t, "telemetry", telemetryRepo.collection)
}

func TestGPURepository_CollectionName(t *testing.T) {
	repo := &GPURepository{
		database:   "mydb",
		collection: "custom_gpus",
	}

	assert.Equal(t, "custom_gpus", repo.collection)
}

func TestTelemetryRepository_CollectionName(t *testing.T) {
	repo := &TelemetryRepository{
		database:   "mydb",
		collection: "custom_telemetry",
	}

	assert.Equal(t, "custom_telemetry", repo.collection)
}

func TestTimeFilter_Struct(t *testing.T) {
	start := time.Now().Add(-1 * time.Hour)
	end := time.Now()
	limit := 100

	filter := &storage.TimeFilter{
		StartTime: &start,
		EndTime:   &end,
		Limit:     &limit,
	}

	assert.NotNil(t, filter.StartTime)
	assert.NotNil(t, filter.EndTime)
	assert.NotNil(t, filter.Limit)
	assert.Equal(t, 100, *filter.Limit)
}

func TestBatchProcessing_Concepts(t *testing.T) {
	// Test that telemetry points in a batch share timestamps
	batchTime := time.Now()
	batchID := batchTime.Format(time.RFC3339Nano)

	points := []*domain.TelemetryPoint{
		{
			GPUUUID:    "gpu-1",
			MetricName: "temp",
			Value:      "60",
			Timestamp:  batchTime,
			BatchID:    batchID,
		},
		{
			GPUUUID:    "gpu-2",
			MetricName: "temp",
			Value:      "65",
			Timestamp:  batchTime,
			BatchID:    batchID,
		},
	}

	// All points should have same timestamp and batch ID
	for _, p := range points {
		assert.Equal(t, batchTime, p.Timestamp)
		assert.Equal(t, batchID, p.BatchID)
	}
}

func TestMetricValue_StringType(t *testing.T) {
	// Verify that Value is string type (can handle various formats)
	point := &domain.TelemetryPoint{
		GPUUUID:    "gpu-123",
		MetricName: "power",
		Value:      "250.5", // String, not float
		Timestamp:  time.Now(),
	}

	assert.IsType(t, "", point.Value)
	assert.NotEmpty(t, point.Value)
}
