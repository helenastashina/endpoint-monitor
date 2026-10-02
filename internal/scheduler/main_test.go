package scheduler_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"endpoint-monitor/internal/config"
	"endpoint-monitor/internal/db"
	"endpoint-monitor/internal/monitor"
	"endpoint-monitor/internal/scheduler"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	os.Setenv("POSTGRES_DB", "endpoint_monitor_test")

	cfg := config.Load()
	if !strings.HasSuffix(cfg.Database.Name, "_test") {
		fmt.Fprintf(os.Stderr, "refusing to run tests against non-test database %q\n", cfg.Database.Name)
		os.Exit(1)
	}

	ctx := context.Background()
	pool, err := db.Connect(ctx, cfg.DatabaseURL())
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect to test database: %v\n", err)
		os.Exit(1)
	}
	testPool = pool

	if err := applyMigrations(cfg.DatabaseURL()); err != nil {
		pool.Close()
		fmt.Fprintf(os.Stderr, "apply migrations: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()
	pool.Close()
	os.Exit(code)
}

func applyMigrations(databaseURL string) error {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return fmt.Errorf("resolve migrations path")
	}

	migrationsDir := filepath.Join(filepath.Dir(filename), "..", "..", "migrations")
	sourceURL := "file://" + migrationsDir
	migrateURL := strings.Replace(databaseURL, "postgres://", "pgx5://", 1)

	m, err := migrate.New(sourceURL, migrateURL)
	if err != nil {
		return err
	}
	defer m.Close()

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}

	return nil
}

func lockTestDB(t *testing.T) func() {
	t.Helper()
	_, err := testPool.Exec(context.Background(), `SELECT pg_advisory_lock(84001)`)
	if err != nil {
		t.Fatalf("lock test db: %v", err)
	}
	return func() {
		_, _ = testPool.Exec(context.Background(), `SELECT pg_advisory_unlock(84001)`)
	}
}

func truncateAll(t *testing.T) {
	t.Helper()
	unlock := lockTestDB(t)
	t.Cleanup(unlock)

	_, err := testPool.Exec(context.Background(), "TRUNCATE monitors RESTART IDENTITY CASCADE")
	if err != nil {
		t.Fatalf("truncate: %v", err)
	}
}

func createMonitor(t *testing.T, name, rawURL string, interval int) monitor.Monitor {
	t.Helper()
	m, err := monitor.Create(context.Background(), testPool, name, rawURL, interval)
	if err != nil {
		t.Fatalf("create monitor: %v", err)
	}
	return m
}

func TestListDueMonitorWithoutChecks(t *testing.T) {
	truncateAll(t)
	m := createMonitor(t, "Fresh", "https://example.com", 60)

	due, err := monitor.ListDue(context.Background(), testPool)
	if err != nil {
		t.Fatalf("list due: %v", err)
	}
	if len(due) != 1 || due[0].ID != m.ID {
		t.Fatalf("expected monitor %d due, got %#v", m.ID, due)
	}
}

func TestListDueSkipsRecentCheck(t *testing.T) {
	truncateAll(t)
	m := createMonitor(t, "Recent", "https://example.com", 3600)

	_, err := testPool.Exec(context.Background(),
		`INSERT INTO checks (monitor_id, status_code, response_time_ms) VALUES ($1, 200, 10)`,
		m.ID,
	)
	if err != nil {
		t.Fatalf("insert check: %v", err)
	}

	due, err := monitor.ListDue(context.Background(), testPool)
	if err != nil {
		t.Fatalf("list due: %v", err)
	}
	if len(due) != 0 {
		t.Fatalf("expected no due monitors, got %#v", due)
	}
}

func TestListDueSkipsDisabled(t *testing.T) {
	truncateAll(t)
	m := createMonitor(t, "Disabled", "https://example.com", 60)
	_, err := testPool.Exec(context.Background(), `UPDATE monitors SET enabled = false WHERE id = $1`, m.ID)
	if err != nil {
		t.Fatalf("disable: %v", err)
	}

	due, err := monitor.ListDue(context.Background(), testPool)
	if err != nil {
		t.Fatalf("list due: %v", err)
	}
	if len(due) != 0 {
		t.Fatalf("expected disabled monitor to be skipped, got %#v", due)
	}
}

func TestListDueAfterInterval(t *testing.T) {
	truncateAll(t)
	m := createMonitor(t, "Old", "https://example.com", 1)

	_, err := testPool.Exec(context.Background(),
		`INSERT INTO checks (monitor_id, status_code, response_time_ms, checked_at)
		 VALUES ($1, 200, 10, NOW() - INTERVAL '2 seconds')`,
		m.ID,
	)
	if err != nil {
		t.Fatalf("insert old check: %v", err)
	}

	due, err := monitor.ListDue(context.Background(), testPool)
	if err != nil {
		t.Fatalf("list due: %v", err)
	}
	if len(due) != 1 || due[0].ID != m.ID {
		t.Fatalf("expected monitor due after interval, got %#v", due)
	}
}

func TestSchedulerPollDueFromDatabase(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	truncateAll(t)
	m := createMonitor(t, "Auto", ts.URL, 60)

	jobs := make(chan scheduler.Job, 4)
	done := make(chan int64, 4)

	listDue := func(ctx context.Context) ([]monitor.Monitor, error) {
		return monitor.ListDue(ctx, testPool)
	}
	s := scheduler.New(listDue, jobs, done, 5*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	schedStop := make(chan struct{})
	defer close(schedStop)
	go s.Run(ctx, schedStop)

	select {
	case job := <-jobs:
		if job.Monitor.ID != m.ID {
			t.Fatalf("expected monitor %d, got %d", m.ID, job.Monitor.ID)
		}
	case <-time.After(300 * time.Millisecond):
		t.Fatal("expected due monitor job")
	}
}
