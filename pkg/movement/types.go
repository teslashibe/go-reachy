// Package movement provides a unified movement management system for Reachy Mini.
// It implements a primary/secondary move architecture where:
// - Primary moves (emotions, dances) control the base pose
// - Secondary moves (face tracking, speech) provide additive offsets
// - The MovementManager composes them together at 30Hz
package movement

import (
	"time"

	"github.com/teslashibe/go-reachy/pkg/robot"
)

// Pose represents a complete robot pose.
type Pose struct {
	Head     robot.Offset // Roll, pitch, yaw in radians
	Antennas [2]float64   // Left, right antenna positions
	BodyYaw  float64      // Body rotation in radians
}

// Zero returns a zero/neutral pose.
func Zero() Pose {
	return Pose{}
}

// Add returns a new Pose with other's values added.
func (p Pose) Add(other Pose) Pose {
	return Pose{
		Head: robot.Offset{
			Roll:  p.Head.Roll + other.Head.Roll,
			Pitch: p.Head.Pitch + other.Head.Pitch,
			Yaw:   p.Head.Yaw + other.Head.Yaw,
		},
		Antennas: [2]float64{
			p.Antennas[0] + other.Antennas[0],
			p.Antennas[1] + other.Antennas[1],
		},
		BodyYaw: p.BodyYaw + other.BodyYaw,
	}
}

// Clamp returns a new Pose with values clamped to safe limits.
func (p Pose) Clamp() Pose {
	return Pose{
		Head:     p.Head.Clamp(),
		Antennas: p.Antennas,
		BodyYaw:  clamp(p.BodyYaw, -2.8, 2.8), // ~160 degrees
	}
}

// Move represents an animation or motion that provides poses over time.
// Moves are "primary" - they control the base pose of the robot.
type Move interface {
	// Name returns the move identifier (for logging).
	Name() string

	// Duration returns the total duration of the move.
	// Returns 0 for infinite/continuous moves.
	Duration() time.Duration

	// Evaluate returns the pose at time t since move start.
	Evaluate(t time.Duration) Pose

	// IsComplete returns true when the move has finished.
	IsComplete(t time.Duration) bool
}

// SecondaryOffset represents additive offsets from secondary sources.
// These are composed on top of the primary move's pose.
type SecondaryOffset struct {
	FaceTracking robot.Offset // From face tracking
	Speech       robot.Offset // From speech wobble
	Audio        robot.Offset // From audio DOA
}

// Combined returns the sum of all secondary offsets.
func (s SecondaryOffset) Combined() robot.Offset {
	return robot.Offset{
		Roll:  s.FaceTracking.Roll + s.Speech.Roll + s.Audio.Roll,
		Pitch: s.FaceTracking.Pitch + s.Speech.Pitch + s.Audio.Pitch,
		Yaw:   s.FaceTracking.Yaw + s.Speech.Yaw + s.Audio.Yaw,
	}
}

// clamp restricts v to the range [min, max].
func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

