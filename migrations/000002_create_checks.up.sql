CREATE TABLE checks (
    id BIGSERIAL PRIMARY KEY,
    monitor_id BIGINT NOT NULL REFERENCES monitors(id) ON DELETE CASCADE,
    status_code INTEGER NULL,
    response_time_ms INTEGER NULL,
    error TEXT NULL,
    checked_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX checks_monitor_id_checked_at_idx ON checks (monitor_id, checked_at DESC);
