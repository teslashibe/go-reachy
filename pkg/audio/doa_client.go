// Package audio provides DOA (Direction of Arrival) integration with go-eva
package audio

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// DOAResult represents a DOA reading from go-eva
type DOAResult struct {
	Angle      float64   `json:"angle"`      // Radians in Eva coordinates (0=front, +π/2=left, -π/2=right)
	Speaking   bool      `json:"speaking"`   // Voice activity detected
	Confidence float64   `json:"confidence"` // 0-1 confidence score
	Timestamp  time.Time `json:"timestamp"`  // When this reading was taken
	RawAngle   float64   `json:"raw_angle"`  // Original XVF3800 angle
}

// Client connects to go-eva's DOA API
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a new DOA client
func NewClient(robotIP string) *Client {
	return &Client{
		baseURL: fmt.Sprintf("http://%s:9000", robotIP),
		httpClient: &http.Client{
			Timeout: 500 * time.Millisecond,
		},
	}
}

// GetDOA fetches the current DOA reading
func (c *Client) GetDOA() (*DOAResult, error) {
	resp, err := c.httpClient.Get(c.baseURL + "/api/audio/doa")
	if err != nil {
		return nil, fmt.Errorf("DOA request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("DOA returned status %d", resp.StatusCode)
	}

	var result DOAResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("DOA decode failed: %w", err)
	}

	return &result, nil
}

// Health checks if go-eva is running
func (c *Client) Health() error {
	resp, err := c.httpClient.Get(c.baseURL + "/health")
	if err != nil {
		return fmt.Errorf("go-eva not reachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("go-eva unhealthy: status %d", resp.StatusCode)
	}

	return nil
}

