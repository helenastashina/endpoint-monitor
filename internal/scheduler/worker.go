package scheduler

import (
	"context"
	"log"
	"sync"

	"endpoint-monitor/internal/checker"
	"endpoint-monitor/internal/probe"

	"github.com/jackc/pgx/v5/pgxpool"
)

func StartWorkers(
	wg *sync.WaitGroup,
	count int,
	jobs <-chan Job,
	done chan<- int64,
	checkCtx context.Context,
	pool *pgxpool.Pool,
	prober *probe.Prober,
) {
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				runJob(checkCtx, pool, prober, job, done)
			}
		}()
	}
}

func runJob(
	checkCtx context.Context,
	pool *pgxpool.Pool,
	prober *probe.Prober,
	job Job,
	done chan<- int64,
) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("worker: panic monitor %d: %v", job.Monitor.ID, r)
		}
		done <- job.Monitor.ID
	}()

	_, err := checker.Run(checkCtx, pool, prober, job.Monitor)
	if err != nil {
		log.Printf("worker: check monitor %d: %v", job.Monitor.ID, err)
	}
}
