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
	WriteURL string
	ReadURL  string
	MaxConns int32
	MinConns int32
}

// DB holds the primary pool plus the pool used for replica-safe reads.
type DB struct {
	writer *pgxpool.Pool
	reader *pgxpool.Pool

	readerOwned bool
}

// Writer returns the primary pool.
func (db *DB) Writer() *pgxpool.Pool {
	return db.writer
}

// Reader returns the pool used for list and count reads.
func (db *DB) Reader() *pgxpool.Pool {
	if db == nil || db.reader == nil {
		return nil
	}
	return db.reader
}

// Close releases every pool the DB owns.
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

// Connect opens the primary pool and, when configured, the replica pool.
// Unread counts may read a replica: a lag of a second only delays a badge.
func Connect(opts Options) (*DB, error) {
	if strings.TrimSpace(opts.WriteURL) == "" {
		return nil, fmt.Errorf("database write url is required")
	}

	writer, err := newPool(opts.WriteURL, opts)
	if err != nil {
		return nil, fmt.Errorf("connect to primary: %w", err)
	}

	if strings.TrimSpace(opts.ReadURL) == "" || opts.ReadURL == opts.WriteURL {
		return &DB{writer: writer, reader: writer}, nil
	}

	reader, err := newPool(opts.ReadURL, opts)
	if err != nil {
		writer.Close()
		return nil, fmt.Errorf("connect to read replica: %w", err)
	}

	return &DB{writer: writer, reader: reader, readerOwned: true}, nil
}

func newPool(url string, opts Options) (*pgxpool.Pool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if opts.MaxConns > 0 {
		cfg.MaxConns = opts.MaxConns
	}
	if opts.MinConns > 0 {
		cfg.MinConns = opts.MinConns
	}

	// PgBouncer in transaction mode cannot preserve named prepared statements
	// between checkouts.
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeExec

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("open pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}

	return pool, nil
}
