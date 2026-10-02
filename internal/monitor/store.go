package monitor

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("monitor not found")

func Create(ctx context.Context, pool *pgxpool.Pool, name, url string, intervalSeconds int) (Monitor, error) {
	const query = `
		INSERT INTO monitors (name, url, interval_seconds)
		VALUES ($1, $2, $3)
		RETURNING id, name, url, interval_seconds, enabled, created_at, updated_at
	`

	var m Monitor
	err := pool.QueryRow(ctx, query, name, url, intervalSeconds).Scan(
		&m.ID,
		&m.Name,
		&m.URL,
		&m.IntervalSeconds,
		&m.Enabled,
		&m.CreatedAt,
		&m.UpdatedAt,
	)
	if err != nil {
		return Monitor{}, fmt.Errorf("insert monitor: %w", err)
	}

	return m, nil
}

func GetByID(ctx context.Context, pool *pgxpool.Pool, id int64) (Monitor, error) {
	const query = `
		SELECT id, name, url, interval_seconds, enabled, created_at, updated_at
		FROM monitors
		WHERE id = $1
	`

	var m Monitor
	err := pool.QueryRow(ctx, query, id).Scan(
		&m.ID,
		&m.Name,
		&m.URL,
		&m.IntervalSeconds,
		&m.Enabled,
		&m.CreatedAt,
		&m.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Monitor{}, ErrNotFound
	}
	if err != nil {
		return Monitor{}, fmt.Errorf("get monitor by id: %w", err)
	}

	return m, nil
}

func List(ctx context.Context, pool *pgxpool.Pool) ([]Monitor, error) {
	const query = `
		SELECT id, name, url, interval_seconds, enabled, created_at, updated_at
		FROM monitors
		ORDER BY id
	`

	rows, err := pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list monitors: %w", err)
	}
	defer rows.Close()

	monitors := make([]Monitor, 0)
	for rows.Next() {
		var m Monitor
		if err := rows.Scan(
			&m.ID,
			&m.Name,
			&m.URL,
			&m.IntervalSeconds,
			&m.Enabled,
			&m.CreatedAt,
			&m.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan monitor: %w", err)
		}
		monitors = append(monitors, m)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate monitors: %w", err)
	}

	return monitors, nil
}

func ListDue(ctx context.Context, pool *pgxpool.Pool) ([]Monitor, error) {
	const query = `
		SELECT m.id, m.name, m.url, m.interval_seconds, m.enabled, m.created_at, m.updated_at
		FROM monitors m
		LEFT JOIN LATERAL (
			SELECT c.checked_at
			FROM checks c
			WHERE c.monitor_id = m.id
			ORDER BY c.checked_at DESC, c.id DESC
			LIMIT 1
		) lc ON true
		WHERE m.enabled = true
		  AND (
		    lc.checked_at IS NULL
		    OR lc.checked_at < NOW() - (m.interval_seconds * INTERVAL '1 second')
		  )
		ORDER BY m.id
	`

	rows, err := pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list due monitors: %w", err)
	}
	defer rows.Close()

	monitors := make([]Monitor, 0)
	for rows.Next() {
		var m Monitor
		if err := rows.Scan(
			&m.ID,
			&m.Name,
			&m.URL,
			&m.IntervalSeconds,
			&m.Enabled,
			&m.CreatedAt,
			&m.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan due monitor: %w", err)
		}
		monitors = append(monitors, m)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate due monitors: %w", err)
	}

	return monitors, nil
}
