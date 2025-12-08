package inmemory

import (
	"sync"

	"github.com/arnabghosh/gpu-metrics-streamer/internal/domain"
)

// GPURepository is an in-memory implementation of GPU storage
// This is a simple implementation using Go maps with mutex protection.
//
// Future MongoDB Implementation Example:
//
//type MongoGPURepository struct {
//    collection *mongo.Collection
//}
//
//func (r *MongoGPURepository) Store(gpu *domain.GPU) error {
//    _, err := r.collection.InsertOne(context.TODO(), gpu)
//    return err
//}
//
//func (r *MongoGPURepository) GetByUUID(uuid string) (*domain.GPU, error) {
//    var gpu domain.GPU
//    err := r.collection.FindOne(context.TODO(), bson.M{"uuid": uuid}).Decode(&gpu)
//    return &gpu, err
//}
//
// To use MongoDB, simply inject MongoGPURepository instead of InMemoryGPURepository
// via dependency injection in main.go or a factory pattern.
type GPURepository struct {
	mu   sync.RWMutex
	data map[string]*domain.GPU // key: GPU UUID
}

// NewGPURepository creates a new in-memory GPU repository
func NewGPURepository() *GPURepository {
	return &GPURepository{
		data: make(map[string]*domain.GPU),
	}
}

// Store persists or updates a GPU record
// Thread-safe for concurrent writes
func (r *GPURepository) Store(gpu *domain.GPU) error {
	if gpu == nil {
		return domain.ErrInvalidInput
	}
	if gpu.UUID == "" {
		return domain.ErrInvalidInput
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.data[gpu.UUID] = gpu
	return nil
}

// GetByUUID retrieves a GPU by its UUID
// Thread-safe for concurrent reads
func (r *GPURepository) GetByUUID(uuid string) (*domain.GPU, error) {
	if uuid == "" {
		return nil, domain.ErrInvalidInput
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	gpu, exists := r.data[uuid]
	if !exists {
		return nil, domain.ErrGPUNotFound
	}
	return gpu, nil
}

// List returns all GPUs that have telemetry data
// Thread-safe for concurrent reads
func (r *GPURepository) List() ([]*domain.GPU, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	gpus := make([]*domain.GPU, 0, len(r.data))
	for _, gpu := range r.data {
		gpus = append(gpus, gpu)
	}
	return gpus, nil
}

// Count returns the total number of GPUs
// Thread-safe for concurrent reads
func (r *GPURepository) Count() int64 {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return int64(len(r.data))
}

// Clear removes all GPUs from the repository
// Useful for testing
func (r *GPURepository) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.data = make(map[string]*domain.GPU)
}
