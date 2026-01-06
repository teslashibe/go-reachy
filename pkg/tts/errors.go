package tts

import (
	"errors"
	"fmt"
)

// Sentinel errors for common error conditions.
var (
	// ErrNoAPIKey is returned when the API key is missing.
	ErrNoAPIKey = errors.New("tts: API key required")

	// ErrNoVoiceID is returned when the voice ID is missing.
	ErrNoVoiceID = errors.New("tts: voice ID required")

	// ErrStreamClosed is returned when reading from a closed stream.
	ErrStreamClosed = errors.New("tts: stream closed")

	// ErrProviderUnavailable is returned when no providers are available.
	ErrProviderUnavailable = errors.New("tts: no providers available")

	// ErrAllProvidersFailed is returned when all providers in a chain fail.
	ErrAllProvidersFailed = errors.New("tts: all providers failed")
)

// APIError represents an error response from a TTS API.
type APIError struct {
	// StatusCode is the HTTP status code.
	StatusCode int

	// Message is the error message from the API.
	Message string

	// Code is the error code from the API (if provided).
	Code string

	// Provider identifies which provider returned the error.
	Provider string
}

// Error implements the error interface.
func (e *APIError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("tts [%s]: API error %d (%s): %s", e.Provider, e.StatusCode, e.Code, e.Message)
	}
	return fmt.Sprintf("tts [%s]: API error %d: %s", e.Provider, e.StatusCode, e.Message)
}

// IsRateLimited returns true if this is a rate limit error (HTTP 429).
func (e *APIError) IsRateLimited() bool {
	return e.StatusCode == 429
}

// IsUnauthorized returns true if this is an authentication error (HTTP 401).
func (e *APIError) IsUnauthorized() bool {
	return e.StatusCode == 401
}

// IsForbidden returns true if this is a permission error (HTTP 403).
func (e *APIError) IsForbidden() bool {
	return e.StatusCode == 403
}

// IsNotFound returns true if the resource was not found (HTTP 404).
func (e *APIError) IsNotFound() bool {
	return e.StatusCode == 404
}

// IsServerError returns true if this is a server-side error (HTTP 5xx).
func (e *APIError) IsServerError() bool {
	return e.StatusCode >= 500 && e.StatusCode < 600
}

// IsRetryable returns true if the request should be retried.
func (e *APIError) IsRetryable() bool {
	return e.IsRateLimited() || e.IsServerError()
}

// ProviderError wraps an error with provider context.
type ProviderError struct {
	Provider string
	Err      error
}

// Error implements the error interface.
func (e *ProviderError) Error() string {
	return fmt.Sprintf("tts [%s]: %v", e.Provider, e.Err)
}

// Unwrap returns the underlying error.
func (e *ProviderError) Unwrap() error {
	return e.Err
}

// WrapError wraps an error with provider context.
func WrapError(provider string, err error) error {
	if err == nil {
		return nil
	}
	return &ProviderError{Provider: provider, Err: err}
}





