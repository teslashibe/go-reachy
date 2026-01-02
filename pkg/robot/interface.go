// Package robot provides interfaces and implementations for Reachy Mini robot control.
//
// This package follows the Interface Segregation Principle (ISP) by defining
// small, focused interfaces that can be composed as needed. Consumers should
// depend only on the interfaces they actually use.
package robot

// HeadController provides head movement control.
// Use this minimal interface when only head control is needed (e.g., tracking).
type HeadController interface {
	SetHeadPose(roll, pitch, yaw float64) error
}

// AntennaController provides antenna position control.
type AntennaController interface {
	SetAntennas(left, right float64) error
}

// BodyController provides body rotation control.
type BodyController interface {
	SetBodyYaw(yaw float64) error
}

// StatusController provides robot status queries.
type StatusController interface {
	GetDaemonStatus() (string, error)
}

// VolumeController provides audio volume control.
type VolumeController interface {
	SetVolume(level int) error
}

// Controller is the composite interface for full robot control.
// It combines all individual control interfaces.
// Use this when you need complete robot control capabilities.
type Controller interface {
	HeadController
	AntennaController
	BodyController
	StatusController
	VolumeController
}

// Ensure HTTPController implements Controller
var _ Controller = (*HTTPController)(nil)
