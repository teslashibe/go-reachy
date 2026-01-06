// Package emotions provides pre-recorded emotion animations for Reachy Mini.
//
// Emotions are keyframe-based animations that control head pose, antenna positions,
// and body yaw. They are loaded from JSON files and played back with interpolation.
//
// This package is a Go port of the Python reachy-mini-emotions-library.
package emotions

import "time"

// Keyframe represents a single frame of animation data.
type Keyframe struct {
	// Head is a 4x4 homogeneous transformation matrix for head pose.
	// Format: [[r00,r01,r02,tx], [r10,r11,r12,ty], [r20,r21,r22,tz], [0,0,0,1]]
	Head [4][4]float64 `json:"head"`

	// Antennas are the left and right antenna positions in radians.
	Antennas [2]float64 `json:"antennas"`

	// BodyYaw is the body rotation in radians.
	BodyYaw float64 `json:"body_yaw"`

	// CheckCollision indicates if collision checking should be enabled.
	CheckCollision bool `json:"check_collision"`
}

// EmotionData represents the raw JSON structure of an emotion file.
type EmotionData struct {
	// Description is a human-readable description of the emotion.
	Description string `json:"description"`

	// Time contains timestamps for each keyframe in seconds.
	Time []float64 `json:"time"`

	// SetTargetData contains the keyframe data for each timestamp.
	SetTargetData []Keyframe `json:"set_target_data"`
}

// Emotion represents a loaded, playable emotion.
type Emotion struct {
	// Name is the identifier for this emotion (e.g., "yes1", "sad2").
	Name string

	// Description explains when to use this emotion.
	Description string

	// Duration is the total playback time.
	Duration time.Duration

	// Keyframes contains all animation frames.
	Keyframes []Keyframe

	// Timestamps contains the time offset for each keyframe.
	Timestamps []float64

	// HasSound indicates if this emotion has an associated audio file.
	HasSound bool

	// SoundPath is the path to the optional audio file.
	SoundPath string
}

// HeadPose represents the head orientation extracted from a 4x4 matrix.
// This is a simplified representation for sending to the robot.
type HeadPose struct {
	// Roll, Pitch, Yaw in radians
	Roll  float64
	Pitch float64
	Yaw   float64

	// Position offsets in meters (usually small)
	X float64
	Y float64
	Z float64
}

// Pose represents a complete robot pose at a point in time.
type Pose struct {
	Head     HeadPose
	Antennas [2]float64
	BodyYaw  float64
}

// PlaybackState represents the current state of emotion playback.
type PlaybackState int

const (
	// StateStopped means no emotion is playing.
	StateStopped PlaybackState = iota

	// StatePlaying means an emotion is actively playing.
	StatePlaying

	// StatePaused means playback is temporarily paused.
	StatePaused
)

// String returns a human-readable state name.
func (s PlaybackState) String() string {
	switch s {
	case StateStopped:
		return "stopped"
	case StatePlaying:
		return "playing"
	case StatePaused:
		return "paused"
	default:
		return "unknown"
	}
}

// PlayerCallback is called for each interpolated pose during playback.
// Return false to stop playback early.
type PlayerCallback func(pose Pose, elapsed time.Duration) bool

// PlayerOptions configures emotion playback.
type PlayerOptions struct {
	// FrameRate is the playback interpolation rate (default: 30 Hz).
	FrameRate float64

	// Loop causes the emotion to repeat when finished.
	Loop bool

	// Speed multiplier (1.0 = normal, 2.0 = 2x speed).
	Speed float64
}

// DefaultPlayerOptions returns sensible defaults for playback.
func DefaultPlayerOptions() PlayerOptions {
	return PlayerOptions{
		FrameRate: 30.0,
		Speed:     1.0,
		Loop:      false,
	}
}


