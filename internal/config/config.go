package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"time"
)

type Config struct {
	AppHost       string
	AppPort       string
	WorkerCount   int
	SchedulerTick time.Duration
	Database      DatabaseConfig
}

type DatabaseConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	Name     string
}

func Load() Config {
	workerCount := intEnv("WORKER_COUNT", 4)
	if workerCount < 1 {
		workerCount = 1
	}

	return Config{
		AppHost:       getenv("APP_HOST", "0.0.0.0"),
		AppPort:       getenv("APP_PORT", "8080"),
		WorkerCount:   workerCount,
		SchedulerTick: time.Duration(intEnv("SCHEDULER_TICK_SECONDS", 3)) * time.Second,
		Database: DatabaseConfig{
			Host:     getenv("POSTGRES_HOST", "localhost"),
			Port:     getenv("POSTGRES_PORT", "5432"),
			User:     getenv("POSTGRES_USER", "postgres"),
			Password: getenv("POSTGRES_PASSWORD", "postgres"),
			Name:     getenv("POSTGRES_DB", "endpoint_monitor"),
		},
	}
}

func (c Config) Addr() string {
	return c.AppHost + ":" + c.AppPort
}

func (c Config) JobQueueSize() int {
	return c.WorkerCount * 2
}

func (c Config) DatabaseURL() string {
	user := url.UserPassword(c.Database.User, c.Database.Password)
	u := &url.URL{
		Scheme: "postgres",
		User:   user,
		Host:   fmt.Sprintf("%s:%s", c.Database.Host, c.Database.Port),
		Path:   c.Database.Name,
	}
	q := u.Query()
	q.Set("sslmode", "disable")
	u.RawQuery = q.Encode()
	return u.String()
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func intEnv(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return n
}
