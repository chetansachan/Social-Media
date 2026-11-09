package database

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Options describes the primary and optional replica endpoints.
type Options struct {
	WriteURL        string
	ReadURL         string
	MaxConns        int
	MinConns        int
	MaxConnLifetime time.Duration
}

// DB holds the primary pool plus the pool used for replica-safe reads. Both
// point at the primary when no replica is configured.
type DB struct {
	writer *pgxpool.Pool
	reader *pgxpool.Pool

	readerOwned bool
}

// Writer returns the primary pool. Every mutation, and every read that must
// observe a write from the same request, uses it.
func (db *DB) Writer() *pgxpool.Pool {
	return db.writer
}

// Reader returns the pool used for inbox list reads.
func (db *DB) Reader() *pgxpool.Pool {
	if db == nil || db.reader == nil {
		return nil
	}
	return db.reader
}

// Close releases every pool the wrapper owns.
func (db *DB) Close() {
	if db == nil {
		return
	}
	if db.readerOwned && db.reader != nil {
		db.reader.Close()
	}
	if db.writer != nil {
		db.writer.Close()
	}
}

// Connect opens the primary pool and, when configured, the replica pool. It
// verifies both are reachable before the service starts serving traffic.
func Connect(ctx context.Context, opts Options) (*DB, error) {
	if strings.TrimSpace(opts.WriteURL) == "" {
		return nil, fmt.Errorf("database URL is empty")
	}

	writer, err := newPool(ctx, opts.WriteURL, opts)
	if err != nil {
		return nil, fmt.Errorf("connect to primary: %w", err)
	}

	if strings.TrimSpace(opts.ReadURL) == "" || opts.ReadURL == opts.WriteURL {
		return &DB{writer: writer, reader: writer}, nil
	}

	reader, err := newPool(ctx, opts.ReadURL, opts)
	if err != nil {
		writer.Close()
		return nil, fmt.Errorf("connect to read replica: %w", err)
	}

	return &DB{writer: writer, reader: reader, readerOwned: true}, nil
}

func newPool(ctx context.Context, url string, opts Options) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database URL: %w", err)
	}

	if opts.MaxConns > 0 {
		config.MaxConns = int32(opts.MaxConns)
	}
	if opts.MinConns > 0 {
		config.MinConns = int32(opts.MinConns)
	}
	if opts.MaxConnLifetime > 0 {
		config.MaxConnLifetime = opts.MaxConnLifetime
	}

	// PgBouncer in transaction mode cannot preserve named prepared statements
	// between checkouts, so use unnamed statements.
	config.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeExec

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
