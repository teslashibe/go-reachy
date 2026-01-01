package conversation

import (
	"errors"
	"fmt"
)

// Sentinel errors for the conversation package.
var (
	// ErrMissingAPIKey indicates the API key was not provided.
	ErrMissingAPIKey = errors.New("conversation: API key is required")

	// ErrMissingAgentID indicates the agent ID was not provided (ElevenLabs).
	ErrMissingAgentID = errors.New("conversation: agent ID is required")

	// ErrNotConnected indicates the provider is not connected.
	ErrNotConnected = errors.New("conversation: not connected")

	// ErrAlreadyConnected indicates the provider is already connected.
	ErrAlreadyConnected = errors.New("conversation: already connected")

	// ErrConnectionFailed indicates the connection could not be established.
	ErrConnectionFailed = errors.New("conversation: connection failed")

	// ErrConnectionClosed indicates the connection was closed unexpectedly.
	ErrConnectionClosed = errors.New("conversation: connection closed")

	// ErrSendFailed indicates sending a message failed.
	ErrSendFailed = errors.New("conversation: send failed")

	// ErrInvalidMessage indicates a malformed message was received.
	ErrInvalidMessage = errors.New("conversation: invalid message")

	// ErrToolCallFailed indicates a tool call could not be processed.
	ErrToolCallFailed = errors.New("conversation: tool call failed")

	// ErrSessionNotConfigured indicates ConfigureSession was not called.
	ErrSessionNotConfigured = errors.New("conversation: session not configured")

	// ErrProviderNotSupported indicates the requested provider is not available.
	ErrProviderNotSupported = errors.New("conversation: provider not supported")

	// ErrTimeout indicates an operation timed out.
	ErrTimeout = errors.New("conversation: operation timed out")

	// ErrRateLimited indicates the API rate limit was exceeded.
	ErrRateLimited = errors.New("conversation: rate limited")

	// ErrQuotaExceeded indicates the usage quota was exceeded.
	ErrQuotaExceeded = errors.New("conversation: quota exceeded")

	// ErrInvalidAudio indicates the audio format is not supported.
	ErrInvalidAudio = errors.New("conversation: invalid audio format")
)

// APIError represents an error from the conversation API.
type APIError struct {
	// StatusCode is the HTTP status code (if applicable).
	StatusCode int

	// Code is the error code from the API.
	Code string

	// Message is the human-readable error message.
	Message string

	// Type is the error type/category.
	Type string

	// Retryable indicates if the request can be retried.
	Retryable bool
}

// Error implements the error interface.
func (e *APIError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("conversation: API error [%s]: %s", e.Code, e.Message)
	}
	if e.StatusCode != 0 {
		return fmt.Sprintf("conversation: API error (HTTP %d): %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("conversation: API error: %s", e.Message)
}

// Unwrap returns nil as APIError is a leaf error.
func (e *APIError) Unwrap() error {
	return nil
}

// IsRetryable returns true if the error can be retried.
func (e *APIError) IsRetryable() bool {
	return e.Retryable
}

// NewAPIError creates a new APIError.
func NewAPIError(statusCode int, code, message string) *APIError {
	retryable := statusCode == 429 || statusCode >= 500
	return &APIError{
		StatusCode: statusCode,
		Code:       code,
		Message:    message,
		Retryable:  retryable,
	}
}

// ConnectionError represents a WebSocket connection error.
type ConnectionError struct {
	// Reason describes why the connection failed.
	Reason string

	// Cause is the underlying error.
	Cause error

	// Retryable indicates if reconnection should be attempted.
	Retryable bool
}

// Error implements the error interface.
func (e *ConnectionError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("conversation: connection error: %s: %v", e.Reason, e.Cause)
	}
	return fmt.Sprintf("conversation: connection error: %s", e.Reason)
}

// Unwrap returns the underlying cause.
func (e *ConnectionError) Unwrap() error {
	return e.Cause
}

// IsRetryable returns true if reconnection should be attempted.
func (e *ConnectionError) IsRetryable() bool {
	return e.Retryable
}

// NewConnectionError creates a new ConnectionError.
func NewConnectionError(reason string, cause error, retryable bool) *ConnectionError {
	return &ConnectionError{
		Reason:    reason,
		Cause:     cause,
		Retryable: retryable,
	}
}

// Error checking helpers.

// IsNotConnected returns true if the error indicates no connection.
func IsNotConnected(err error) bool {
	return errors.Is(err, ErrNotConnected) || errors.Is(err, ErrConnectionClosed)
}

// IsRetryable returns true if the error can be retried.
func IsRetryable(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.IsRetryable()
	}
	var connErr *ConnectionError
	if errors.As(err, &connErr) {
		return connErr.IsRetryable()
	}
	return errors.Is(err, ErrRateLimited) || errors.Is(err, ErrTimeout)
}

// IsRateLimited returns true if the error is due to rate limiting.
func IsRateLimited(err error) bool {
	if errors.Is(err, ErrRateLimited) {
		return true
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == 429
	}
	return false
}

// IsQuotaExceeded returns true if the error is due to quota exhaustion.
func IsQuotaExceeded(err error) bool {
	if errors.Is(err, ErrQuotaExceeded) {
		return true
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.Code == "quota_exceeded" || apiErr.Code == "insufficient_quota"
	}
	return false
}

