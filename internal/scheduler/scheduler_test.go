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

func TestSchedulerEnqueuesDueMonitor(t *testing.T) {
	jobs := make(chan scheduler.Job, 4)
	done := make(chan int64, 4)

	listDue := func(ctx context.Context) ([]monitor.Monitor, error) {
		return []monitor.Monitor{{
			ID:              1,
			Name:            "A",
			URL:             "http://example.com",
			IntervalSeconds: 60,
			Enabled:         true,
		}}, nil
	}

	schedStop := make(chan struct{})
	defer close(schedStop)

	s := scheduler.New(listDue, jobs, done, 5*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go s.Run(ctx, schedStop)

	select {
	case job := <-jobs:
		if job.Monitor.ID != 1 {
			t.Fatalf("expected monitor 1, got %d", job.Monitor.ID)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected job")
	}
}

func TestSchedulerSkipsNotDueFromLister(t *testing.T) {
	jobs := make(chan scheduler.Job, 4)
	done := make(chan int64, 4)

	listDue := func(ctx context.Context) ([]monitor.Monitor, error) {
		return nil, nil
	}

	schedStop := make(chan struct{})
	defer close(schedStop)

	s := scheduler.New(listDue, jobs, done, 5*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go s.Run(ctx, schedStop)

	time.Sleep(30 * time.Millisecond)

	select {
	case <-jobs:
		t.Fatal("unexpected job")
	default:
	}
}

func TestSchedulerNoDuplicateWhileInFlight(t *testing.T) {
	jobs := make(chan scheduler.Job, 4)
	done := make(chan int64, 4)

	listDue := func(ctx context.Context) ([]monitor.Monitor, error) {
		return []monitor.Monitor{{
			ID:              42,
			IntervalSeconds: 1,
			Enabled:         true,
		}}, nil
	}

	schedStop := make(chan struct{})
	defer close(schedStop)

	s := scheduler.New(listDue, jobs, done, 5*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go s.Run(ctx, schedStop)

	job := <-jobs
	time.Sleep(40 * time.Millisecond)

	select {
	case <-jobs:
		t.Fatal("duplicate job while in-flight")
	default:
	}

	done <- job.Monitor.ID
	time.Sleep(30 * time.Millisecond)

	select {
	case <-jobs:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected job after completion")
	}
}

func TestSchedulerStopsOnContextCancel(t *testing.T) {
	jobs := make(chan scheduler.Job, 4)
	done := make(chan int64, 4)

	listDue := func(ctx context.Context) ([]monitor.Monitor, error) {
		return nil, nil
	}

	schedStop := make(chan struct{})
	s := scheduler.New(listDue, jobs, done, 5*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())

	finished := make(chan struct{})
	go func() {
		s.Run(ctx, schedStop)
		close(finished)
	}()

	cancel()
	close(schedStop)

	select {
	case <-finished:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("scheduler did not stop")
	}
}

func TestWorkersContinueAfterCheckError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	truncateAll(t)

	m := createMonitor(t, "Worker", ts.URL, 60)
	prober := probe.NewProber(2 * time.Second).WithURLValidator(probe.AllowHostValidator(hostOf(ts.URL)))

	jobs := make(chan scheduler.Job, 4)
	done := make(chan int64, 4)

	var workerWG sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	scheduler.StartWorkers(&workerWG, 2, jobs, done, ctx, testPool, prober)

	jobs <- scheduler.Job{Monitor: monitor.Monitor{ID: 999999, URL: "http://127.0.0.1/"}}
	jobs <- scheduler.Job{Monitor: m}

	go func() {
		for range 2 {
			<-done
		}
	}()

	time.Sleep(100 * time.Millisecond)
	close(jobs)
	workerWG.Wait()
}

func TestMultipleWorkersProcessJobs(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	truncateAll(t)

	m1 := createMonitor(t, "One", ts.URL, 60)
	m2 := createMonitor(t, "Two", ts.URL, 60)
	prober := probe.NewProber(2 * time.Second).WithURLValidator(probe.AllowHostValidator(hostOf(ts.URL)))

	jobs := make(chan scheduler.Job, 4)
	done := make(chan int64, 4)

	var workerWG sync.WaitGroup
	scheduler.StartWorkers(&workerWG, 2, jobs, done, context.Background(), testPool, prober)

	jobs <- scheduler.Job{Monitor: m1}
	jobs <- scheduler.Job{Monitor: m2}
	close(jobs)

	completed := make(map[int64]struct{})
	for i := 0; i < 2; i++ {
		completed[<-done] = struct{}{}
	}
	workerWG.Wait()

	if len(completed) != 2 {
		t.Fatalf("expected 2 completions, got %d", len(completed))
	}
}

func hostOf(rawURL string) string {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		panic(err)
	}
	return req.URL.Host
}
