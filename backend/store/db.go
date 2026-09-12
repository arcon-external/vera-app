package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Store owns the Postgres pool and schema configuration. Repository methods
// are intentionally kept behind this type so transactions remain centralized.
type Store struct {
	pool   *pgxpool.Pool
	config Config
}

// Open connects to Postgres, verifies the connection, and applies all pending
// migrations before returning a usable store.
func Open(ctx context.Context, config Config) (*Store, error) {
	pool, err := pgxpool.New(ctx, config.DSN)
	if err != nil {
		return nil, fmt.Errorf("create Postgres pool: %w", err)
	}

	store := &Store{pool: pool, config: config}
	if err := store.pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping Postgres at %s: %w", config.RedactedDSN(), err)
	}
	if err := store.Migrate(ctx); err != nil {
		pool.Close()
		return nil, err
	}

	return store, nil
}

func OpenFromEnvironment(ctx context.Context) (*Store, error) {
	config, err := LoadConfigFromEnvironment()
	if err != nil {
		return nil, err
	}
	return Open(ctx, config)
}

func (s *Store) Close() {
	if s != nil && s.pool != nil {
		s.pool.Close()
	}
}

func (s *Store) configSchema() string {
	return quoteIdentifier(s.config.Schema)
}

func (s *Store) poolOrError() (*pgxpool.Pool, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("Postgres store is not initialized")
	}
	return s.pool, nil
}
