package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/arnabghosh/gpu-metrics-streamer/internal/domain"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// MongoTelemetryRepository implements TelemetryRepository using MongoDB
type MongoTelemetryRepository struct {
	client     *mongo.Client
	database   string
	collection string
}

// NewMongoTelemetryRepository creates a new MongoDB-backed telemetry repository
func NewMongoTelemetryRepository(mongoURI, database, collection string) (*MongoTelemetryRepository, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	clientOptions := options.Client().ApplyURI(mongoURI)
	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	// Ping to verify connection
	if err := client.Ping(ctx, nil); err != nil {
		return nil, fmt.Errorf("failed to ping MongoDB: %w", err)
	}

	return &MongoTelemetryRepository{
		client:     client,
		database:   database,
		collection: collection,
	}, nil
}

// StoreTelemetry stores a telemetry point in MongoDB
func (r *MongoTelemetryRepository) StoreTelemetry(ctx context.Context, point domain.TelemetryPoint) error {
	coll := r.client.Database(r.database).Collection(r.collection)

	_, err := coll.InsertOne(ctx, point)
	if err != nil {
		return fmt.Errorf("failed to insert telemetry: %w", err)
	}

	return nil
}

// GetTelemetryByGPU retrieves telemetry for a specific GPU, ordered by timestamp
func (r *MongoTelemetryRepository) GetTelemetryByGPU(ctx context.Context, gpuID string, startTime, endTime *time.Time) ([]domain.TelemetryPoint, error) {
	coll := r.client.Database(r.database).Collection(r.collection)

	// Build filter
	filter := bson.M{"gpu_uuid": gpuID}

	if startTime != nil || endTime != nil {
		timeFilter := bson.M{}
		if startTime != nil {
			timeFilter["$gte"] = *startTime
		}
		if endTime != nil {
			timeFilter["$lte"] = *endTime
		}
		filter["timestamp"] = timeFilter
	}

	// Sort by timestamp ascending
	opts := options.Find().SetSort(bson.D{{Key: "timestamp", Value: 1}})

	cursor, err := coll.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to query telemetry: %w", err)
	}
	defer cursor.Close(ctx)

	var results []domain.TelemetryPoint
	if err := cursor.All(ctx, &results); err != nil {
		return nil, fmt.Errorf("failed to decode telemetry: %w", err)
	}

	return results, nil
}

// ListGPUs returns all unique GPU IDs that have telemetry data
func (r *MongoTelemetryRepository) ListGPUs(ctx context.Context) ([]string, error) {
	coll := r.client.Database(r.database).Collection(r.collection)

	gpuIDs, err := coll.Distinct(ctx, "gpu_id", bson.M{})
	if err != nil {
		return nil, fmt.Errorf("failed to get distinct GPU IDs: %w", err)
	}

	result := make([]string, len(gpuIDs))
	for i, id := range gpuIDs {
		result[i] = id.(string)
	}

	return result, nil
}

// Store stores a telemetry point (alias for StoreTelemetry)
func (r *MongoTelemetryRepository) Store(telemetry *domain.TelemetryPoint) error {
	return r.StoreTelemetry(context.Background(), *telemetry)
}

// GetByGPU retrieves telemetry for a specific GPU (alias for GetTelemetryByGPU)
func (r *MongoTelemetryRepository) GetByGPU(gpuUUID string, filter TimeFilter) ([]*domain.TelemetryPoint, error) {
	points, err := r.GetTelemetryByGPU(context.Background(), gpuUUID, filter.StartTime, filter.EndTime)
	if err != nil {
		return nil, err
	}

	result := make([]*domain.TelemetryPoint, len(points))
	for i := range points {
		result[i] = &points[i]
	}
	return result, nil
}

// Count returns the total number of telemetry points
func (r *MongoTelemetryRepository) Count() int64 {
	coll := r.client.Database(r.database).Collection(r.collection)
	count, err := coll.CountDocuments(context.Background(), bson.M{})
	if err != nil {
		return 0
	}
	return count
}

// Close closes the MongoDB connection
func (r *MongoTelemetryRepository) Close(ctx context.Context) error {
	return r.client.Disconnect(ctx)
}

// MongoGPURepository implements GPURepository using MongoDB
type MongoGPURepository struct {
	client     *mongo.Client
	database   string
	collection string
}

// NewMongoGPURepository creates a new MongoDB-backed GPU repository
func NewMongoGPURepository(mongoURI, database, collection string) (*MongoGPURepository, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	clientOptions := options.Client().ApplyURI(mongoURI)
	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	// Ping to verify connection
	if err := client.Ping(ctx, nil); err != nil {
		return nil, fmt.Errorf("failed to ping MongoDB: %w", err)
	}

	return &MongoGPURepository{
		client:     client,
		database:   database,
		collection: collection,
	}, nil
}

// GetGPU retrieves a GPU by ID
func (r *MongoGPURepository) GetGPU(ctx context.Context, id string) (*domain.GPU, error) {
	coll := r.client.Database(r.database).Collection(r.collection)

	var gpu domain.GPU
	err := coll.FindOne(ctx, bson.M{"uuid": id}).Decode(&gpu)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get GPU: %w", err)
	}

	return &gpu, nil
}

// StoreGPU stores or updates a GPU
func (r *MongoGPURepository) StoreGPU(ctx context.Context, gpu domain.GPU) error {
	coll := r.client.Database(r.database).Collection(r.collection)

	opts := options.Update().SetUpsert(true)
	_, err := coll.UpdateOne(
		ctx,
		bson.M{"uuid": gpu.UUID},
		bson.M{"$set": gpu},
		opts,
	)

	if err != nil {
		return fmt.Errorf("failed to store GPU: %w", err)
	}

	return nil
}

// Store stores or updates a GPU (alias for StoreGPU)
func (r *MongoGPURepository) Store(gpu *domain.GPU) error {
	return r.StoreGPU(context.Background(), *gpu)
}

// ListGPUs returns all GPUs
func (r *MongoGPURepository) ListGPUs(ctx context.Context) ([]domain.GPU, error) {
	coll := r.client.Database(r.database).Collection(r.collection)

	cursor, err := coll.Find(ctx, bson.M{})
	if err != nil {
		return nil, fmt.Errorf("failed to list GPUs: %w", err)
	}
	defer cursor.Close(ctx)

	var results []domain.GPU
	if err := cursor.All(ctx, &results); err != nil {
		return nil, fmt.Errorf("failed to decode GPUs: %w", err)
	}

	return results, nil
}

// List returns all GPUs (alias for ListGPUs)
func (r *MongoGPURepository) List() ([]*domain.GPU, error) {
	gpus, err := r.ListGPUs(context.Background())
	if err != nil {
		return nil, err
	}

	result := make([]*domain.GPU, len(gpus))
	for i := range gpus {
		result[i] = &gpus[i]
	}
	return result, nil
}

// GetByUUID retrieves a GPU by UUID (alias for GetGPU)
func (r *MongoGPURepository) GetByUUID(uuid string) (*domain.GPU, error) {
	return r.GetGPU(context.Background(), uuid)
}

// Count returns the total number of GPUs
func (r *MongoGPURepository) Count() int64 {
	coll := r.client.Database(r.database).Collection(r.collection)
	count, err := coll.CountDocuments(context.Background(), bson.M{})
	if err != nil {
		return 0
	}
	return count
}

// Close closes the MongoDB connection
func (r *MongoGPURepository) Close(ctx context.Context) error {
	return r.client.Disconnect(ctx)
}
