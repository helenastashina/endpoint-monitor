package checker

import (
	"context"

	"endpoint-monitor/internal/check"
	"endpoint-monitor/internal/monitor"
	"endpoint-monitor/internal/probe"

	"github.com/jackc/pgx/v5/pgxpool"
)

func Run(ctx context.Context, pool *pgxpool.Pool, prober *probe.Prober, m monitor.Monitor) (check.Check, error) {
	result, err := prober.Probe(ctx, m.URL)
	if err != nil {
		return check.Check{}, err
	}

	ms := result.ResponseTimeMs
	return check.Create(ctx, pool, m.ID, result.StatusCode, &ms, result.Error)
}
