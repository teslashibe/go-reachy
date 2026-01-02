package tts

import (
	"context"
	"fmt"
	"log/slog"
)

// Chain implements Provider by trying multiple providers in order.
// The first successful provider wins; if all fail, returns an aggregate error.
type Chain struct {
	providers []Provider
	logger    *slog.Logger
}

// NewChain creates a provider chain that tries providers in order.
// At least one provider is required.
func NewChain(providers ...Provider) (*Chain, error) {
	if len(providers) == 0 {
		return nil, ErrProviderUnavailable
	}

	return &Chain{
		providers: providers,
		logger:    slog.Default().With("component", "tts.chain"),
	}, nil
}

// NewChainWithLogger creates a provider chain with a custom logger.
func NewChainWithLogger(logger *slog.Logger, providers ...Provider) (*Chain, error) {
	chain, err := NewChain(providers...)
	if err != nil {
		return nil, err
	}
	chain.logger = logger.With("component", "tts.chain")
	return chain, nil
}

// Synthesize tries each provider until one succeeds.
func (c *Chain) Synthesize(ctx context.Context, text string) (*AudioResult, error) {
	var errors []error

	for i, p := range c.providers {
		result, err := p.Synthesize(ctx, text)
		if err == nil {
			if i > 0 {
				c.logger.Info("fallback provider succeeded",
					"provider_index", i,
					"chars", len(text),
				)
			}
			return result, nil
		}

		errors = append(errors, err)
		c.logger.Warn("provider failed, trying next",
			"provider_index", i,
			"error", err,
		)

		// Check if context was cancelled
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}

	return nil, &ChainError{Errors: errors}
}

// Stream tries each provider until one succeeds.
func (c *Chain) Stream(ctx context.Context, text string) (AudioStream, error) {
	var errors []error

	for i, p := range c.providers {
		stream, err := p.Stream(ctx, text)
		if err == nil {
			if i > 0 {
				c.logger.Info("fallback provider stream succeeded",
					"provider_index", i,
					"chars", len(text),
				)
			}
			return stream, nil
		}

		errors = append(errors, err)
		c.logger.Warn("provider stream failed, trying next",
			"provider_index", i,
			"error", err,
		)

		// Check if context was cancelled
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}

	return nil, &ChainError{Errors: errors}
}

// Health checks all providers and returns error if all are unhealthy.
func (c *Chain) Health(ctx context.Context) error {
	var healthy int
	var lastErr error

	for _, p := range c.providers {
		if err := p.Health(ctx); err != nil {
			lastErr = err
		} else {
			healthy++
		}
	}

	if healthy == 0 {
		return fmt.Errorf("all %d providers unhealthy: %w", len(c.providers), lastErr)
	}

	c.logger.Debug("health check complete",
		"healthy", healthy,
		"total", len(c.providers),
	)

	return nil
}

// Close closes all providers.
func (c *Chain) Close() error {
	var lastErr error
	for _, p := range c.providers {
		if err := p.Close(); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// Providers returns the list of providers in the chain.
func (c *Chain) Providers() []Provider {
	return c.providers
}

// ChainError aggregates errors from all providers in a chain.
type ChainError struct {
	Errors []error
}

// Error implements the error interface.
func (e *ChainError) Error() string {
	if len(e.Errors) == 0 {
		return "tts chain: no errors recorded"
	}
	if len(e.Errors) == 1 {
		return fmt.Sprintf("tts chain: %v", e.Errors[0])
	}
	return fmt.Sprintf("tts chain: all %d providers failed, last error: %v", len(e.Errors), e.Errors[len(e.Errors)-1])
}

// Unwrap returns the last error in the chain.
func (e *ChainError) Unwrap() error {
	if len(e.Errors) == 0 {
		return nil
	}
	return e.Errors[len(e.Errors)-1]
}

// Verify Chain implements Provider at compile time.
var _ Provider = (*Chain)(nil)
