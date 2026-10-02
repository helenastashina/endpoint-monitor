package probe_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"endpoint-monitor/internal/probe"
)

func TestProbeHTTP200(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	prober := probe.NewProber(2 * time.Second).WithURLValidator(probe.AllowHostValidator(hostOf(ts.URL)))
	result, err := prober.Probe(context.Background(), ts.URL)
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if result.StatusCode == nil || *result.StatusCode != 200 {
		t.Fatalf("expected status 200, got %#v", result.StatusCode)
	}
	if result.Error != nil {
		t.Fatalf("expected nil error, got %q", *result.Error)
	}
	if result.ResponseTimeMs < 0 {
		t.Fatalf("expected non-negative response time")
	}
}

func TestProbeHTTP500IsStatusNotError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	prober := probe.NewProber(2 * time.Second).WithURLValidator(probe.AllowHostValidator(hostOf(ts.URL)))
	result, err := prober.Probe(context.Background(), ts.URL)
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if result.StatusCode == nil || *result.StatusCode != 500 {
		t.Fatalf("expected status 500, got %#v", result.StatusCode)
	}
	if result.Error != nil {
		t.Fatalf("expected nil error for HTTP 500, got %q", *result.Error)
	}
}

func TestProbeTimeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	prober := probe.NewProber(50 * time.Millisecond).WithURLValidator(probe.AllowHostValidator(hostOf(ts.URL)))
	result, err := prober.Probe(context.Background(), ts.URL)
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if result.StatusCode != nil {
		t.Fatalf("expected nil status code on timeout, got %d", *result.StatusCode)
	}
	if result.Error == nil {
		t.Fatal("expected timeout error")
	}
	if result.ResponseTimeMs <= 0 {
		t.Fatalf("expected response time to be measured on timeout")
	}
}

func TestProbeConnectionError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	host := hostOf(ts.URL)
	ts.Close()

	prober := probe.NewProber(2 * time.Second).WithURLValidator(probe.AllowHostValidator(host))
	result, err := prober.Probe(context.Background(), "http://"+host+"/")
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if result.StatusCode != nil {
		t.Fatalf("expected nil status code, got %d", *result.StatusCode)
	}
	if result.Error == nil {
		t.Fatal("expected connection error")
	}
}

func TestValidatePublicURLBlocksLocalhost(t *testing.T) {
	err := probe.ValidatePublicURL(context.Background(), "http://localhost/health")
	if !errors.Is(err, probe.ErrBlockedHost) {
		t.Fatalf("expected ErrBlockedHost, got %v", err)
	}
}

func TestValidatePublicURLBlocksPrivateIP(t *testing.T) {
	err := probe.ValidatePublicURL(context.Background(), "http://10.0.0.1/")
	if !errors.Is(err, probe.ErrBlockedHost) {
		t.Fatalf("expected ErrBlockedHost, got %v", err)
	}
}

func TestValidatePublicURLBlocksLoopbackIP(t *testing.T) {
	err := probe.ValidatePublicURL(context.Background(), "http://127.0.0.1/")
	if !errors.Is(err, probe.ErrBlockedHost) {
		t.Fatalf("expected ErrBlockedHost, got %v", err)
	}
}

func TestProbeRedirectNotFollowed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "http://127.0.0.1/private")
		w.WriteHeader(http.StatusFound)
	}))
	defer ts.Close()

	prober := probe.NewProber(2 * time.Second).WithURLValidator(probe.AllowHostValidator(hostOf(ts.URL)))
	result, err := prober.Probe(context.Background(), ts.URL)
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if result.StatusCode == nil || *result.StatusCode != http.StatusFound {
		t.Fatalf("expected status 302, got %#v", result.StatusCode)
	}
	if result.Error != nil {
		t.Fatalf("expected nil error for redirect response, got %q", *result.Error)
	}
}

func TestValidatePublicURLBlocksNonHTTP(t *testing.T) {
	err := probe.ValidatePublicURL(context.Background(), "ftp://example.com/")
	if !errors.Is(err, probe.ErrInvalidScheme) {
		t.Fatalf("expected ErrInvalidScheme, got %v", err)
	}
}

func hostOf(rawURL string) string {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		panic(err)
	}
	return req.URL.Host
}
