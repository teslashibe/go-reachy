// Package protocol defines the WebSocket message types for robot-cloud communication.
// This package is shared between go-eva (robot) and go-reachy (cloud).
package protocol

import (
	"encoding/json"
	"fmt"
	"time"
)

// MessageType identifies the type of WebSocket message
type MessageType string

const (
	// Robot → Cloud messages
	TypeFrame MessageType = "frame" // Video frame
	TypeDOA   MessageType = "doa"   // Direction of arrival
	TypeMic   MessageType = "mic"   // Microphone audio
	TypeState MessageType = "state" // Robot state

	// Cloud → Robot messages
	TypeMotor   MessageType = "motor"   // Motor command
	TypeSpeak   MessageType = "speak"   // TTS audio playback
	TypeEmotion MessageType = "emotion" // Play emotion animation
	TypeConfig  MessageType = "config"  // Configuration update

	// Bidirectional
	TypePing MessageType = "ping" // Health check
	TypePong MessageType = "pong" // Health check response
)

// Message is the base wrapper for all WebSocket messages
type Message struct {
	Type      MessageType     `json:"type"`
	Timestamp int64           `json:"ts,omitempty"` // Unix milliseconds
	Data      json.RawMessage `json:"data,omitempty"`
}

// NewMessage creates a new message with the current timestamp
func NewMessage(msgType MessageType, data interface{}) (*Message, error) {
	var rawData json.RawMessage
	if data != nil {
		var err error
		rawData, err = json.Marshal(data)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal message data: %w", err)
		}
	}

	return &Message{
		Type:      msgType,
		Timestamp: time.Now().UnixMilli(),
		Data:      rawData,
	}, nil
}

// ParseData unmarshals the message data into the provided struct
func (m *Message) ParseData(v interface{}) error {
	if m.Data == nil {
		return nil
	}
	return json.Unmarshal(m.Data, v)
}

// Bytes returns the JSON-encoded message
func (m *Message) Bytes() ([]byte, error) {
	return json.Marshal(m)
}

// ParseMessage parses a JSON message from bytes
func ParseMessage(data []byte) (*Message, error) {
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("failed to parse message: %w", err)
	}
	return &msg, nil
}

// =============================================================================
// Robot → Cloud Message Types
// =============================================================================

// FrameData contains a video frame
type FrameData struct {
	Width   int    `json:"width"`
	Height  int    `json:"height"`
	Format  string `json:"format"` // "jpeg", "h264"
	Data    string `json:"data"`   // base64 encoded
	FrameID uint64 `json:"frame_id,omitempty"`
}

// DOAData contains direction of arrival information
type DOAData struct {
	Angle           float64 `json:"angle"`            // Radians, robot-relative
	SmoothedAngle   float64 `json:"smoothed_angle"`   // EMA-smoothed angle
	Speaking        bool    `json:"speaking"`         // Voice activity detected
	SpeakingLatched bool    `json:"speaking_latched"` // Latched speaking state
	Confidence      float64 `json:"confidence"`       // 0.0 to 1.0

	// Enhanced 3D positioning data (from XVF3800 speech energy)
	EstX        float64    `json:"est_x,omitempty"`        // Estimated forward distance (meters)
	EstY        float64    `json:"est_y,omitempty"`        // Estimated lateral position (meters, + = left)
	TotalEnergy float64    `json:"total_energy,omitempty"` // Total speech energy (higher = closer)
	MicEnergy   [4]float64 `json:"mic_energy,omitempty"`   // Per-mic speech energy
}

// MicData contains microphone audio
type MicData struct {
	Format     string `json:"format"`      // "pcm16", "opus"
	SampleRate int    `json:"sample_rate"` // e.g., 16000
	Channels   int    `json:"channels"`    // 1 for mono
	Data       string `json:"data"`        // base64 encoded
}

// StateData contains robot state information
type StateData struct {
	Connected bool           `json:"connected"`
	Joints    *JointState    `json:"joints,omitempty"`
	HeadPose  *HeadPoseState `json:"head_pose,omitempty"`
}

// JointState contains joint positions
type JointState struct {
	NeckRoll     float64 `json:"neck_roll"`
	NeckPitch    float64 `json:"neck_pitch"`
	NeckYaw      float64 `json:"neck_yaw"`
	LeftAntenna  float64 `json:"l_antenna"`
	RightAntenna float64 `json:"r_antenna"`
	BodyYaw      float64 `json:"body_yaw"`
}

// HeadPoseState contains the head pose
type HeadPoseState struct {
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	Z     float64 `json:"z"`
	Roll  float64 `json:"roll"`
	Pitch float64 `json:"pitch"`
	Yaw   float64 `json:"yaw"`
}

// =============================================================================
// Cloud → Robot Message Types
// =============================================================================

// MotorCommand contains motor movement instructions
type MotorCommand struct {
	Head     HeadTarget `json:"head"`
	Antennas [2]float64 `json:"antennas"` // [left, right]
	BodyYaw  float64    `json:"body_yaw"`
}

// HeadTarget specifies head position
type HeadTarget struct {
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	Z     float64 `json:"z"`
	Roll  float64 `json:"roll"`
	Pitch float64 `json:"pitch"`
	Yaw   float64 `json:"yaw"`
}

// SpeakData contains TTS audio to play
type SpeakData struct {
	Format     string `json:"format"`      // "pcm16", "mp3"
	SampleRate int    `json:"sample_rate"` // e.g., 22050
	Channels   int    `json:"channels"`    // 1 for mono
	Data       string `json:"data"`        // base64 encoded
}

// EmotionCommand triggers an emotion animation
type EmotionCommand struct {
	Name     string  `json:"name"`               // "happy", "sad", "surprised", etc.
	Duration float64 `json:"duration,omitempty"` // Duration in seconds, 0 for default
}

// ConfigUpdate contains configuration changes
type ConfigUpdate struct {
	Camera *CameraConfig `json:"camera,omitempty"`
	Audio  *AudioConfig  `json:"audio,omitempty"`
}

// CameraConfig contains camera settings
type CameraConfig struct {
	Width     int    `json:"width,omitempty"`
	Height    int    `json:"height,omitempty"`
	Framerate int    `json:"framerate,omitempty"`
	Quality   int    `json:"quality,omitempty"`
	Preset    string `json:"preset,omitempty"` // "720p", "1080p", "4k"
}

// AudioConfig contains audio settings
type AudioConfig struct {
	MicEnabled     bool `json:"mic_enabled,omitempty"`
	SpeakerEnabled bool `json:"speaker_enabled,omitempty"`
	Volume         int  `json:"volume,omitempty"` // 0-100
}

// =============================================================================
// Bidirectional Message Types
// =============================================================================

// PingData contains ping information
type PingData struct {
	ID        string `json:"id"`
	Timestamp int64  `json:"ts"`
}

// PongData contains pong response
type PongData struct {
	ID        string `json:"id"`
	PingTS    int64  `json:"ping_ts"`
	PongTS    int64  `json:"pong_ts"`
	LatencyMs int64  `json:"latency_ms"`
}

