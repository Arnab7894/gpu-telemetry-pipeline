package mongodb

import (
	"context"
	"fmt"
	"time"

	"github.com/arnabghosh/gpu-metrics-streamer/internal/domain"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// GPURepository implements storage.GPURepository using MongoDB
type GPURepository struct {
	client     *mongo.Client
	database   string
	collection string
}

// NewGPURepository creates a new MongoDB-backed GPU repository
func NewGPURepository(mongoURI, database, collection string) (*GPURepository, error) {
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

	return &GPURepository{
		client:     client,
		database:   database,
		collection: collection,
	}, nil
}

// Store stores or updates a GPU
func (r *GPURepository) Store(gpu *domain.GPU) error {
	coll := r.client.Database(r.database).Collection(r.collection)

	opts := options.Update().SetUpsert(true)
	_, err := coll.UpdateOne(
		context.Background(),
		bson.M{"uuid": gpu.UUID},
		bson.M{"$set": gpu},
		opts,
	)

	if err != nil {
		return fmt.Errorf("failed to store GPU: %w", err)
	}

	return nil
}

// BulkStore stores or updates multiple GPUs efficiently using MongoDB bulk write
func (r *GPURepository) BulkStore(gpus []*domain.GPU) error {
	if len(gpus) == 0 {
		return nil
	}

	coll := r.client.Database(r.database).Collection(r.collection)

	// Build bulk write operations (upserts)
	models := make([]mongo.WriteModel, len(gpus))
	for i, gpu := range gpus {
		models[i] = mongo.NewUpdateOneModel().
			SetFilter(bson.M{"uuid": gpu.UUID}).
			SetUpdate(bson.M{"$set": gpu}).
			SetUpsert(true)
	}

	// Use ordered=false for better performance
	opts := options.BulkWrite().SetOrdered(false)
	_, err := coll.BulkWrite(context.Background(), models, opts)
	if err != nil {
		return fmt.Errorf("failed to bulk write GPUs: %w", err)
	}

	return nil
}

// GetByUUID retrieves a GPU by UUID
func (r *GPURepository) GetByUUID(uuid string) (*domain.GPU, error) {
	coll := r.client.Database(r.database).Collection(r.collection)

	var gpu domain.GPU
	err := coll.FindOne(context.Background(), bson.M{"uuid": uuid}).Decode(&gpu)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, domain.ErrGPUNotFound
		}
		return nil, fmt.Errorf("failed to get GPU: %w", err)
	}

	return &gpu, nil
}

// List returns all GPUs
func (r *GPURepository) List() ([]*domain.GPU, error) {
	coll := r.client.Database(r.database).Collection(r.collection)

	cursor, err := coll.Find(context.Background(), bson.M{})
	if err != nil {
		return nil, fmt.Errorf("failed to list GPUs: %w", err)
	}
	defer cursor.Close(context.Background())

	var results []domain.GPU
	if err := cursor.All(context.Background(), &results); err != nil {
		return nil, fmt.Errorf("failed to decode GPUs: %w", err)
	}

	// Convert to pointers
	pointers := make([]*domain.GPU, len(results))
	for i := range results {
		pointers[i] = &results[i]
	}

	return pointers, nil
}

// Count returns the total number of GPUs
func (r *GPURepository) Count() int64 {
	coll := r.client.Database(r.database).Collection(r.collection)
	count, err := coll.CountDocuments(context.Background(), bson.M{})
	if err != nil {
		return 0
	}
	return count
}

// Close closes the MongoDB connection
func (r *GPURepository) Close(ctx context.Context) error {
	return r.client.Disconnect(ctx)
}
