// Package worldmodel provides a spatial world model for tracking entities
// and managing Eva's focus and engagement state.
package worldmodel

import "time"

// TrackedEntity represents a person or object being tracked in world coordinates
type TrackedEntity struct {
	ID            string    // Unique identifier
	WorldAngle    float64   // Position in world coords (radians from Eva's forward)
	Confidence    float64   // 0-1, decays over time when not seen
	LastSeen      time.Time // When last detected
	LastPosition  float64   // Previous frame position (for velocity)
	Velocity      float64   // Estimated angular velocity (rad/sec)
	FramePosition float64   // Last known position in frame (0-100)

	// Audio-visual association
	AudioConfidence float64   // How confident we are this entity is speaking (0-1)
	LastAudioMatch  time.Time // When audio was last associated with this entity

	// Depth estimation
	Distance  float64 // Estimated distance in meters (0 = unknown)
	FaceWidth float64 // Normalized face width (0-1) used for depth estimation
}

// AudioSource represents a detected sound direction
type AudioSource struct {
	Angle      float64   // Direction in Eva coordinates (0=front, +left, -right)
	Confidence float64   // 0-1 confidence
	Speaking   bool      // Voice activity detected
	LastSeen   time.Time // When last updated
}


