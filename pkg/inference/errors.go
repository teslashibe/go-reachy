package inference

import (
	"errors"
	"fmt"
)

// Sentinel errors for common conditions.
var (
	// ErrNoAPIKey is returned when API key is required but missing.
	ErrNoAPIKey = errors.New("inference: API key required")

	// ErrNoModel is returned when model is required but missing.
	ErrNoModel = errors.New("inference: model required")

	// ErrProviderUnavailable is returned when no providers are available.
	ErrProviderUnavailable = errors.New("inference: provider unavailable")

	// ErrAllProvidersFailed is returned when all providers in a chain fail.
	ErrAllProvidersFailed = errors.New("inference: all providers failed")

	// ErrStreamClosed is returned when reading from a closed stream.
	ErrStreamClosed = errors.New("inference: stream closed")

	// ErrVisionNotSupported is returned when vision is not supported.
	ErrVisionNotSupported = errors.New("inference: vision not supported by provider")

	// ErrEmbeddingsNotSupported is returned when embeddings are not supported.
	ErrEmbeddingsNotSupported = errors.New("inference: embeddings not supported by provider")
)

// APIError represents an error response from an inference API.
type APIError struct {
	// StatusCode is the HTTP status code.
	StatusCode int

	// Message is the error message from the API.
	Message string

	// Code is the error code (if provided).
	Code string

	// Provider identifies which provider returned the error.
	Provider string
}

// Error implements the error interface.
func (e *APIError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("inference [%s]: API error %d (%s): %s",
			e.Provider, e.StatusCode, e.Code, e.Message)
	}
	return fmt.Sprintf("inference [%s]: API error %d: %s",
		e.Provider, e.StatusCode, e.Message)
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
	return fmt.Sprintf("inference [%s]: %v", e.Provider, e.Err)
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

// ChainError aggregates errors from all providers in a chain.
type ChainError struct {
	Errors []error
}

// Error implements the error interface.
func (e *ChainError) Error() string {
	if len(e.Errors) == 0 {
		return "inference chain: no errors recorded"
	}
	if len(e.Errors) == 1 {
		return fmt.Sprintf("inference chain: %v", e.Errors[0])
	}
	return fmt.Sprintf("inference chain: all %d providers failed, last error: %v",
		len(e.Errors), e.Errors[len(e.Errors)-1])
}

// Unwrap returns the last error in the chain.
func (e *ChainError) Unwrap() error {
	if len(e.Errors) == 0 {
		return nil
	}
	return e.Errors[len(e.Errors)-1]
}

