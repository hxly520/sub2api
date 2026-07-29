package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func NewPointsPool(ctx context.Context, databaseURL, schema string, maxConns int) (*pgxpool.Pool, error) {
	poolConfig, err := pointsPoolConfig(databaseURL, schema, maxConns)
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("open POINTS_DATABASE_URL: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping POINTS_DATABASE_URL: %w", err)
	}
	var currentSchema string
	if err := pool.QueryRow(ctx, `SELECT current_schema()`).Scan(&currentSchema); err != nil {
		pool.Close()
		return nil, fmt.Errorf("verify points database schema: %w", err)
	}
	if currentSchema != schema {
		pool.Close()
		return nil, fmt.Errorf("POINTS_DATABASE_SCHEMA %q does not exist or is not accessible", schema)
	}
	return pool, nil
}

func pointsPoolConfig(databaseURL, schema string, maxConns int) (*pgxpool.Config, error) {
	databaseURL = strings.TrimSpace(databaseURL)
	schema = strings.TrimSpace(schema)
	if databaseURL == "" || schema == "" || schema == "public" || schema == "information_schema" ||
		strings.HasPrefix(schema, "pg_") || maxConns < 1 || maxConns > 32 {
		return nil, fmt.Errorf("invalid points database configuration")
	}
	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse POINTS_DATABASE_URL: %w", err)
	}
	if poolConfig.ConnConfig.RuntimeParams == nil {
		poolConfig.ConnConfig.RuntimeParams = make(map[string]string)
	}
	poolConfig.ConnConfig.RuntimeParams["application_name"] = "sub2api-points"
	poolConfig.ConnConfig.RuntimeParams["search_path"] = pgx.Identifier{schema}.Sanitize() + ",public"
	poolConfig.MinConns = 0
	poolConfig.MaxConns = int32(maxConns)
	return poolConfig, nil
}
