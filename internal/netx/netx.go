// Package netx owns Ryotunes' process-wide HTTP transport and live proxy setting.
package netx

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"
)

type proxyConfig struct {
	raw string
	url *url.URL
}

var (
	proxy     atomic.Pointer[proxyConfig]
	transport = newTransport()
)

func newTransport() *http.Transport {
	base := http.DefaultTransport.(*http.Transport).Clone()
	base.Proxy = func(request *http.Request) (*url.URL, error) {
		if configured := proxy.Load(); configured != nil {
			return configured.url, nil
		}
		return http.ProxyFromEnvironment(request)
	}
	return base
}

// Transport returns the shared connection pool. Its proxy callback observes
// SetProxy changes without replacing clients or transports.
func Transport() *http.Transport { return transport }

// Client returns an HTTP client backed by the process-wide transport.
func Client(timeout time.Duration) *http.Client {
	return &http.Client{Transport: transport, Timeout: timeout}
}

// SetProxy validates and atomically installs raw. An empty value restores the
// standard environment-proxy behavior.
func SetProxy(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		proxy.Store(nil)
		transport.CloseIdleConnections()
		return nil
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("proxy: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("proxy must be an absolute URL")
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
	default:
		return fmt.Errorf("unsupported proxy scheme %q", parsed.Scheme)
	}
	proxy.Store(&proxyConfig{raw: raw, url: parsed})
	transport.CloseIdleConnections()
	return nil
}

// Proxy returns the configured proxy URL for subprocesses such as yt-dlp.
func Proxy() string {
	if configured := proxy.Load(); configured != nil {
		return configured.raw
	}
	return ""
}
