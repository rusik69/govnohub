package httputil

import (
	"net"
	"net/http"
	"time"
)

// DefaultTransport returns an *http.Transport with sensible connection pooling defaults:
// keep-alive, limited idle connections, timeouts to prevent resource leaks.
func DefaultTransport() *http.Transport {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
}

// NewClient returns an *http.Client with the default pooled transport and an
// optional timeout. When no timeout is provided the client has no global timeout
// (callers should use context-based timeouts instead).
func NewClient(timeout ...time.Duration) *http.Client {
	c := &http.Client{
		Transport: DefaultTransport(),
	}
	if len(timeout) > 0 {
		c.Timeout = timeout[0]
	}
	return c
}
