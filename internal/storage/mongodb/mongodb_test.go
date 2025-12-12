package mongodb

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewGPURepository_InvalidURI(t *testing.T) {
	_, err := NewGPURepository("invalid://uri", "test", "gpus")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to connect to MongoDB")
}

func TestNewTelemetryRepository_InvalidURI(t *testing.T) {
	_, err := NewTelemetryRepository("invalid://uri", "test", "metrics")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to connect to MongoDB")
}

func TestGPURepository_BulkStoreEmpty(t *testing.T) {
	repo := &GPURepository{
		database:   "test",
		collection: "gpus",
	}
	err := repo.BulkStore(nil)
	assert.NoError(t, err, "BulkStore should handle nil slice gracefully")
}

func TestTelemetryRepository_BulkStoreEmpty(t *testing.T) {
	repo := &TelemetryRepository{
		database:   "test",
		collection: "metrics",
	}
	err := repo.BulkStore(nil)
	assert.NoError(t, err, "BulkStore should handle nil slice gracefully")
}
