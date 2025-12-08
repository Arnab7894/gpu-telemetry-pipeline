package inmemory

import (
	"fmt"
	"sync"
	"testing"

	"github.com/arnabghosh/gpu-metrics-streamer/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGPURepository_Store(t *testing.T) {
	repo := NewGPURepository()

	gpu := &domain.GPU{
		UUID:      "GPU-123",
		DeviceID:  "nvidia0",
		GPUIndex:  "0",
		ModelName: "NVIDIA A100",
		Hostname:  "node-1",
	}

	err := repo.Store(gpu)
	require.NoError(t, err)

	// Verify stored
	assert.Equal(t, int64(1), repo.Count())

	// Retrieve and verify
	retrieved, err := repo.GetByUUID("GPU-123")
	require.NoError(t, err)
	assert.Equal(t, gpu.UUID, retrieved.UUID)
	assert.Equal(t, gpu.ModelName, retrieved.ModelName)
}

func TestGPURepository_StoreUpdate(t *testing.T) {
	repo := NewGPURepository()

	gpu := &domain.GPU{
		UUID:      "GPU-123",
		DeviceID:  "nvidia0",
		ModelName: "NVIDIA A100",
	}

	// Store initial
	err := repo.Store(gpu)
	require.NoError(t, err)

	// Update
	gpu.ModelName = "NVIDIA A100-SXM4-80GB"
	err = repo.Store(gpu)
	require.NoError(t, err)

	// Should still be 1 GPU (update, not insert)
	assert.Equal(t, int64(1), repo.Count())

	// Verify update
	retrieved, err := repo.GetByUUID("GPU-123")
	require.NoError(t, err)
	assert.Equal(t, "NVIDIA A100-SXM4-80GB", retrieved.ModelName)
}

func TestGPURepository_StoreNil(t *testing.T) {
	repo := NewGPURepository()

	err := repo.Store(nil)
	assert.ErrorIs(t, err, domain.ErrInvalidInput)
}

func TestGPURepository_StoreEmptyUUID(t *testing.T) {
	repo := NewGPURepository()

	gpu := &domain.GPU{
		DeviceID: "nvidia0",
	}

	err := repo.Store(gpu)
	assert.ErrorIs(t, err, domain.ErrInvalidInput)
}

func TestGPURepository_GetByUUID_NotFound(t *testing.T) {
	repo := NewGPURepository()

	gpu, err := repo.GetByUUID("non-existent")
	assert.ErrorIs(t, err, domain.ErrGPUNotFound)
	assert.Nil(t, gpu)
}

func TestGPURepository_GetByUUID_EmptyUUID(t *testing.T) {
	repo := NewGPURepository()

	gpu, err := repo.GetByUUID("")
	assert.ErrorIs(t, err, domain.ErrInvalidInput)
	assert.Nil(t, gpu)
}

func TestGPURepository_List(t *testing.T) {
	repo := NewGPURepository()

	// Store multiple GPUs
	gpus := []*domain.GPU{
		{UUID: "GPU-1", ModelName: "A100"},
		{UUID: "GPU-2", ModelName: "V100"},
		{UUID: "GPU-3", ModelName: "H100"},
	}

	for _, gpu := range gpus {
		err := repo.Store(gpu)
		require.NoError(t, err)
	}

	// List all
	list, err := repo.List()
	require.NoError(t, err)
	assert.Len(t, list, 3)

	// Verify all present
	uuids := make(map[string]bool)
	for _, gpu := range list {
		uuids[gpu.UUID] = true
	}
	assert.True(t, uuids["GPU-1"])
	assert.True(t, uuids["GPU-2"])
	assert.True(t, uuids["GPU-3"])
}

func TestGPURepository_ListEmpty(t *testing.T) {
	repo := NewGPURepository()

	list, err := repo.List()
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestGPURepository_Count(t *testing.T) {
	repo := NewGPURepository()

	assert.Equal(t, int64(0), repo.Count())

	repo.Store(&domain.GPU{UUID: "GPU-1"})
	assert.Equal(t, int64(1), repo.Count())

	repo.Store(&domain.GPU{UUID: "GPU-2"})
	assert.Equal(t, int64(2), repo.Count())

	// Update should not increase count
	repo.Store(&domain.GPU{UUID: "GPU-1", ModelName: "Updated"})
	assert.Equal(t, int64(2), repo.Count())
}

func TestGPURepository_Clear(t *testing.T) {
	repo := NewGPURepository()

	repo.Store(&domain.GPU{UUID: "GPU-1"})
	repo.Store(&domain.GPU{UUID: "GPU-2"})
	assert.Equal(t, int64(2), repo.Count())

	repo.Clear()
	assert.Equal(t, int64(0), repo.Count())

	list, _ := repo.List()
	assert.Empty(t, list)
}

// TestGPURepository_ConcurrentStore tests concurrent writes
func TestGPURepository_ConcurrentStore(t *testing.T) {
	repo := NewGPURepository()

	numGoroutines := 100
	gpusPerGoroutine := 10

	var wg sync.WaitGroup
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			for j := 0; j < gpusPerGoroutine; j++ {
				gpu := &domain.GPU{
					UUID:      fmt.Sprintf("GPU-%d-%d", goroutineID, j),
					ModelName: "NVIDIA A100",
				}
				repo.Store(gpu)
			}
		}(i)
	}

	wg.Wait()

	// Verify count
	expectedCount := int64(numGoroutines * gpusPerGoroutine)
	assert.Equal(t, expectedCount, repo.Count())
}

// TestGPURepository_ConcurrentReadWrite tests concurrent reads and writes
func TestGPURepository_ConcurrentReadWrite(t *testing.T) {
	repo := NewGPURepository()

	// Pre-populate
	for i := 0; i < 10; i++ {
		gpu := &domain.GPU{
			UUID:      string(rune('A' + i)),
			ModelName: "GPU-" + string(rune('0'+i)),
		}
		repo.Store(gpu)
	}

	var wg sync.WaitGroup

	// Writers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				gpu := &domain.GPU{
					UUID:      string(rune('A' + (id % 10))),
					ModelName: "Updated",
				}
				repo.Store(gpu)
			}
		}(i)
	}

	// Readers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				repo.GetByUUID(string(rune('A' + (id % 10))))
				repo.List()
				repo.Count()
			}
		}(i)
	}

	wg.Wait()

	// Verify data integrity
	assert.Equal(t, int64(10), repo.Count())
}
