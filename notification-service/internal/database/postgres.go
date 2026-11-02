package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ConnectPostgres creates a PostgreSQL connection pool and verifies that the
// database is reachable before the service starts serving traffic.
func ConnectPostgres(ctx context.Context, databaseURL string, maxOpen, maxIdle int, maxLifetime time.Duration) (*pgxpool.Pool, error) {
	if databaseURL == "" {
		return nil, fmt.Errorf("database URL is empty")
	}

	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database URL: %w", err)
	}

	if maxOpen > 0 {
		config.MaxConns = int32(maxOpen)
	}
	if maxIdle > 0 {
		config.MinConns = int32(maxIdle)
	}
	if maxLifetime > 0 {
		config.MaxConnLifetime = maxLifetime
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create database pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("unable to ping PostgreSQL: %w", err)
	}

	return pool, nil
}
