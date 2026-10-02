package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"endpoint-monitor/internal/check"
	"endpoint-monitor/internal/monitor"
	"endpoint-monitor/internal/probe"
)

func TestManualCheckHTTP200(t *testing.T) {
	truncateMonitors(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	router := mustRouterWithProbe(t, ts)
	created := createMonitor(t, router, "OK", ts.URL, 60)

	rec := performJSON(t, router, http.MethodPost, "/api/monitors/"+itoa(created.ID)+"/check", nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected %d, got %d body=%s", http.StatusCreated, rec.Code, rec.Body.String())
	}

	var saved check.Check
	if err := json.Unmarshal(rec.Body.Bytes(), &saved); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if saved.StatusCode == nil || *saved.StatusCode != 200 {
		t.Fatalf("expected status_code 200, got %#v", saved.StatusCode)
	}
	if saved.Error != nil {
		t.Fatalf("expected null error, got %q", *saved.Error)
	}
	if saved.ResponseTimeMs == nil || *saved.ResponseTimeMs < 0 {
		t.Fatalf("expected response_time_ms, got %#v", saved.ResponseTimeMs)
	}
}

func TestManualCheckHTTP500StoredAsStatus(t *testing.T) {
	truncateMonitors(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	router := mustRouterWithProbe(t, ts)
	created := createMonitor(t, router, "Failing", ts.URL, 60)

	rec := performJSON(t, router, http.MethodPost, "/api/monitors/"+itoa(created.ID)+"/check", nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected %d, got %d body=%s", http.StatusCreated, rec.Code, rec.Body.String())
	}

	var saved check.Check
	if err := json.Unmarshal(rec.Body.Bytes(), &saved); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if saved.StatusCode == nil || *saved.StatusCode != 500 {
		t.Fatalf("expected status_code 500, got %#v", saved.StatusCode)
	}
	if saved.Error != nil {
		t.Fatalf("expected null error, got %q", *saved.Error)
	}
}

func TestManualCheckTimeout(t *testing.T) {
	truncateMonitors(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	prober := probe.NewProber(40 * time.Millisecond).WithURLValidator(probe.AllowHostValidator(hostOf(ts.URL)))
	router, err := newRouter(testPool, prober)
	if err != nil {
		t.Fatalf("new router: %v", err)
	}

	created := createMonitor(t, router, "Slow", ts.URL, 60)
	rec := performJSON(t, router, http.MethodPost, "/api/monitors/"+itoa(created.ID)+"/check", nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected %d, got %d body=%s", http.StatusCreated, rec.Code, rec.Body.String())
	}

	var saved check.Check
	if err := json.Unmarshal(rec.Body.Bytes(), &saved); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if saved.StatusCode != nil {
		t.Fatalf("expected null status_code, got %d", *saved.StatusCode)
	}
	if saved.Error == nil {
		t.Fatal("expected error on timeout")
	}
}

func TestManualCheckNetworkError(t *testing.T) {
	truncateMonitors(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	host := hostOf(ts.URL)
	closedURL := ts.URL
	ts.Close()

	prober := probe.NewProber(2 * time.Second).WithURLValidator(probe.AllowHostValidator(host))
	router, err := newRouter(testPool, prober)
	if err != nil {
		t.Fatalf("new router: %v", err)
	}

	created := createMonitor(t, router, "Down", closedURL, 60)
	rec := performJSON(t, router, http.MethodPost, "/api/monitors/"+itoa(created.ID)+"/check", nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected %d, got %d body=%s", http.StatusCreated, rec.Code, rec.Body.String())
	}

	var saved check.Check
	if err := json.Unmarshal(rec.Body.Bytes(), &saved); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if saved.StatusCode != nil {
		t.Fatalf("expected null status_code, got %d", *saved.StatusCode)
	}
	if saved.Error == nil {
		t.Fatal("expected network error")
	}
}

func TestManualCheckUnknownMonitor(t *testing.T) {
	truncateMonitors(t)
	router, err := NewRouter(testPool, nil)
	if err != nil {
		t.Fatalf("new router: %v", err)
	}

	rec := performJSON(t, router, http.MethodPost, "/api/monitors/999999/check", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected %d, got %d", http.StatusNotFound, rec.Code)
	}
}

func TestManualCheckBlocksLocalhostSSRF(t *testing.T) {
	truncateMonitors(t)
	router, err := NewRouter(testPool, nil)
	if err != nil {
		t.Fatalf("new router: %v", err)
	}

	created := createMonitor(t, router, "Local", "http://127.0.0.1/", 60)
	rec := performJSON(t, router, http.MethodPost, "/api/monitors/"+itoa(created.ID)+"/check", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected %d, got %d body=%s", http.StatusBadRequest, rec.Code, rec.Body.String())
	}
}

func TestListChecksNewestFirst(t *testing.T) {
	truncateMonitors(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	router := mustRouterWithProbe(t, ts)
	created := createMonitor(t, router, "History", ts.URL, 60)

	first := performJSON(t, router, http.MethodPost, "/api/monitors/"+itoa(created.ID)+"/check", nil)
	if first.Code != http.StatusCreated {
		t.Fatalf("first check failed: %s", first.Body.String())
	}
	time.Sleep(5 * time.Millisecond)
	second := performJSON(t, router, http.MethodPost, "/api/monitors/"+itoa(created.ID)+"/check", nil)
	if second.Code != http.StatusCreated {
		t.Fatalf("second check failed: %s", second.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/api/monitors/"+itoa(created.ID)+"/checks", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d body=%s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var checks []check.Check
	if err := json.Unmarshal(rec.Body.Bytes(), &checks); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(checks) != 2 {
		t.Fatalf("expected 2 checks, got %d", len(checks))
	}
	if !checks[0].CheckedAt.After(checks[1].CheckedAt) && checks[0].ID <= checks[1].ID {
		t.Fatalf("expected newest first, got ids %d then %d", checks[0].ID, checks[1].ID)
	}
}

func TestListChecksUnknownMonitor(t *testing.T) {
	truncateMonitors(t)
	router, err := NewRouter(testPool, nil)
	if err != nil {
		t.Fatalf("new router: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/monitors/999999/checks", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected %d, got %d", http.StatusNotFound, rec.Code)
	}
}

func mustRouterWithProbe(t *testing.T, ts *httptest.Server) http.Handler {
	t.Helper()
	prober := probe.NewProber(2 * time.Second).WithURLValidator(probe.AllowHostValidator(hostOf(ts.URL)))
	router, err := newRouter(testPool, prober)
	if err != nil {
		t.Fatalf("new router: %v", err)
	}
	return router
}

func createMonitor(t *testing.T, router http.Handler, name, rawURL string, interval int) monitor.Monitor {
	t.Helper()
	rec := performJSON(t, router, http.MethodPost, "/api/monitors", map[string]any{
		"name":             name,
		"url":              rawURL,
		"interval_seconds": interval,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create monitor failed: %s", rec.Body.String())
	}
	var created monitor.Monitor
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode monitor: %v", err)
	}
	return created
}

func hostOf(rawURL string) string {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		panic(err)
	}
	return req.URL.Host
}
