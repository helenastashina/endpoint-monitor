package check

import "time"

type Check struct {
	ID             int64     `json:"id"`
	MonitorID      int64     `json:"monitor_id"`
	StatusCode     *int      `json:"status_code"`
	ResponseTimeMs *int      `json:"response_time_ms"`
	Error          *string   `json:"error"`
	CheckedAt      time.Time `json:"checked_at"`
}
