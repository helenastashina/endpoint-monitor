# Endpoint Monitor

Go-сервис для периодической проверки HTTP endpoints. Создаёт monitors, выполняет ручные и автоматические проверки и сохраняет историю результатов в PostgreSQL.

## What it does

- создание Monitor;
- ручная HTTP-проверка;
- автоматические scheduled checks через worker pool;
- хранение status code, latency и network errors;
- история checks по Monitor.

## Stack

- Go 1.27
- Gin
- PostgreSQL
- pgx
- golang-migrate
- net/http
- Docker / Docker Compose
- Go testing / httptest

## How it works

```
scheduler → jobs channel → workers → HTTP probe → PostgreSQL
```

Scheduler периодически ищет due monitors (нет checks или последний check старше `interval_seconds`) и кладёт задачи в buffered channel. In-flight map в памяти не даёт поставить один Monitor в очередь дважды, пока worker не отправит completion. При SIGINT/SIGTERM scheduler перестаёт enqueue, HTTP server уходит в Shutdown, `jobs` закрывается, workers завершаются, затем закрывается pgxpool.

## Run locally

```bash
cp .env.example .env
docker compose up --build -d postgres

docker run --rm --network endpoint-monitor_default \
  -v "$PWD/migrations:/migrations" \
  migrate/migrate:v4.18.3 \
  -path=/migrations \
  -database "postgres://postgres:postgres@postgres:5432/endpoint_monitor?sslmode=disable" \
  up

docker compose up --build -d
```

Основные env: `APP_HOST`, `APP_PORT`, `POSTGRES_*`, `WORKER_COUNT`, `SCHEDULER_TICK_SECONDS`.

## Example

```bash
curl -s http://localhost:8080/api/monitors \
  -H 'Content-Type: application/json' \
  -d '{"name":"Example","url":"https://example.com","interval_seconds":60}'

curl -s -X POST http://localhost:8080/api/monitors/1/check

curl -s http://localhost:8080/api/monitors/1/checks
```

## Tests

Тесты используют отдельную БД `endpoint_monitor_test` (имя должно оканчиваться на `_test`).

```bash
docker compose exec postgres psql -U postgres -c "CREATE DATABASE endpoint_monitor_test;"

go test ./...
go test -race ./...
```

Через Docker:

```bash
docker run --rm --network endpoint-monitor_default \
  -e POSTGRES_HOST=postgres \
  -e POSTGRES_USER=postgres \
  -e POSTGRES_PASSWORD=postgres \
  -v "$PWD":/src -w /src \
  golang:1.27-alpine go test ./...
```

## Notes

- SSRF-защита минимальная: только http/https и запрет localhost/private IP;
- redirects не follow (`CheckRedirect` → `ErrUseLastResponse`);
- in-flight state только in-memory, в рамках одного процесса;
- multi-replica / distributed locking не реализованы;
- retries не реализованы.
