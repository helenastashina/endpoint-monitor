package check

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

func Create(
	ctx context.Context,
	pool *pgxpool.Pool,
	monitorID int64,
	statusCode *int,
	responseTimeMs *int,
	errMsg *string,
) (Check, error) {
	const query = `
		INSERT INTO checks (monitor_id, status_code, response_time_ms, error)
		VALUES ($1, $2, $3, $4)
		RETURNING id, monitor_id, status_code, response_time_ms, error, checked_at
	`

	var c Check
	err := pool.QueryRow(ctx, query, monitorID, statusCode, responseTimeMs, errMsg).Scan(
		&c.ID,
		&c.MonitorID,
		&c.StatusCode,
		&c.ResponseTimeMs,
		&c.Error,
		&c.CheckedAt,
	)
	if err != nil {
		return Check{}, fmt.Errorf("insert check: %w", err)
	}

	return c, nil
}

func ListByMonitorID(ctx context.Context, pool *pgxpool.Pool, monitorID int64) ([]Check, error) {
	const query = `
		SELECT id, monitor_id, status_code, response_time_ms, error, checked_at
		FROM checks
		WHERE monitor_id = $1
		ORDER BY checked_at DESC, id DESC
	`

	rows, err := pool.Query(ctx, query, monitorID)
	if err != nil {
		return nil, fmt.Errorf("list checks: %w", err)
	}
	defer rows.Close()

	checks := make([]Check, 0)
	for rows.Next() {
		var c Check
		if err := rows.Scan(
			&c.ID,
			&c.MonitorID,
			&c.StatusCode,
			&c.ResponseTimeMs,
			&c.Error,
			&c.CheckedAt,
		); err != nil {
			return nil, fmt.Errorf("scan check: %w", err)
		}
		checks = append(checks, c)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate checks: %w", err)
	}

	return checks, nil
}
