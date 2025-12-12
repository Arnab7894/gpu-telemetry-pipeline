package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/arnabghosh/gpu-metrics-streamer/internal/api/dto"
	"github.com/arnabghosh/gpu-metrics-streamer/internal/domain"
	"github.com/arnabghosh/gpu-metrics-streamer/internal/storage"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// Valid UUID format for testing
const testValidUUID = "GPU-12345678-1234-1234-1234-123456789abc"

func TestTelemetryHandler_GetGPUTelemetry_Success(t *testing.T) {
	// Use valid UUID format that matches the regex requirement
	validUUID := "GPU-12345678-1234-1234-1234-123456789abc"
	mockGPU := &domain.GPU{
		UUID:      validUUID,
		DeviceID:  "nvidia0",
		ModelName: "Tesla V100",
		Hostname:  "host-001",
	}

	baseTime := time.Date(2025, 1, 18, 12, 0, 0, 0, time.UTC)
	mockTelemetry := []*domain.TelemetryPoint{
		{
			GPUUUID:    validUUID,
			MetricName: "DCGM_FI_DEV_GPU_UTIL",
			Value:      "85.5",
			Timestamp:  baseTime,
			LabelsRaw:  "Hostname=\"host-001\"",
		},
	}

	mockGPURepo := &MockGPURepository{
		GetByUUIDFunc: func(uuid string) (*domain.GPU, error) {
			if uuid == validUUID {
				return mockGPU, nil
			}
			return nil, domain.ErrGPUNotFound
		},
	}

	mockTelemetryRepo := &MockTelemetryRepository{
		GetByGPUFunc: func(gpuUUID string, filter storage.TimeFilter) ([]*domain.TelemetryPoint, error) {
			return mockTelemetry, nil
		},
	}

	handler := NewTelemetryHandler(mockTelemetryRepo, mockGPURepo)
	router, w := setupGinTest()
	router.GET("/gpus/:uuid/telemetry", handler.GetGPUTelemetry)

	// Use valid UUID format in URL
	req := httptest.NewRequest(http.MethodGet, "/gpus/"+validUUID+"/telemetry?start_time=2024-01-01T00:00:00Z&end_time=2024-12-31T23:59:59Z", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response dto.TelemetryListResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, validUUID, response.GPUUUID)
	assert.Equal(t, 1, response.Total)
	assert.Len(t, response.Metrics, 1)
	assert.Equal(t, "85.5", response.Metrics[0].Value)
}

func TestTelemetryHandler_GetGPUTelemetry_WithTimeFilter(t *testing.T) {
	mockGPU := &domain.GPU{
		UUID:      testValidUUID,
		DeviceID:  "nvidia0",
		ModelName: "Tesla V100",
		Hostname:  "host-001",
	}

	mockTelemetry := []*domain.TelemetryPoint{}

	mockGPURepo := &MockGPURepository{
		GetByUUIDFunc: func(uuid string) (*domain.GPU, error) {
			return mockGPU, nil
		},
	}

	mockTelemetryRepo := &MockTelemetryRepository{
		GetByGPUFunc: func(gpuUUID string, filter storage.TimeFilter) ([]*domain.TelemetryPoint, error) {
			// Verify filter was passed correctly
			assert.NotNil(t, filter.StartTime)
			assert.NotNil(t, filter.EndTime)
			return mockTelemetry, nil
		},
	}

	handler := NewTelemetryHandler(mockTelemetryRepo, mockGPURepo)
	router, w := setupGinTest()
	router.GET("/gpus/:uuid/telemetry", handler.GetGPUTelemetry)

	req := httptest.NewRequest(http.MethodGet, "/gpus/"+testValidUUID+"/telemetry?start_time=2025-01-18T12:00:00Z&end_time=2025-01-18T13:00:00Z", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestTelemetryHandler_GetGPUTelemetry_GPUNotFound(t *testing.T) {
	mockGPURepo := &MockGPURepository{
		GetByUUIDFunc: func(uuid string) (*domain.GPU, error) {
			return nil, domain.ErrGPUNotFound
		},
	}

	mockTelemetryRepo := &MockTelemetryRepository{}

	handler := NewTelemetryHandler(mockTelemetryRepo, mockGPURepo)
	router, w := setupGinTest()
	router.GET("/gpus/:uuid/telemetry", handler.GetGPUTelemetry)

	// Use a different valid UUID format that will not be found
	notFoundUUID := "GPU-00000000-0000-0000-0000-000000000000"
	req := httptest.NewRequest(http.MethodGet, "/gpus/"+notFoundUUID+"/telemetry", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var response dto.ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "GPU not found", response.Error)
}

func TestTelemetryHandler_GetGPUTelemetry_InvalidStartTime(t *testing.T) {
	mockGPU := &domain.GPU{
		UUID:      testValidUUID,
		DeviceID:  "nvidia0",
		ModelName: "Tesla V100",
		Hostname:  "host-001",
	}

	mockGPURepo := &MockGPURepository{
		GetByUUIDFunc: func(uuid string) (*domain.GPU, error) {
			return mockGPU, nil
		},
	}
	mockTelemetryRepo := &MockTelemetryRepository{}

	handler := NewTelemetryHandler(mockTelemetryRepo, mockGPURepo)
	router, w := setupGinTest()
	router.GET("/gpus/:uuid/telemetry", handler.GetGPUTelemetry)

	req := httptest.NewRequest(http.MethodGet, "/gpus/"+testValidUUID+"/telemetry?start_time=invalid", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response dto.ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Contains(t, response.Error, "Invalid start_time format")
}

func TestTelemetryHandler_GetGPUTelemetry_InvalidEndTime(t *testing.T) {
	mockGPU := &domain.GPU{
		UUID:      testValidUUID,
		DeviceID:  "nvidia0",
		ModelName: "Tesla V100",
		Hostname:  "host-001",
	}

	mockGPURepo := &MockGPURepository{
		GetByUUIDFunc: func(uuid string) (*domain.GPU, error) {
			return mockGPU, nil
		},
	}
	mockTelemetryRepo := &MockTelemetryRepository{}

	handler := NewTelemetryHandler(mockTelemetryRepo, mockGPURepo)
	router, w := setupGinTest()
	router.GET("/gpus/:uuid/telemetry", handler.GetGPUTelemetry)

	req := httptest.NewRequest(http.MethodGet, "/gpus/"+testValidUUID+"/telemetry?end_time=invalid", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response dto.ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Contains(t, response.Error, "Invalid end_time format")
}

func TestTelemetryHandler_GetGPUTelemetry_StartTimeAfterEndTime(t *testing.T) {
	mockGPU := &domain.GPU{
		UUID:      testValidUUID,
		DeviceID:  "nvidia0",
		ModelName: "Tesla V100",
		Hostname:  "host-001",
	}

	mockGPURepo := &MockGPURepository{
		GetByUUIDFunc: func(uuid string) (*domain.GPU, error) {
			return mockGPU, nil
		},
	}
	mockTelemetryRepo := &MockTelemetryRepository{}

	handler := NewTelemetryHandler(mockTelemetryRepo, mockGPURepo)
	router, w := setupGinTest()
	router.GET("/gpus/:uuid/telemetry", handler.GetGPUTelemetry)

	req := httptest.NewRequest(http.MethodGet, "/gpus/"+testValidUUID+"/telemetry?start_time=2025-01-18T13:00:00Z&end_time=2025-01-18T12:00:00Z", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response dto.ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "Invalid time range", response.Error)
}

func TestTelemetryHandler_GetGPUTelemetry_EmptyResults(t *testing.T) {
	mockGPU := &domain.GPU{
		UUID:      testValidUUID,
		DeviceID:  "nvidia0",
		ModelName: "Tesla V100",
		Hostname:  "host-001",
	}

	mockGPURepo := &MockGPURepository{
		GetByUUIDFunc: func(uuid string) (*domain.GPU, error) {
			return mockGPU, nil
		},
	}

	mockTelemetryRepo := &MockTelemetryRepository{
		GetByGPUFunc: func(gpuUUID string, filter storage.TimeFilter) ([]*domain.TelemetryPoint, error) {
			return []*domain.TelemetryPoint{}, nil
		},
	}

	handler := NewTelemetryHandler(mockTelemetryRepo, mockGPURepo)
	router, w := setupGinTest()
	router.GET("/gpus/:uuid/telemetry", handler.GetGPUTelemetry)

	req := httptest.NewRequest(http.MethodGet, "/gpus/"+testValidUUID+"/telemetry", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response dto.TelemetryListResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, 0, response.Total)
	assert.Len(t, response.Metrics, 0)
}

func TestTelemetryHandler_GetGPUTelemetry_RepositoryError(t *testing.T) {
	mockGPU := &domain.GPU{
		UUID:      testValidUUID,
		DeviceID:  "nvidia0",
		ModelName: "Tesla V100",
		Hostname:  "host-001",
	}

	mockGPURepo := &MockGPURepository{
		GetByUUIDFunc: func(uuid string) (*domain.GPU, error) {
			return mockGPU, nil
		},
	}

	mockTelemetryRepo := &MockTelemetryRepository{
		GetByGPUFunc: func(gpuUUID string, filter storage.TimeFilter) ([]*domain.TelemetryPoint, error) {
			return nil, errors.New("database connection failed")
		},
	}

	handler := NewTelemetryHandler(mockTelemetryRepo, mockGPURepo)
	router, w := setupGinTest()
	router.GET("/gpus/:uuid/telemetry", handler.GetGPUTelemetry)

	req := httptest.NewRequest(http.MethodGet, "/gpus/"+testValidUUID+"/telemetry", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var response dto.ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "Failed to retrieve telemetry data", response.Error)
}

func BenchmarkTelemetryHandler_GetGPUTelemetry(b *testing.B) {
	mockGPU := &domain.GPU{
		UUID:      testValidUUID,
		DeviceID:  "nvidia0",
		ModelName: "Tesla V100",
		Hostname:  "host-001",
	}

	baseTime := time.Now()
	mockTelemetry := make([]*domain.TelemetryPoint, 100)
	for i := 0; i < 100; i++ {
		mockTelemetry[i] = &domain.TelemetryPoint{
			GPUUUID:    testValidUUID,
			MetricName: "DCGM_FI_DEV_GPU_UTIL",
			Value:      "85.5",
			Timestamp:  baseTime.Add(time.Duration(i) * time.Second),
		}
	}

	mockGPURepo := &MockGPURepository{
		GetByUUIDFunc: func(uuid string) (*domain.GPU, error) {
			return mockGPU, nil
		},
	}

	mockTelemetryRepo := &MockTelemetryRepository{
		GetByGPUFunc: func(gpuUUID string, filter storage.TimeFilter) ([]*domain.TelemetryPoint, error) {
			return mockTelemetry, nil
		},
	}

	handler := NewTelemetryHandler(mockTelemetryRepo, mockGPURepo)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/gpus/:uuid/telemetry", handler.GetGPUTelemetry)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/gpus/"+testValidUUID+"/telemetry", nil)
		router.ServeHTTP(w, req)
	}
}
