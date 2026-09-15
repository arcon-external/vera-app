package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"vera-app/backend/store"
)

func TestOpenStoreWithRetrySucceedsAfterFlakes(t *testing.T) {
	calls := 0
	database, err := openStoreWithRetry(context.Background(), store.Config{}, 5, time.Millisecond,
		func(context.Context, store.Config) (*store.Store, error) {
			calls++
			if calls < 3 {
				return nil, errors.New("connection refused")
			}
			return &store.Store{}, nil
		})
	if err != nil {
		t.Fatalf("retry failed: %v", err)
	}
	if database == nil {
		t.Fatal("expected a store, got nil")
	}
	if calls != 3 {
		t.Errorf("attempts = %d; want 3", calls)
	}
}

func TestOpenStoreWithRetryExhaustsBudget(t *testing.T) {
	start := time.Now()
	_, err := openStoreWithRetry(context.Background(), store.Config{}, 3, time.Millisecond,
		func(context.Context, store.Config) (*store.Store, error) {
			return nil, errors.New("connection refused")
		})
	if err == nil {
		t.Fatal("expected error after exhausted budget")
	}
	// 3 attempts, 2 millisecond delays: must fail fast, not hang.
	if time.Since(start) > 5*time.Second {
		t.Error("budget exhaustion took too long")
	}
}
