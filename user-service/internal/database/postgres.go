package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func Connect(dbUrl string) (*pgxpool.Pool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfgStg, err := pgxpool.ParseConfig(dbUrl)
	if err != nil {
		return nil, fmt.Errorf("error parsing config: %w", err)
	}

	conn, err := pgxpool.NewWithConfig(ctx, cfgStg)
	if err != nil {
		return nil, fmt.Errorf("error connecting to database: %w", err)
	}

	if err := conn.Ping(ctx); err != nil {
		return nil, fmt.Errorf("unable to ping the connection: %w", err)
	}

	return conn, nil
}
