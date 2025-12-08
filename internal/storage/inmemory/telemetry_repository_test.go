package inmemory

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/arnabghosh/gpu-metrics-streamer/internal/domain"
	"github.com/arnabghosh/gpu-metrics-streamer/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTelemetryRepository_Store(t *testing.T) {
	repo := NewTelemetryRepository()

	point := &domain.TelemetryPoint{
		GPUUUID:    "GPU-123",
		MetricName: "DCGM_FI_DEV_GPU_UTIL",
		Value:      "75.5",
		Timestamp:  time.Now(),
	}

	err := repo.Store(point)
	require.NoError(t, err)

	assert.Equal(t, int64(1), repo.Count())
}

func TestTelemetryRepository_StoreNil(t *testing.T) {
	repo := NewTelemetryRepository()

	err := repo.Store(nil)
	assert.ErrorIs(t, err, domain.ErrInvalidInput)
}

func TestTelemetryRepository_StoreEmptyGPUUUID(t *testing.T) {
	repo := NewTelemetryRepository()

	point := &domain.TelemetryPoint{
		MetricName: "DCGM_FI_DEV_GPU_UTIL",
		Value:      "75.5",
	}

	err := repo.Store(point)
	assert.ErrorIs(t, err, domain.ErrInvalidInput)
}

func TestTelemetryRepository_GetByGPU_OrderedByTimestamp(t *testing.T) {
	repo := NewTelemetryRepository()

	baseTime := time.Now()

	// Insert in random order
	points := []*domain.TelemetryPoint{
		{GPUUUID: "GPU-1", MetricName: "metric1", Value: "3", Timestamp: baseTime.Add(3 * time.Second)},
		{GPUUUID: "GPU-1", MetricName: "metric2", Value: "1", Timestamp: baseTime.Add(1 * time.Second)},
		{GPUUUID: "GPU-1", MetricName: "metric3", Value: "2", Timestamp: baseTime.Add(2 * time.Second)},
	}

	for _, point := range points {
		repo.Store(point)
	}

	// Retrieve
	results, err := repo.GetByGPU("GPU-1", storage.TimeFilter{})
	require.NoError(t, err)
	assert.Len(t, results, 3)

	// Verify ordered by timestamp (ascending)
	assert.Equal(t, "1", results[0].Value)
	assert.Equal(t, "2", results[1].Value)
	assert.Equal(t, "3", results[2].Value)

	// Verify timestamps are in order
	assert.True(t, results[0].Timestamp.Before(results[1].Timestamp))
	assert.True(t, results[1].Timestamp.Before(results[2].Timestamp))
}

func TestTelemetryRepository_GetByGPU_EmptyUUID(t *testing.T) {
	repo := NewTelemetryRepository()

	results, err := repo.GetByGPU("", storage.TimeFilter{})
	assert.ErrorIs(t, err, domain.ErrInvalidInput)
	assert.Nil(t, results)
}

func TestTelemetryRepository_GetByGPU_NotFound(t *testing.T) {
	repo := NewTelemetryRepository()

	results, err := repo.GetByGPU("non-existent", storage.TimeFilter{})
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestTelemetryRepository_GetByGPU_StartTimeFilter(t *testing.T) {
	repo := NewTelemetryRepository()

	baseTime := time.Now()
	startTime := baseTime.Add(2 * time.Second)

	// Insert points at different times
	points := []*domain.TelemetryPoint{
		{GPUUUID: "GPU-1", Value: "1", Timestamp: baseTime.Add(1 * time.Second)},
		{GPUUUID: "GPU-1", Value: "2", Timestamp: baseTime.Add(2 * time.Second)},
		{GPUUUID: "GPU-1", Value: "3", Timestamp: baseTime.Add(3 * time.Second)},
	}

	for _, point := range points {
		repo.Store(point)
	}

	// Query with start_time filter
	results, err := repo.GetByGPU("GPU-1", storage.TimeFilter{
		StartTime: &startTime,
	})
	require.NoError(t, err)

	// Should only return points >= startTime
	assert.Len(t, results, 2)
	assert.Equal(t, "2", results[0].Value)
	assert.Equal(t, "3", results[1].Value)
}

func TestTelemetryRepository_GetByGPU_EndTimeFilter(t *testing.T) {
	repo := NewTelemetryRepository()

	baseTime := time.Now()
	endTime := baseTime.Add(2 * time.Second)

	// Insert points at different times
	points := []*domain.TelemetryPoint{
		{GPUUUID: "GPU-1", Value: "1", Timestamp: baseTime.Add(1 * time.Second)},
		{GPUUUID: "GPU-1", Value: "2", Timestamp: baseTime.Add(2 * time.Second)},
		{GPUUUID: "GPU-1", Value: "3", Timestamp: baseTime.Add(3 * time.Second)},
	}

	for _, point := range points {
		repo.Store(point)
	}

	// Query with end_time filter
	results, err := repo.GetByGPU("GPU-1", storage.TimeFilter{
		EndTime: &endTime,
	})
	require.NoError(t, err)

	// Should only return points <= endTime
	assert.Len(t, results, 2)
	assert.Equal(t, "1", results[0].Value)
	assert.Equal(t, "2", results[1].Value)
}

func TestTelemetryRepository_GetByGPU_TimeRangeFilter(t *testing.T) {
	repo := NewTelemetryRepository()

	baseTime := time.Now()
	startTime := baseTime.Add(2 * time.Second)
	endTime := baseTime.Add(4 * time.Second)

	// Insert points across a wider time range
	points := []*domain.TelemetryPoint{
		{GPUUUID: "GPU-1", Value: "1", Timestamp: baseTime.Add(1 * time.Second)},
		{GPUUUID: "GPU-1", Value: "2", Timestamp: baseTime.Add(2 * time.Second)},
		{GPUUUID: "GPU-1", Value: "3", Timestamp: baseTime.Add(3 * time.Second)},
		{GPUUUID: "GPU-1", Value: "4", Timestamp: baseTime.Add(4 * time.Second)},
		{GPUUUID: "GPU-1", Value: "5", Timestamp: baseTime.Add(5 * time.Second)},
	}

	for _, point := range points {
		repo.Store(point)
	}

	// Query with both start_time and end_time
	results, err := repo.GetByGPU("GPU-1", storage.TimeFilter{
		StartTime: &startTime,
		EndTime:   &endTime,
	})
	require.NoError(t, err)

	// Should only return points in range [startTime, endTime]
	assert.Len(t, results, 3)
	assert.Equal(t, "2", results[0].Value)
	assert.Equal(t, "3", results[1].Value)
	assert.Equal(t, "4", results[2].Value)
}

func TestTelemetryRepository_MultipleGPUs(t *testing.T) {
	repo := NewTelemetryRepository()

	// Store telemetry for multiple GPUs
	repo.Store(&domain.TelemetryPoint{GPUUUID: "GPU-1", Value: "10", Timestamp: time.Now()})
	repo.Store(&domain.TelemetryPoint{GPUUUID: "GPU-1", Value: "20", Timestamp: time.Now()})
	repo.Store(&domain.TelemetryPoint{GPUUUID: "GPU-2", Value: "30", Timestamp: time.Now()})
	repo.Store(&domain.TelemetryPoint{GPUUUID: "GPU-2", Value: "40", Timestamp: time.Now()})

	// Verify GPU-1
	results1, err := repo.GetByGPU("GPU-1", storage.TimeFilter{})
	require.NoError(t, err)
	assert.Len(t, results1, 2)

	// Verify GPU-2
	results2, err := repo.GetByGPU("GPU-2", storage.TimeFilter{})
	require.NoError(t, err)
	assert.Len(t, results2, 2)

	// Total count
	assert.Equal(t, int64(4), repo.Count())
}

func TestTelemetryRepository_Count(t *testing.T) {
	repo := NewTelemetryRepository()

	assert.Equal(t, int64(0), repo.Count())

	repo.Store(&domain.TelemetryPoint{GPUUUID: "GPU-1", Timestamp: time.Now()})
	assert.Equal(t, int64(1), repo.Count())

	repo.Store(&domain.TelemetryPoint{GPUUUID: "GPU-1", Timestamp: time.Now()})
	assert.Equal(t, int64(2), repo.Count())

	repo.Store(&domain.TelemetryPoint{GPUUUID: "GPU-2", Timestamp: time.Now()})
	assert.Equal(t, int64(3), repo.Count())
}

func TestTelemetryRepository_Clear(t *testing.T) {
	repo := NewTelemetryRepository()

	repo.Store(&domain.TelemetryPoint{GPUUUID: "GPU-1", Timestamp: time.Now()})
	repo.Store(&domain.TelemetryPoint{GPUUUID: "GPU-2", Timestamp: time.Now()})
	assert.Equal(t, int64(2), repo.Count())

	repo.Clear()
	assert.Equal(t, int64(0), repo.Count())

	results, _ := repo.GetByGPU("GPU-1", storage.TimeFilter{})
	assert.Empty(t, results)
}

func TestTelemetryRepository_GetGPUUUIDs(t *testing.T) {
	repo := NewTelemetryRepository()

	repo.Store(&domain.TelemetryPoint{GPUUUID: "GPU-1", Timestamp: time.Now()})
	repo.Store(&domain.TelemetryPoint{GPUUUID: "GPU-2", Timestamp: time.Now()})
	repo.Store(&domain.TelemetryPoint{GPUUUID: "GPU-1", Timestamp: time.Now()})

	uuids := repo.GetGPUUUIDs()
	assert.Len(t, uuids, 2)

	uuidMap := make(map[string]bool)
	for _, uuid := range uuids {
		uuidMap[uuid] = true
	}
	assert.True(t, uuidMap["GPU-1"])
	assert.True(t, uuidMap["GPU-2"])
}

// TestTelemetryRepository_ConcurrentStore tests concurrent writes
func TestTelemetryRepository_ConcurrentStore(t *testing.T) {
	repo := NewTelemetryRepository()

	numGoroutines := 100
	pointsPerGoroutine := 100

	var wg sync.WaitGroup
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			for j := 0; j < pointsPerGoroutine; j++ {
				point := &domain.TelemetryPoint{
					GPUUUID:    fmt.Sprintf("GPU-%d", goroutineID%10),
					MetricName: "metric",
					Value:      fmt.Sprintf("%d", j),
					Timestamp:  time.Now(),
				}
				repo.Store(point)
			}
		}(i)
	}

	wg.Wait()

	// Verify count
	expectedCount := int64(numGoroutines * pointsPerGoroutine)
	assert.Equal(t, expectedCount, repo.Count())
}

// TestTelemetryRepository_ConcurrentReadWrite tests concurrent reads and writes
func TestTelemetryRepository_ConcurrentReadWrite(t *testing.T) {
	repo := NewTelemetryRepository()

	// Pre-populate
	for i := 0; i < 10; i++ {
		for j := 0; j < 10; j++ {
			repo.Store(&domain.TelemetryPoint{
				GPUUUID:   fmt.Sprintf("GPU-%d", i),
				Value:     fmt.Sprintf("%d", j),
				Timestamp: time.Now(),
			})
		}
	}

	var wg sync.WaitGroup

	// Writers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				repo.Store(&domain.TelemetryPoint{
					GPUUUID:   fmt.Sprintf("GPU-%d", id%10),
					Value:     "new",
					Timestamp: time.Now(),
				})
			}
		}(i)
	}

	// Readers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				repo.GetByGPU(fmt.Sprintf("GPU-%d", id%10), storage.TimeFilter{})
				repo.Count()
				repo.GetGPUUUIDs()
			}
		}(i)
	}

	wg.Wait()

	// Verify data integrity (initial 100 + 1000 new writes)
	assert.Equal(t, int64(1100), repo.Count())
}

// TestTelemetryRepository_StressTest tests repository under load
func TestTelemetryRepository_StressTest(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	repo := NewTelemetryRepository()

	numGPUs := 50
	pointsPerGPU := 1000

	var wg sync.WaitGroup

	// Concurrent writes
	for i := 0; i < numGPUs; i++ {
		wg.Add(1)
		go func(gpuID int) {
			defer wg.Done()

			for j := 0; j < pointsPerGPU; j++ {
				point := &domain.TelemetryPoint{
					GPUUUID:    fmt.Sprintf("GPU-%d", gpuID),
					MetricName: fmt.Sprintf("metric-%d", j%10),
					Value:      fmt.Sprintf("%d", j),
					Timestamp:  time.Now().Add(time.Duration(j) * time.Millisecond),
				}
				if err := repo.Store(point); err != nil {
					t.Errorf("Failed to store: %v", err)
				}
			}
		}(i)
	}

	wg.Wait()

	// Verify total count
	expectedTotal := int64(numGPUs * pointsPerGPU)
	assert.Equal(t, expectedTotal, repo.Count())

	// Verify each GPU has correct count
	for i := 0; i < numGPUs; i++ {
		gpuUUID := fmt.Sprintf("GPU-%d", i)
		points, err := repo.GetByGPU(gpuUUID, storage.TimeFilter{})
		require.NoError(t, err)
		assert.Len(t, points, pointsPerGPU, "GPU %s should have %d points", gpuUUID, pointsPerGPU)

		// Verify ordering
		for j := 1; j < len(points); j++ {
			assert.True(t, points[j-1].Timestamp.Before(points[j].Timestamp) || 
				points[j-1].Timestamp.Equal(points[j].Timestamp),
				"Points should be ordered by timestamp")
		}
	}
}
