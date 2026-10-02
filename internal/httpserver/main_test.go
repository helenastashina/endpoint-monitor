package httpserver

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"endpoint-monitor/internal/config"
	"endpoint-monitor/internal/db"

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

func truncateMonitors(t *testing.T) {
	t.Helper()
	unlock := lockTestDB(t)
	t.Cleanup(unlock)

	_, err := testPool.Exec(context.Background(), "TRUNCATE monitors RESTART IDENTITY CASCADE")
	if err != nil {
		t.Fatalf("truncate monitors: %v", err)
	}
}
