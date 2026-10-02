package scheduler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"endpoint-monitor/internal/monitor"
	"endpoint-monitor/internal/probe"
	"endpoint-monitor/internal/scheduler"
)

func TestShutdownWorkerCompletionNotBlocked(t *testing.T) {
	block := make(chan struct{})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-block
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	jobs := make(chan scheduler.Job, 1)
	done := make(chan int64, 1)

	listDue := func(ctx context.Context) ([]monitor.Monitor, error) {
		return nil, nil
	}

	schedStop := make(chan struct{})
	s := scheduler.New(listDue, jobs, done, time.Hour)

	appCtx, cancel := context.WithCancel(context.Background())

	var schedWG sync.WaitGroup
	schedWG.Add(1)
	go func() {
		defer schedWG.Done()
		s.Run(appCtx, schedStop)
	}()

	prober := probe.NewProber(2 * time.Second).WithURLValidator(probe.AllowHostValidator(hostOf(ts.URL)))

	var workerWG sync.WaitGroup
	scheduler.StartWorkers(&workerWG, 1, jobs, done, appCtx, testPool, prober)

	jobs <- scheduler.Job{Monitor: monitor.Monitor{
		ID:  1,
		URL: ts.URL,
	}}

	time.Sleep(20 * time.Millisecond)
	cancel()

	finished := make(chan struct{})
	go func() {
		<-s.EnqueueStopped()
		close(jobs)
		workerWG.Wait()
		close(schedStop)
		schedWG.Wait()
		close(finished)
	}()

	close(block)

	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown hung waiting for worker completion")
	}
}
