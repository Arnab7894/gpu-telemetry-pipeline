package streamer

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	csvLockKey               = "streamer:csv:lock"
	lockTTL                  = 5 * time.Minute // Max time to hold lock
	lockAcquireRetryInterval = 2 * time.Second
)

// BatchLock provides distributed locking for CSV batch processing
// Ensures only one streamer instance reads and publishes a CSV batch at a time
type BatchLock struct {
	client    *redis.Client
	lockValue string // Unique identifier for this streamer instance
	logger    *slog.Logger
}

// NewBatchLock creates a new batch lock manager
func NewBatchLock(redisURL string, instanceID string, logger *slog.Logger) (*BatchLock, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("invalid redis URL: %w", err)
	}

	client := redis.NewClient(opts)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return &BatchLock{
		client:    client,
		lockValue: instanceID,
		logger:    logger,
	}, nil
}

// AcquireLock attempts to acquire the CSV processing lock
// Blocks until lock is acquired or context is cancelled
func (bl *BatchLock) AcquireLock(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			// Try to set lock with NX (only if not exists) and EX (expiration)
			success, err := bl.client.SetNX(ctx, csvLockKey, bl.lockValue, lockTTL).Result()
			if err != nil {
				bl.logger.Warn("Failed to acquire lock", "error", err)
				time.Sleep(lockAcquireRetryInterval)
				continue
			}

			if success {
				bl.logger.Info("Acquired CSV batch lock", "instance_id", bl.lockValue)
				return nil
			}

			// Lock is held by another instance, wait and retry
			bl.logger.Debug("CSV lock held by another streamer, waiting...",
				"retry_interval", lockAcquireRetryInterval,
			)

			// Interruptible sleep - allows graceful shutdown during retry wait period
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(lockAcquireRetryInterval):
				// Continue to next iteration
			}
		}
	}
}

// ReleaseLock releases the CSV processing lock
func (bl *BatchLock) ReleaseLock(ctx context.Context) error {
	// Only release if we own the lock (check value matches)
	script := `
		if redis.call("get", KEYS[1]) == ARGV[1] then
			return redis.call("del", KEYS[1])
		else
			return 0
		end
	`

	result, err := bl.client.Eval(ctx, script, []string{csvLockKey}, bl.lockValue).Int()
	if err != nil {
		return fmt.Errorf("failed to release lock: %w", err)
	}

	if result == 1 {
		bl.logger.Info("Released CSV batch lock", "instance_id", bl.lockValue)
	} else {
		bl.logger.Warn("Lock was not owned by this instance or already released",
			"instance_id", bl.lockValue,
		)
	}

	return nil
}

// ExtendLock extends the TTL of the lock (for long-running batch processing)
func (bl *BatchLock) ExtendLock(ctx context.Context) error {
	script := `
		if redis.call("get", KEYS[1]) == ARGV[1] then
			return redis.call("pexpire", KEYS[1], ARGV[2])
		else
			return 0
		end
	`

	result, err := bl.client.Eval(ctx, script, []string{csvLockKey}, bl.lockValue, int64(lockTTL.Milliseconds())).Int()
	if err != nil {
		return fmt.Errorf("failed to extend lock: %w", err)
	}

	if result == 0 {
		return fmt.Errorf("lock not owned by this instance")
	}

	bl.logger.Debug("Extended CSV batch lock", "instance_id", bl.lockValue, "ttl", lockTTL)
	return nil
}

// Close closes the Redis connection
func (bl *BatchLock) Close() error {
	if bl.client != nil {
		return bl.client.Close()
	}
	return nil
}
