package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"endpoint-monitor/internal/config"
	"endpoint-monitor/internal/db"
	"endpoint-monitor/internal/httpserver"
	"endpoint-monitor/internal/monitor"
	"endpoint-monitor/internal/probe"
	"endpoint-monitor/internal/scheduler"
)

const probeTimeout = 10 * time.Second
const httpShutdownTimeout = 10 * time.Second

func main() {
	cfg := config.Load()

	appCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(appCtx, cfg.DatabaseURL())
	if err != nil {
		log.Fatalf("connect to database: %v", err)
	}

	prober := probe.NewProber(probeTimeout)

	jobs := make(chan scheduler.Job, cfg.JobQueueSize())
	done := make(chan int64, cfg.JobQueueSize())

	listDue := func(ctx context.Context) ([]monitor.Monitor, error) {
		return monitor.ListDue(ctx, pool)
	}
	sched := scheduler.New(listDue, jobs, done, cfg.SchedulerTick)

	schedShutdownComplete := make(chan struct{})

	var workerWG sync.WaitGroup
	scheduler.StartWorkers(&workerWG, cfg.WorkerCount, jobs, done, appCtx, pool, prober)

	var schedWG sync.WaitGroup
	schedWG.Add(1)
	go func() {
		defer schedWG.Done()
		sched.Run(appCtx, schedShutdownComplete)
	}()

	router, err := httpserver.NewRouter(pool, prober)
	if err != nil {
		log.Fatalf("create router: %v", err)
	}

	srv := &http.Server{
		Addr:    cfg.Addr(),
		Handler: router,
	}

	go func() {
		log.Printf("listening on %s", cfg.Addr())
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("http server: %v", err)
		}
	}()

	<-appCtx.Done()
	log.Println("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), httpShutdownTimeout)
	defer cancel()

	var httpWG sync.WaitGroup
	httpWG.Add(1)
	go func() {
		defer httpWG.Done()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("http shutdown: %v", err)
		}
	}()

	<-sched.EnqueueStopped()
	close(jobs)
	workerWG.Wait()
	close(schedShutdownComplete)
	schedWG.Wait()
	httpWG.Wait()

	pool.Close()
}
