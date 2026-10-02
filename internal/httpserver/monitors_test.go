package httpserver

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"endpoint-monitor/internal/monitor"
)

func TestCreateMonitorSuccess(t *testing.T) {
	truncateMonitors(t)
	router, err := NewRouter(testPool, nil)
	if err != nil {
		t.Fatalf("new router: %v", err)
	}

	body := map[string]any{
		"name":             "GitHub",
		"url":              "https://github.com",
		"interval_seconds": 60,
	}
	rec := performJSON(t, router, http.MethodPost, "/api/monitors", body)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusCreated, rec.Code, rec.Body.String())
	}

	var created monitor.Monitor
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if created.ID == 0 {
		t.Fatal("expected non-zero id")
	}
	if created.Name != "GitHub" {
		t.Fatalf("expected name %q, got %q", "GitHub", created.Name)
	}
	if created.URL != "https://github.com" {
		t.Fatalf("expected url %q, got %q", "https://github.com", created.URL)
	}
	if created.IntervalSeconds != 60 {
		t.Fatalf("expected interval_seconds %d, got %d", 60, created.IntervalSeconds)
	}
	if !created.Enabled {
		t.Fatal("expected enabled true")
	}
}

func TestCreateMonitorEmptyName(t *testing.T) {
	truncateMonitors(t)
	router, err := NewRouter(testPool, nil)
	if err != nil {
		t.Fatalf("new router: %v", err)
	}

	body := map[string]any{
		"name":             "   ",
		"url":              "https://github.com",
		"interval_seconds": 60,
	}
	rec := performJSON(t, router, http.MethodPost, "/api/monitors", body)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestCreateMonitorEmptyURL(t *testing.T) {
	truncateMonitors(t)
	router, err := NewRouter(testPool, nil)
	if err != nil {
		t.Fatalf("new router: %v", err)
	}

	body := map[string]any{
		"name":             "GitHub",
		"url":              "",
		"interval_seconds": 60,
	}
	rec := performJSON(t, router, http.MethodPost, "/api/monitors", body)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestCreateMonitorInvalidInterval(t *testing.T) {
	truncateMonitors(t)
	router, err := NewRouter(testPool, nil)
	if err != nil {
		t.Fatalf("new router: %v", err)
	}

	body := map[string]any{
		"name":             "GitHub",
		"url":              "https://github.com",
		"interval_seconds": 0,
	}
	rec := performJSON(t, router, http.MethodPost, "/api/monitors", body)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestGetMonitorByID(t *testing.T) {
	truncateMonitors(t)
	router, err := NewRouter(testPool, nil)
	if err != nil {
		t.Fatalf("new router: %v", err)
	}

	createRec := performJSON(t, router, http.MethodPost, "/api/monitors", map[string]any{
		"name":             "GitHub",
		"url":              "https://github.com",
		"interval_seconds": 60,
	})
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create failed: %s", createRec.Body.String())
	}

	var created monitor.Monitor
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/monitors/"+itoa(created.ID), nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var got monitor.Monitor
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.ID != created.ID {
		t.Fatalf("expected id %d, got %d", created.ID, got.ID)
	}
	if got.Name != created.Name {
		t.Fatalf("expected name %q, got %q", created.Name, got.Name)
	}
}

func TestGetMonitorNotFound(t *testing.T) {
	truncateMonitors(t)
	router, err := NewRouter(testPool, nil)
	if err != nil {
		t.Fatalf("new router: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/monitors/999999", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, rec.Code)
	}
}

func TestListMonitors(t *testing.T) {
	truncateMonitors(t)
	router, err := NewRouter(testPool, nil)
	if err != nil {
		t.Fatalf("new router: %v", err)
	}

	performJSON(t, router, http.MethodPost, "/api/monitors", map[string]any{
		"name":             "GitHub",
		"url":              "https://github.com",
		"interval_seconds": 60,
	})
	performJSON(t, router, http.MethodPost, "/api/monitors", map[string]any{
		"name":             "Example",
		"url":              "https://example.com",
		"interval_seconds": 30,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/monitors", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var monitors []monitor.Monitor
	if err := json.Unmarshal(rec.Body.Bytes(), &monitors); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(monitors) != 2 {
		t.Fatalf("expected 2 monitors, got %d", len(monitors))
	}
}

func performJSON(t *testing.T, router http.Handler, method, path string, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()

	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}

	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func itoa(v int64) string {
	return strconv.FormatInt(v, 10)
}
