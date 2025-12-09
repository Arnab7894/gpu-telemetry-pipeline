package mongodb

import (
	"context"
	"fmt"
	"time"

	"github.com/arnabghosh/gpu-metrics-streamer/internal/domain"
	"github.com/arnabghosh/gpu-metrics-streamer/internal/storage"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// TelemetryRepository implements storage.TelemetryRepository using MongoDB
type TelemetryRepository struct {
	client     *mongo.Client
	database   string
	collection string
}

// NewTelemetryRepository creates a new MongoDB-backed telemetry repository
func NewTelemetryRepository(mongoURI, database, collection string) (*TelemetryRepository, error) {
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

	return &TelemetryRepository{
		client:     client,
		database:   database,
		collection: collection,
	}, nil
}

// Store stores a telemetry point in MongoDB
func (r *TelemetryRepository) Store(telemetry *domain.TelemetryPoint) error {
	coll := r.client.Database(r.database).Collection(r.collection)

	_, err := coll.InsertOne(context.Background(), telemetry)
	if err != nil {
		return fmt.Errorf("failed to insert telemetry: %w", err)
	}

	return nil
}

// GetByGPU retrieves telemetry for a specific GPU, ordered by timestamp
func (r *TelemetryRepository) GetByGPU(gpuUUID string, filter storage.TimeFilter) ([]*domain.TelemetryPoint, error) {
	coll := r.client.Database(r.database).Collection(r.collection)

	// Build filter
	queryFilter := bson.M{"gpu_uuid": gpuUUID}

	if filter.StartTime != nil || filter.EndTime != nil {
		timeFilter := bson.M{}
		if filter.StartTime != nil {
			timeFilter["$gte"] = *filter.StartTime
		}
		if filter.EndTime != nil {
			timeFilter["$lte"] = *filter.EndTime
		}
		queryFilter["timestamp"] = timeFilter
	}

	// Sort by timestamp ascending
	opts := options.Find().SetSort(bson.D{{Key: "timestamp", Value: 1}})

	cursor, err := coll.Find(context.Background(), queryFilter, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to query telemetry: %w", err)
	}
	defer cursor.Close(context.Background())

	var results []domain.TelemetryPoint
	if err := cursor.All(context.Background(), &results); err != nil {
		return nil, fmt.Errorf("failed to decode telemetry: %w", err)
	}

	// Convert to pointers
	pointers := make([]*domain.TelemetryPoint, len(results))
	for i := range results {
		pointers[i] = &results[i]
	}

	return pointers, nil
}

// Count returns the total number of telemetry points
func (r *TelemetryRepository) Count() int64 {
	coll := r.client.Database(r.database).Collection(r.collection)
	count, err := coll.CountDocuments(context.Background(), bson.M{})
	if err != nil {
		return 0
	}
	return count
}

// Close closes the MongoDB connection
func (r *TelemetryRepository) Close(ctx context.Context) error {
	return r.client.Disconnect(ctx)
}
