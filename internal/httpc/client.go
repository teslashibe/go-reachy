// Package httpc provides a shared HTTP client with sensible defaults.
// Use this instead of http.DefaultClient to ensure timeouts are set.
package httpc

import (
	"net"
	"net/http"
	"time"
)

// Default timeouts for HTTP operations.
const (
	DefaultTimeout         = 30 * time.Second
	DefaultConnectTimeout  = 10 * time.Second
	DefaultKeepAlive       = 30 * time.Second
	DefaultIdleConnTimeout = 90 * time.Second
)

// Client is a shared HTTP client with production-ready defaults.
// Use this instead of http.DefaultClient.
var Client = &http.Client{
	Timeout: DefaultTimeout,
	Transport: &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   DefaultConnectTimeout,
			KeepAlive: DefaultKeepAlive,
		}).DialContext,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       DefaultIdleConnTimeout,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	},
}

// NewClient creates a new HTTP client with the specified timeout.
// For most cases, use the shared Client variable instead.
func NewClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   DefaultConnectTimeout,
				KeepAlive: DefaultKeepAlive,
			}).DialContext,
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   10,
			IdleConnTimeout:       DefaultIdleConnTimeout,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}
}

// Get performs an HTTP GET with the shared client.
func Get(url string) (*http.Response, error) {
	return Client.Get(url)
}

// Post performs an HTTP POST with the shared client.
func Post(url, contentType string, body []byte) (*http.Response, error) {
	return Client.Post(url, contentType, nil)
}

// Do performs an HTTP request with the shared client.
func Do(req *http.Request) (*http.Response, error) {
	return Client.Do(req)
}



