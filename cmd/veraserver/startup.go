package main

import (
	"context"
	"fmt"
	"time"

	"vera-app/backend/store"
)

// openStoreWithRetry opens the database, retrying transient startup failures
// (e.g. a serverless database waking from sleep) with backoff. It gives up
// after maxAttempts so genuine outages still fail fast. The opener is a
// parameter so tests can substitute a stub.
func openStoreWithRetry(ctx context.Context, config store.Config, maxAttempts int, delay time.Duration, open func(context.Context, store.Config) (*store.Store, error)) (*store.Store, error) {
	var err error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		var database *store.Store
		database, err = open(ctx, config)
		if err == nil {
			return database, nil
		}
		if attempt < maxAttempts {
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("database connect cancelled: %w", ctx.Err())
			case <-time.After(delay):
			}
		}
	}
	return nil, fmt.Errorf("database connect failed after %d attempts: %w", maxAttempts, err)
}
