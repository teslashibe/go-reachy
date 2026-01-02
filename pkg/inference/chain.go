package inference

import (
	"context"
	"log/slog"
)

// Chain tries multiple providers in order until one succeeds.
type Chain struct {
	providers []Provider
	logger    *slog.Logger
}

// NewChain creates a provider chain.
// At least one provider is required.
func NewChain(providers ...Provider) (*Chain, error) {
	if len(providers) == 0 {
		return nil, ErrProviderUnavailable
	}
	return &Chain{
		providers: providers,
		logger:    slog.Default().With("component", "inference.chain"),
	}, nil
}

// NewChainWithLogger creates a provider chain with a custom logger.
func NewChainWithLogger(logger *slog.Logger, providers ...Provider) (*Chain, error) {
	chain, err := NewChain(providers...)
	if err != nil {
		return nil, err
	}
	chain.logger = logger.With("component", "inference.chain")
	return chain, nil
}

// Chat tries each provider until one succeeds.
func (c *Chain) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	var errors []error

	for i, p := range c.providers {
		if !p.Capabilities().Chat {
			continue
		}

		resp, err := p.Chat(ctx, req)
		if err == nil {
			if i > 0 {
				c.logger.Info("fallback provider succeeded",
					"provider_index", i,
				)
			}
			return resp, nil
		}

		errors = append(errors, err)
		c.logger.Warn("provider failed, trying next",
			"provider_index", i,
			"error", err,
		)

		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}

	if len(errors) == 0 {
		return nil, ErrProviderUnavailable
	}
	return nil, &ChainError{Errors: errors}
}

// Stream tries each provider until one succeeds.
func (c *Chain) Stream(ctx context.Context, req *ChatRequest) (Stream, error) {
	var errors []error

	for i, p := range c.providers {
		if !p.Capabilities().Streaming {
			continue
		}

		stream, err := p.Stream(ctx, req)
		if err == nil {
			if i > 0 {
				c.logger.Info("fallback provider stream succeeded",
					"provider_index", i,
				)
			}
			return stream, nil
		}

		errors = append(errors, err)
		c.logger.Warn("provider stream failed, trying next",
			"provider_index", i,
			"error", err,
		)

		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}

	if len(errors) == 0 {
		return nil, ErrProviderUnavailable
	}
	return nil, &ChainError{Errors: errors}
}

// Vision tries each provider that supports vision.
func (c *Chain) Vision(ctx context.Context, req *VisionRequest) (*VisionResponse, error) {
	var errors []error

	for i, p := range c.providers {
		if !p.Capabilities().Vision {
			continue
		}

		resp, err := p.Vision(ctx, req)
		if err == nil {
			if i > 0 {
				c.logger.Info("fallback provider vision succeeded",
					"provider_index", i,
				)
			}
			return resp, nil
		}

		errors = append(errors, err)
		c.logger.Warn("provider vision failed, trying next",
			"provider_index", i,
			"error", err,
		)

		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}

	if len(errors) == 0 {
		return nil, ErrVisionNotSupported
	}
	return nil, &ChainError{Errors: errors}
}

// Embed tries each provider that supports embeddings.
func (c *Chain) Embed(ctx context.Context, req *EmbedRequest) (*EmbedResponse, error) {
	var errors []error

	for i, p := range c.providers {
		if !p.Capabilities().Embeddings {
			continue
		}

		resp, err := p.Embed(ctx, req)
		if err == nil {
			if i > 0 {
				c.logger.Info("fallback provider embed succeeded",
					"provider_index", i,
				)
			}
			return resp, nil
		}

		errors = append(errors, err)
		c.logger.Warn("provider embed failed, trying next",
			"provider_index", i,
			"error", err,
		)

		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}

	if len(errors) == 0 {
		return nil, ErrEmbeddingsNotSupported
	}
	return nil, &ChainError{Errors: errors}
}

// Capabilities returns combined capabilities of all providers.
func (c *Chain) Capabilities() Capabilities {
	var caps Capabilities
	for _, p := range c.providers {
		pc := p.Capabilities()
		caps.Chat = caps.Chat || pc.Chat
		caps.Vision = caps.Vision || pc.Vision
		caps.Streaming = caps.Streaming || pc.Streaming
		caps.Tools = caps.Tools || pc.Tools
		caps.Embeddings = caps.Embeddings || pc.Embeddings
	}
	return caps
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
		return WrapError("chain", lastErr)
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

// Verify Chain implements Provider at compile time.
var _ Provider = (*Chain)(nil)
