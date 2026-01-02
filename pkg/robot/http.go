package robot

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// httpClient is a shared HTTP client with timeout to prevent blocking.
// Used by all HTTPController instances.
var httpClient = &http.Client{
	Timeout: 2 * time.Second,
}

// HTTPController implements RobotController using the robot's HTTP API.
// This is the primary controller used by Eva for robot movement.
type HTTPController struct {
	BaseURL string
}

// NewHTTPController creates a new HTTP-based robot controller.
func NewHTTPController(robotIP string) *HTTPController {
	return &HTTPController{
		BaseURL: fmt.Sprintf("http://%s:8000", robotIP),
	}
}

// SetHeadPose sets the robot's head position (preserves body yaw).
func (r *HTTPController) SetHeadPose(roll, pitch, yaw float64) error {
	payload := map[string]interface{}{
		"target_head_pose": map[string]float64{
			"roll":  roll,
			"pitch": pitch,
			"yaw":   yaw,
		},
		"target_antennas": nil,
		"target_body_yaw": nil,
		"duration":        0.3,
	}

	return r.postMove(payload)
}

// SetAntennas sets the robot's antenna positions.
func (r *HTTPController) SetAntennas(left, right float64) error {
	payload := map[string]interface{}{
		"target_head_pose": map[string]float64{
			"roll":  0,
			"pitch": 0,
			"yaw":   0,
		},
		"target_antennas": []float64{left, right},
		"duration":        0.15,
	}

	return r.postMove(payload)
}

// SetBodyYaw rotates the robot's body (base) left or right.
func (r *HTTPController) SetBodyYaw(yaw float64) error {
	payload := map[string]interface{}{
		"target_head_pose": nil,
		"target_antennas":  nil,
		"target_body_yaw":  yaw,
		"duration":         0.5,
	}

	return r.postMove(payload)
}

// GetDaemonStatus returns the robot daemon status.
func (r *HTTPController) GetDaemonStatus() (string, error) {
	resp, err := httpClient.Get(r.BaseURL + "/api/daemon/status")
	if err != nil {
		return "", fmt.Errorf("daemon status request failed: %w", err)
	}
	defer resp.Body.Close()

	var status struct {
		State string `json:"state"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return "", fmt.Errorf("failed to decode daemon status: %w", err)
	}

	return status.State, nil
}

// SetVolume sets the robot's speaker volume (0-100).
func (r *HTTPController) SetVolume(level int) error {
	if level < 0 {
		level = 0
	}
	if level > 100 {
		level = 100
	}

	payload := fmt.Sprintf(`{"volume": %d}`, level)
	resp, err := httpClient.Post(
		r.BaseURL+"/api/volume/set",
		"application/json",
		strings.NewReader(payload),
	)
	if err != nil {
		return fmt.Errorf("volume set request failed: %w", err)
	}
	resp.Body.Close()

	return nil
}

// postMove sends a movement command to the robot API.
func (r *HTTPController) postMove(payload map[string]interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal move payload: %w", err)
	}

	resp, err := httpClient.Post(
		r.BaseURL+"/api/move/set_target",
		"application/json",
		strings.NewReader(string(data)),
	)
	if err != nil {
		return fmt.Errorf("move request failed: %w", err)
	}
	resp.Body.Close()

	return nil
}


