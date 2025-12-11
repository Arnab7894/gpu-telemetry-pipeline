package inmemory

import (
	"sort"
	"sync"

	"github.com/arnabghosh/gpu-metrics-streamer/internal/domain"
	"github.com/arnabghosh/gpu-metrics-streamer/internal/storage"
)

// TelemetryRepository is an in-memory implementation of telemetry storage
// Uses Go maps with mutex protection for thread-safety.
//
// Future MongoDB Implementation Example:
//
//	type MongoTelemetryRepository struct {
//	   collection *mongo.Collection
//	}
//
//	func (r *MongoTelemetryRepository) Store(telemetry *domain.TelemetryPoint) error {
//	   _, err := r.collection.InsertOne(context.TODO(), telemetry)
//	   return err
//	}
//
//	func (r *MongoTelemetryRepository) GetByGPU(gpuUUID string, filter storage.TimeFilter) ([]*domain.TelemetryPoint, error) {
//	   findOptions := options.Find().SetSort(bson.D{{"timestamp", 1}})
//	   query := bson.M{"gpuuuid": gpuUUID}
//
//	   if filter.StartTime != nil {
//	       query["timestamp"] = bson.M{"$gte": *filter.StartTime}
//	   }
//	   if filter.EndTime != nil {
//	       if _, ok := query["timestamp"]; ok {
//	           query["timestamp"].(bson.M)["$lte"] = *filter.EndTime
//	       } else {
//	           query["timestamp"] = bson.M{"$lte": *filter.EndTime}
//	       }
//	   }
//
//	   cursor, err := r.collection.Find(context.TODO(), query, findOptions)
//	   if err != nil {
//	       return nil, err
//	   }
//	   defer cursor.Close(context.TODO())
//
//	   var results []*domain.TelemetryPoint
//	   if err := cursor.All(context.TODO(), &results); err != nil {
//	       return nil, err
//	   }
//	   return results, nil
//	}
//
// To use MongoDB, inject MongoTelemetryRepository instead of InMemoryTelemetryRepository.
type TelemetryRepository struct {
	mu   sync.RWMutex
	data map[string][]*domain.TelemetryPoint // key: GPU UUID, value: slice of telemetry points
}

// NewTelemetryRepository creates a new in-memory telemetry repository
func NewTelemetryRepository() *TelemetryRepository {
	return &TelemetryRepository{
		data: make(map[string][]*domain.TelemetryPoint),
	}
}

// Store persists a telemetry point
// Thread-safe for concurrent writes
func (r *TelemetryRepository) Store(telemetry *domain.TelemetryPoint) error {
	if telemetry == nil {
		return domain.ErrInvalidInput
	}
	if telemetry.GPUUUID == "" {
		return domain.ErrInvalidInput
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.data[telemetry.GPUUUID] = append(r.data[telemetry.GPUUUID], telemetry)
	return nil
}

// BulkStore persists multiple telemetry points efficiently
// Thread-safe for concurrent writes
func (r *TelemetryRepository) BulkStore(telemetry []*domain.TelemetryPoint) error {
	if len(telemetry) == 0 {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for _, t := range telemetry {
		if t == nil || t.GPUUUID == "" {
			continue // Skip invalid entries
		}
		r.data[t.GPUUUID] = append(r.data[t.GPUUUID], t)
	}

	return nil
}

// GetByGPU retrieves all telemetry for a specific GPU, ordered by timestamp
// Supports optional time filtering via TimeFilter
// Thread-safe for concurrent reads
func (r *TelemetryRepository) GetByGPU(gpuUUID string, filter storage.TimeFilter) ([]*domain.TelemetryPoint, error) {
	if gpuUUID == "" {
		return nil, domain.ErrInvalidInput
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	points, exists := r.data[gpuUUID]
	if !exists {
		return []*domain.TelemetryPoint{}, nil
	}

	// Filter by time range if specified
	filtered := make([]*domain.TelemetryPoint, 0, len(points))
	for _, point := range points {
		if filter.StartTime != nil && point.Timestamp.Before(*filter.StartTime) {
			continue
		}
		if filter.EndTime != nil && point.Timestamp.After(*filter.EndTime) {
			continue
		}
		filtered = append(filtered, point)
	}

	// Sort by timestamp (ascending) - REQUIRED by API contract
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Timestamp.Before(filtered[j].Timestamp)
	})

	return filtered, nil
}

// Count returns the total number of telemetry points stored
// Thread-safe for concurrent reads
func (r *TelemetryRepository) Count() int64 {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var count int64
	for _, points := range r.data {
		count += int64(len(points))
	}
	return count
}

// Clear removes all telemetry data from the repository
// Useful for testing
func (r *TelemetryRepository) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.data = make(map[string][]*domain.TelemetryPoint)
}

// GetGPUUUIDs returns all GPU UUIDs that have telemetry data
// Useful for listing GPUs with telemetry
func (r *TelemetryRepository) GetGPUUUIDs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	uuids := make([]string, 0, len(r.data))
	for uuid := range r.data {
		uuids = append(uuids, uuid)
	}
	return uuids
}
