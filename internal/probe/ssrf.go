package probe

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

var (
	ErrInvalidURL    = errors.New("invalid url")
	ErrInvalidScheme = errors.New("url scheme must be http or https")
	ErrBlockedHost   = errors.New("url host is not allowed")
)

func ValidatePublicURL(ctx context.Context, rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidURL, err)
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ErrInvalidScheme
	}

	host := parsed.Hostname()
	if host == "" {
		return ErrInvalidURL
	}

	if isBlockedHostname(host) {
		return ErrBlockedHost
	}

	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return nil
	}
	if len(ips) == 0 {
		return ErrBlockedHost
	}

	for _, ip := range ips {
		if !isPublicIP(ip) {
			return ErrBlockedHost
		}
	}

	return nil
}

func isBlockedHostname(host string) bool {
	h := strings.ToLower(host)
	if h == "localhost" || strings.HasSuffix(h, ".localhost") {
		return true
	}
	if ip := net.ParseIP(h); ip != nil {
		return !isPublicIP(ip)
	}
	return false
}

func isPublicIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return false
	}
	if ip.IsMulticast() || ip.IsUnspecified() {
		return false
	}
	return ip.IsGlobalUnicast()
}
