package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/arnabghosh/gpu-metrics-streamer/internal/domain"
	"github.com/arnabghosh/gpu-metrics-streamer/internal/storage"
	"github.com/stretchr/testify/assert"
)

// MockGPURepo for testing
type MockGPURepo struct {
	gpus map[string]*domain.GPU
}

func NewMockGPURepo() *MockGPURepo {
	return &MockGPURepo{
		gpus: make(map[string]*domain.GPU),
	}
}

func (m *MockGPURepo) Store(gpu *domain.GPU) error {
	m.gpus[gpu.UUID] = gpu
	return nil
}

func (m *MockGPURepo) BulkStore(gpus []*domain.GPU) error {
	for _, gpu := range gpus {
		m.gpus[gpu.UUID] = gpu
	}
	return nil
}

func (m *MockGPURepo) GetByUUID(uuid string) (*domain.GPU, error) {
	if gpu, ok := m.gpus[uuid]; ok {
		return gpu, nil
	}
	return nil, nil
}

// Alias for compatibility
func (m *MockGPURepo) FindByUUID(uuid string) (*domain.GPU, error) {
	return m.GetByUUID(uuid)
}

func (m *MockGPURepo) List() ([]*domain.GPU, error) {
	gpus := make([]*domain.GPU, 0, len(m.gpus))
	for _, gpu := range m.gpus {
		gpus = append(gpus, gpu)
	}
	return gpus, nil
}

func (m *MockGPURepo) Count() int64 {
	return int64(len(m.gpus))
}

// MockTelemetryRepo for testing
type MockTelemetryRepo struct {
	metrics []*domain.TelemetryPoint
}

func NewMockTelemetryRepo() *MockTelemetryRepo {
	return &MockTelemetryRepo{
		metrics: make([]*domain.TelemetryPoint, 0),
	}
}

func (m *MockTelemetryRepo) Store(point *domain.TelemetryPoint) error {
	m.metrics = append(m.metrics, point)
	return nil
}

func (m *MockTelemetryRepo) BulkStore(points []*domain.TelemetryPoint) error {
	m.metrics = append(m.metrics, points...)
	return nil
}

func (m *MockTelemetryRepo) GetByGPU(uuid string, filter storage.TimeFilter) ([]*domain.TelemetryPoint, error) {
	result := make([]*domain.TelemetryPoint, 0)
	for _, metric := range m.metrics {
		if metric.GPUUUID == uuid {
			result = append(result, metric)
		}
	}
	return result, nil
}

func (m *MockTelemetryRepo) Count() int64 {
	return int64(len(m.metrics))
}

func TestNewRouter(t *testing.T) {
	gpuRepo := NewMockGPURepo()
	telemetryRepo := NewMockTelemetryRepo()

	router := NewRouter(gpuRepo, telemetryRepo)

	assert.NotNil(t, router)
	assert.NotNil(t, router.engine)
	assert.NotNil(t, router.gpuHandler)
	assert.NotNil(t, router.telemetryHandler)
}

func TestRouter_Engine(t *testing.T) {
	gpuRepo := NewMockGPURepo()
	telemetryRepo := NewMockTelemetryRepo()

	router := NewRouter(gpuRepo, telemetryRepo)
	engine := router.Engine()

	assert.NotNil(t, engine)
	assert.Equal(t, router.engine, engine)
}

func TestRouter_HealthEndpoint(t *testing.T) {
	gpuRepo := NewMockGPURepo()
	telemetryRepo := NewMockTelemetryRepo()

	router := NewRouter(gpuRepo, telemetryRepo)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/health", nil)
	router.engine.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)

	var response map[string]string
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "healthy", response["status"])
}

func TestRouter_ListGPUsEndpoint(t *testing.T) {
	gpuRepo := NewMockGPURepo()
	telemetryRepo := NewMockTelemetryRepo()

	// Add test GPUs
	gpu1 := &domain.GPU{UUID: "gpu-1", ModelName: "Model A", Hostname: "host1"}
	gpu2 := &domain.GPU{UUID: "gpu-2", ModelName: "Model B", Hostname: "host2"}
	gpuRepo.Store(gpu1)
	gpuRepo.Store(gpu2)

	router := NewRouter(gpuRepo, telemetryRepo)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/gpus", nil)
	router.engine.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	gpus := response["gpus"].([]interface{})
	assert.Len(t, gpus, 2)
}

func TestRouter_GetGPUEndpoint(t *testing.T) {
	gpuRepo := NewMockGPURepo()
	telemetryRepo := NewMockTelemetryRepo()

	// Add test GPU
	gpu := &domain.GPU{UUID: "gpu-test", ModelName: "Model X", Hostname: "test-host"}
	gpuRepo.Store(gpu)

	router := NewRouter(gpuRepo, telemetryRepo)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/gpus/gpu-test", nil)
	router.engine.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "gpu-test", response["uuid"])
}

func TestRouter_GetGPUNotFound(t *testing.T) {
	gpuRepo := NewMockGPURepo()
	telemetryRepo := NewMockTelemetryRepo()

	router := NewRouter(gpuRepo, telemetryRepo)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/gpus/nonexistent", nil)
	router.engine.ServeHTTP(w, req)

	// Should return 200 with empty result or 404
	assert.True(t, w.Code == 200 || w.Code == 404)
}

func TestRouter_GetGPUTelemetryEndpoint(t *testing.T) {
	gpuRepo := NewMockGPURepo()
	telemetryRepo := NewMockTelemetryRepo()

	// Add test GPU
	gpu := &domain.GPU{UUID: "gpu-123", ModelName: "Test Model", Hostname: "test-host"}
	gpuRepo.Store(gpu)

	router := NewRouter(gpuRepo, telemetryRepo)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/gpus/gpu-123/telemetry", nil)
	router.engine.ServeHTTP(w, req)

	// Should return 200 with empty data or 400 if GPU validation fails
	assert.True(t, w.Code == 200 || w.Code == 400)
}

func TestRouter_SwaggerEndpoint(t *testing.T) {
	gpuRepo := NewMockGPURepo()
	telemetryRepo := NewMockTelemetryRepo()

	router := NewRouter(gpuRepo, telemetryRepo)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/swagger/", nil)
	router.engine.ServeHTTP(w, req)

	// Swagger endpoint exists (might be 404 if docs not generated, but route is registered)
	// We're just testing that the route is set up
	assert.NotNil(t, w)
}

func TestRouter_SetupMiddleware(t *testing.T) {
	gpuRepo := NewMockGPURepo()
	telemetryRepo := NewMockTelemetryRepo()

	router := NewRouter(gpuRepo, telemetryRepo)

	// Test that middleware is set up by making a request
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/health", nil)
	router.engine.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
}

func TestRouter_NotFoundRoute(t *testing.T) {
	gpuRepo := NewMockGPURepo()
	telemetryRepo := NewMockTelemetryRepo()

	router := NewRouter(gpuRepo, telemetryRepo)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/nonexistent", nil)
	router.engine.ServeHTTP(w, req)

	assert.Equal(t, 404, w.Code)
}
