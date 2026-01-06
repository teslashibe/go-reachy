// Package zenohclient provides a high-level wrapper around zenoh-go
// for Reachy Mini robot communication.
//
// This package handles:
//   - Session management with automatic reconnection
//   - Audio streaming over Zenoh topics
//   - Motor control command publishing
//   - State subscription and feedback
package zenohclient

import (
	"fmt"
	"time"
)

// Config holds Zenoh client configuration.
type Config struct {
	// Endpoint is the Zenoh router endpoint.
	// Examples: "tcp/localhost:7447", "tcp/192.168.68.83:7447"
	Endpoint string `yaml:"endpoint" json:"endpoint"`

	// Mode is the Zenoh session mode.
	// Options: "client", "peer"
	Mode string `yaml:"mode" json:"mode"`

	// Prefix is the topic prefix for all Zenoh topics.
	// Default: "reachy_mini"
	Prefix string `yaml:"prefix" json:"prefix"`

	// ReconnectInterval is how often to attempt reconnection on failure.
	ReconnectInterval time.Duration `yaml:"reconnect_interval" json:"reconnect_interval"`

	// MaxReconnectAttempts is the maximum number of reconnection attempts.
	// 0 means unlimited.
	MaxReconnectAttempts int `yaml:"max_reconnect_attempts" json:"max_reconnect_attempts"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Endpoint:             "tcp/localhost:7447",
		Mode:                 "client",
		Prefix:               "reachy_mini",
		ReconnectInterval:    2 * time.Second,
		MaxReconnectAttempts: 0, // Unlimited
	}
}

// Validate checks that the configuration is valid.
func (c *Config) Validate() error {
	if c.Endpoint == "" {
		return fmt.Errorf("endpoint is required")
	}
	if c.Mode != "client" && c.Mode != "peer" {
		return fmt.Errorf("mode must be 'client' or 'peer', got '%s'", c.Mode)
	}
	if c.Prefix == "" {
		return fmt.Errorf("prefix is required")
	}
	return nil
}

