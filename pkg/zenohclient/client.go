package zenohclient

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	zenoh "github.com/teslashibe/zenoh-go"
)

// Client provides a high-level interface to Zenoh for Reachy Mini.
type Client struct {
	cfg    Config
	logger *slog.Logger
	topics *Topics

	mu      sync.RWMutex
	session zenoh.Session
	closed  bool

	// Publishers (cached for reuse)
	publishers map[string]zenoh.Publisher

	// Stats
	messagesSent     atomic.Int64
	messagesReceived atomic.Int64
	reconnectCount   atomic.Int64
}

// New creates a new Zenoh client.
// Call Connect() to establish the session.
func New(cfg Config, logger *slog.Logger) (*Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	if logger == nil {
		logger = slog.Default()
	}

	return &Client{
		cfg:        cfg,
		logger:     logger,
		topics:     NewTopics(cfg.Prefix),
		publishers: make(map[string]zenoh.Publisher),
	}, nil
}

// Connect establishes the Zenoh session.
func (c *Client) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return io.ErrClosedPipe
	}

	if c.session != nil {
		return nil // Already connected
	}

	c.logger.Info("connecting to Zenoh",
		"endpoint", c.cfg.Endpoint,
		"mode", c.cfg.Mode,
	)

	var zenohCfg zenoh.Config
	if c.cfg.Mode == "peer" {
		zenohCfg = zenoh.PeerConfig()
	} else {
		zenohCfg = zenoh.ClientConfig(c.cfg.Endpoint)
	}

	session, err := zenoh.Open(zenohCfg)
	if err != nil {
		return fmt.Errorf("failed to open zenoh session: %w", err)
	}

	c.session = session

	c.logger.Info("connected to Zenoh",
		"endpoint", c.cfg.Endpoint,
		"session_id", session.Info().ID,
	)

	return nil
}

// ConnectWithRetry connects with automatic retry on failure.
func (c *Client) ConnectWithRetry(ctx context.Context) error {
	attempts := 0

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		err := c.Connect(ctx)
		if err == nil {
			return nil
		}

		attempts++
		c.reconnectCount.Add(1)

		if c.cfg.MaxReconnectAttempts > 0 && attempts >= c.cfg.MaxReconnectAttempts {
			return fmt.Errorf("max reconnect attempts (%d) reached: %w", c.cfg.MaxReconnectAttempts, err)
		}

		c.logger.Warn("zenoh connection failed, retrying",
			"error", err,
			"attempt", attempts,
			"retry_in", c.cfg.ReconnectInterval,
		)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(c.cfg.ReconnectInterval):
		}
	}
}

// Topics returns the topics helper.
func (c *Client) Topics() *Topics {
	return c.topics
}

// Session returns the underlying Zenoh session.
// Returns nil if not connected.
func (c *Client) Session() zenoh.Session {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.session
}

// IsConnected returns true if the client is connected.
func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.session != nil && !c.closed
}

// Publish publishes data to a topic.
func (c *Client) Publish(topic string, data []byte) error {
	c.mu.RLock()
	session := c.session
	c.mu.RUnlock()

	if session == nil {
		return fmt.Errorf("not connected")
	}

	// Get or create publisher
	c.mu.Lock()
	pub, exists := c.publishers[topic]
	if !exists {
		var err error
		pub, err = session.Publisher(zenoh.KeyExpr(topic))
		if err != nil {
			c.mu.Unlock()
			return fmt.Errorf("failed to create publisher for %s: %w", topic, err)
		}
		c.publishers[topic] = pub
	}
	c.mu.Unlock()

	if err := pub.Put(data); err != nil {
		return fmt.Errorf("failed to publish to %s: %w", topic, err)
	}

	c.messagesSent.Add(1)
	return nil
}

// Subscribe subscribes to a topic and calls the handler for each message.
func (c *Client) Subscribe(topic string, handler func(data []byte)) (zenoh.Subscriber, error) {
	c.mu.RLock()
	session := c.session
	c.mu.RUnlock()

	if session == nil {
		return nil, fmt.Errorf("not connected")
	}

	sub, err := session.Subscribe(zenoh.KeyExpr(topic), func(sample zenoh.Sample) {
		c.messagesReceived.Add(1)
		handler(sample.Payload)
	})

	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to %s: %w", topic, err)
	}

	c.logger.Debug("subscribed to topic", "topic", topic)

	return sub, nil
}

// Close closes the Zenoh session and releases resources.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}
	c.closed = true

	// Close publishers
	for topic, pub := range c.publishers {
		if err := pub.Close(); err != nil {
			c.logger.Warn("error closing publisher", "topic", topic, "error", err)
		}
	}
	c.publishers = nil

	// Close session
	if c.session != nil {
		if err := c.session.Close(); err != nil {
			return fmt.Errorf("failed to close session: %w", err)
		}
		c.session = nil
	}

	c.logger.Info("zenoh client closed")
	return nil
}

// Stats returns client statistics.
func (c *Client) Stats() ClientStats {
	c.mu.RLock()
	connected := c.session != nil && !c.closed
	c.mu.RUnlock()

	return ClientStats{
		Connected:        connected,
		MessagesSent:     c.messagesSent.Load(),
		MessagesReceived: c.messagesReceived.Load(),
		ReconnectCount:   c.reconnectCount.Load(),
	}
}

// ClientStats contains client statistics.
type ClientStats struct {
	Connected        bool  `json:"connected"`
	MessagesSent     int64 `json:"messages_sent"`
	MessagesReceived int64 `json:"messages_received"`
	ReconnectCount   int64 `json:"reconnect_count"`
}

