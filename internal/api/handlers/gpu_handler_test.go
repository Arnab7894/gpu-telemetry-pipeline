package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/arnabghosh/gpu-metrics-streamer/internal/api/dto"
	"github.com/arnabghosh/gpu-metrics-streamer/internal/domain"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestGPUHandler_ListGPUs_Success(t *testing.T) {
	mockGPUs := []*domain.GPU{
		{
			UUID:      "gpu-001",
			DeviceID:  "nvidia0",
			GPUIndex:  "0",
			ModelName: "Tesla V100",
			Hostname:  "host-001",
		},
		{
			UUID:      "gpu-002",
			DeviceID:  "nvidia1",
			GPUIndex:  "1",
			ModelName: "Tesla P100",
			Hostname:  "host-002",
		},
	}

	mockRepo := &MockGPURepository{
		ListFunc: func() ([]*domain.GPU, error) {
			return mockGPUs, nil
		},
	}

	handler := NewGPUHandler(mockRepo)
	router, w := setupGinTest()
	router.GET("/gpus", handler.ListGPUs)

	req := httptest.NewRequest(http.MethodGet, "/gpus", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response dto.GPUListResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, 2, response.Total)
	assert.Len(t, response.GPUs, 2)
	assert.Equal(t, "gpu-001", response.GPUs[0].UUID)
	assert.Equal(t, "Tesla V100", response.GPUs[0].ModelName)
}

func TestGPUHandler_ListGPUs_EmptyList(t *testing.T) {
	mockRepo := &MockGPURepository{
		ListFunc: func() ([]*domain.GPU, error) {
			return []*domain.GPU{}, nil
		},
	}

	handler := NewGPUHandler(mockRepo)
	router, w := setupGinTest()
	router.GET("/gpus", handler.ListGPUs)

	req := httptest.NewRequest(http.MethodGet, "/gpus", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response dto.GPUListResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, 0, response.Total)
	assert.Len(t, response.GPUs, 0)
}

func TestGPUHandler_ListGPUs_RepositoryError(t *testing.T) {
	mockRepo := &MockGPURepository{
		ListFunc: func() ([]*domain.GPU, error) {
			return nil, errors.New("database connection failed")
		},
	}

	handler := NewGPUHandler(mockRepo)
	router, w := setupGinTest()
	router.GET("/gpus", handler.ListGPUs)

	req := httptest.NewRequest(http.MethodGet, "/gpus", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var response dto.ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "Failed to retrieve GPUs", response.Error)
}

func TestGPUHandler_GetGPU_Success(t *testing.T) {
	mockGPU := &domain.GPU{
		UUID:      "gpu-001",
		DeviceID:  "nvidia0",
		GPUIndex:  "0",
		ModelName: "Tesla V100",
		Hostname:  "host-001",
	}

	mockRepo := &MockGPURepository{
		GetByUUIDFunc: func(uuid string) (*domain.GPU, error) {
			if uuid == "gpu-001" {
				return mockGPU, nil
			}
			return nil, domain.ErrGPUNotFound
		},
	}

	handler := NewGPUHandler(mockRepo)
	router, w := setupGinTest()
	router.GET("/gpus/:uuid", handler.GetGPU)

	req := httptest.NewRequest(http.MethodGet, "/gpus/gpu-001", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response dto.GPUResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "gpu-001", response.UUID)
	assert.Equal(t, "Tesla V100", response.ModelName)
	assert.Equal(t, "nvidia0", response.DeviceID)
}

func TestGPUHandler_GetGPU_NotFound(t *testing.T) {
	mockRepo := &MockGPURepository{
		GetByUUIDFunc: func(uuid string) (*domain.GPU, error) {
			return nil, domain.ErrGPUNotFound
		},
	}

	handler := NewGPUHandler(mockRepo)
	router, w := setupGinTest()
	router.GET("/gpus/:uuid", handler.GetGPU)

	req := httptest.NewRequest(http.MethodGet, "/gpus/nonexistent", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var response dto.ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "GPU not found", response.Error)
}

func TestGPUHandler_GetGPU_EmptyUUID(t *testing.T) {
	mockRepo := &MockGPURepository{}
	handler := NewGPUHandler(mockRepo)
	router, w := setupGinTest()
	router.GET("/gpus/:uuid", handler.GetGPU)

	req := httptest.NewRequest(http.MethodGet, "/gpus/", nil)
	router.ServeHTTP(w, req)

	// Gin will return 404 for empty UUID (no route match)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGPUHandler_GetGPU_RepositoryError(t *testing.T) {
	mockRepo := &MockGPURepository{
		GetByUUIDFunc: func(uuid string) (*domain.GPU, error) {
			return nil, errors.New("database connection failed")
		},
	}

	handler := NewGPUHandler(mockRepo)
	router, w := setupGinTest()
	router.GET("/gpus/:uuid", handler.GetGPU)

	req := httptest.NewRequest(http.MethodGet, "/gpus/gpu-001", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var response dto.ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "Failed to retrieve GPU", response.Error)
}

// Benchmark tests
func BenchmarkGPUHandler_ListGPUs(b *testing.B) {
	mockGPUs := make([]*domain.GPU, 100)
	for i := 0; i < 100; i++ {
		mockGPUs[i] = &domain.GPU{
			UUID:      "gpu-" + string(rune(i)),
			DeviceID:  "nvidia0",
			GPUIndex:  "0",
			ModelName: "Tesla V100",
			Hostname:  "host-001",
		}
	}

	mockRepo := &MockGPURepository{
		ListFunc: func() ([]*domain.GPU, error) {
			return mockGPUs, nil
		},
	}

	handler := NewGPUHandler(mockRepo)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/gpus", handler.ListGPUs)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/gpus", nil)
		router.ServeHTTP(w, req)
	}
}

func BenchmarkGPUHandler_GetGPU(b *testing.B) {
	mockGPU := &domain.GPU{
		UUID:      "gpu-001",
		DeviceID:  "nvidia0",
		GPUIndex:  "0",
		ModelName: "Tesla V100",
		Hostname:  "host-001",
	}

	mockRepo := &MockGPURepository{
		GetByUUIDFunc: func(uuid string) (*domain.GPU, error) {
			return mockGPU, nil
		},
	}

	handler := NewGPUHandler(mockRepo)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/gpus/:uuid", handler.GetGPU)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/gpus/gpu-001", nil)
		router.ServeHTTP(w, req)
	}
}
