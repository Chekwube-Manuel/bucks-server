package db

import (
"context"
"fmt"
"time"

"github.com/jackc/pgx/v5/pgxpool"
)

const (
maxConns        = 25
minConns        = 5
connMaxLifetime = 30 * time.Minute
)

// Connect creates and returns a pgxpool.Pool using the provided DSN.
// Pool is configured with max 25 connections, min 5 connections, and a
// 30-minute maximum connection lifetime.
func Connect(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
cfg, err := pgxpool.ParseConfig(dsn)
if err != nil {
return nil, fmt.Errorf("db: parse config: %w", err)
}

cfg.MaxConns = maxConns
cfg.MinConns = minConns
cfg.MaxConnLifetime = connMaxLifetime

pool, err := pgxpool.NewWithConfig(ctx, cfg)
if err != nil {
return nil, fmt.Errorf("db: create pool: %w", err)
}

// Verify the pool is healthy before returning it.
if err := Ping(ctx, pool); err != nil {
pool.Close()
return nil, fmt.Errorf("db: initial ping failed: %w", err)
}

return pool, nil
}

// Ping checks that the pool can acquire a connection and execute a trivial
// query. It is used both at startup (inside Connect) and by the health-check
// handler.
func Ping(ctx context.Context, pool *pgxpool.Pool) error {
conn, err := pool.Acquire(ctx)
if err != nil {
return fmt.Errorf("db: acquire connection: %w", err)
}
defer conn.Release()

if err := conn.Conn().Ping(ctx); err != nil {
return fmt.Errorf("db: ping: %w", err)
}
return nil
}
