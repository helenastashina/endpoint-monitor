package probe

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

type Result struct {
	StatusCode     *int
	ResponseTimeMs int
	Error          *string
}

type Prober struct {
	client      *http.Client
	validateURL func(ctx context.Context, rawURL string) error
}

func NewProber(timeout time.Duration) *Prober {
	return &Prober{
		client: &http.Client{
			Timeout: timeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		validateURL: ValidatePublicURL,
	}
}

func (p *Prober) WithURLValidator(validate func(ctx context.Context, rawURL string) error) *Prober {
	clone := *p
	clone.validateURL = validate
	return &clone
}

func (p *Prober) Probe(ctx context.Context, rawURL string) (Result, error) {
	if err := p.validateURL(ctx, rawURL); err != nil {
		return Result{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrInvalidURL, err)
	}

	start := time.Now()
	resp, err := p.client.Do(req)
	elapsedMs := int(time.Since(start).Milliseconds())

	if err != nil {
		msg := err.Error()
		return Result{
			ResponseTimeMs: elapsedMs,
			Error:          &msg,
		}, nil
	}
	defer resp.Body.Close()

	code := resp.StatusCode
	return Result{
		StatusCode:     &code,
		ResponseTimeMs: elapsedMs,
	}, nil
}

func AllowHostValidator(allowedHost string) func(ctx context.Context, rawURL string) error {
	return func(ctx context.Context, rawURL string) error {
		parsed, err := url.Parse(rawURL)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidURL, err)
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return ErrInvalidScheme
		}
		if parsed.Host != allowedHost {
			return ErrBlockedHost
		}
		return nil
	}
}
