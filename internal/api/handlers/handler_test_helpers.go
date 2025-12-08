package handlers

import (
	"net/http/httptest"
	
	"github.com/arnabghosh/gpu-metrics-streamer/internal/domain"
	"github.com/arnabghosh/gpu-metrics-streamer/internal/storage"
	"github.com/gin-gonic/gin"
)

// MockGPURepository implements storage.GPURepository for testing
type MockGPURepository struct {
	ListFunc      func() ([]*domain.GPU, error)
	GetByUUIDFunc func(uuid string) (*domain.GPU, error)
	StoreFunc     func(gpu *domain.GPU) error
	CountFunc     func() int64
}

func (m *MockGPURepository) List() ([]*domain.GPU, error) {
	if m.ListFunc != nil {
		return m.ListFunc()
	}
	return nil, nil
}

func (m *MockGPURepository) GetByUUID(uuid string) (*domain.GPU, error) {
	if m.GetByUUIDFunc != nil {
		return m.GetByUUIDFunc(uuid)
	}
	return nil, domain.ErrGPUNotFound
}

func (m *MockGPURepository) Store(gpu *domain.GPU) error {
	if m.StoreFunc != nil {
		return m.StoreFunc(gpu)
	}
	return nil
}

func (m *MockGPURepository) Count() int64 {
	if m.CountFunc != nil {
		return m.CountFunc()
	}
	return 0
}

// MockTelemetryRepository implements storage.TelemetryRepository for testing
type MockTelemetryRepository struct {
	GetByGPUFunc func(gpuUUID string, filter storage.TimeFilter) ([]*domain.TelemetryPoint, error)
	StoreFunc    func(point *domain.TelemetryPoint) error
	CountFunc    func() int64
}

func (m *MockTelemetryRepository) GetByGPU(gpuUUID string, filter storage.TimeFilter) ([]*domain.TelemetryPoint, error) {
	if m.GetByGPUFunc != nil {
		return m.GetByGPUFunc(gpuUUID, filter)
	}
	return nil, nil
}

func (m *MockTelemetryRepository) Store(point *domain.TelemetryPoint) error {
	if m.StoreFunc != nil {
		return m.StoreFunc(point)
	}
	return nil
}

func (m *MockTelemetryRepository) Count() int64 {
	if m.CountFunc != nil {
		return m.CountFunc()
	}
	return 0
}

func setupGinTest() (*gin.Engine, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	w := httptest.NewRecorder()
	return router, w
}
