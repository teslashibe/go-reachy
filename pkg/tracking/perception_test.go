package tracking

import (
	"math"
	"testing"
)

func TestPerception_FrameToWorld(t *testing.T) {
	cfg := DefaultConfig()
	p := NewPerception(cfg, nil)

	tests := []struct {
		name          string
		framePosition float64
		headYaw       float64
		expected      float64
	}{
		{
			name:          "center of frame, head forward",
			framePosition: 50,
			headYaw:       0,
			expected:      0,
		},
		{
			name:          "left of frame, head forward",
			framePosition: 0,
			headYaw:       0,
			expected:      cfg.CameraFOV / 2, // Looking left
		},
		{
			name:          "right of frame, head forward",
			framePosition: 100,
			headYaw:       0,
			expected:      -cfg.CameraFOV / 2, // Looking right
		},
		{
			name:          "center of frame, head turned left",
			framePosition: 50,
			headYaw:       0.5,
			expected:      0.5, // Target at same angle as head
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := p.FrameToWorld(tt.framePosition, tt.headYaw)
			if math.Abs(result-tt.expected) > 0.01 {
				t.Errorf("got %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestPerception_FrameToRoomAngle(t *testing.T) {
	cfg := DefaultConfig()
	p := NewPerception(cfg, nil)

	tests := []struct {
		name          string
		framePosition float64
		headYaw       float64
		bodyYaw       float64
		expected      float64
	}{
		{
			name:          "all centered",
			framePosition: 50,
			headYaw:       0,
			bodyYaw:       0,
			expected:      0,
		},
		{
			name:          "body rotated right, face in center",
			framePosition: 50,
			headYaw:       0,
			bodyYaw:       -0.5, // Body turned right
			expected:      -0.5, // Face is to room's right
		},
		{
			name:          "body rotated left, face in center",
			framePosition: 50,
			headYaw:       0,
			bodyYaw:       0.5, // Body turned left
			expected:      0.5, // Face is to room's left
		},
		{
			name:          "body rotated, head compensating",
			framePosition: 50,
			headYaw:       0.3, // Head turned left relative to body
			bodyYaw:       -0.3, // Body turned right
			expected:      0,   // Net: face is straight ahead in room
		},
		{
			name:          "face on left of frame, body and head rotated",
			framePosition: 0, // Left edge of frame
			headYaw:       0.2,
			bodyYaw:       0.3,
			// Body-relative: 0.2 + FOV/2 = 0.2 + ~0.785 = ~0.985
			// Room: 0.3 + 0.985 = ~1.285
			expected:      0.3 + 0.2 + cfg.CameraFOV/2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := p.FrameToRoomAngle(tt.framePosition, tt.headYaw, tt.bodyYaw)
			if math.Abs(result-tt.expected) > 0.01 {
				t.Errorf("got %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestPerception_FrameToRoomAngle_BodyYawMaintainsAccuracy(t *testing.T) {
	cfg := DefaultConfig()
	p := NewPerception(cfg, nil)

	// Scenario: Face is detected at room angle 0.5
	// Body rotates, but the room angle should stay the same
	
	// Initial: face at center of frame, head at 0.5, body at 0
	// Room angle = 0 + 0.5 + 0 = 0.5
	roomAngle1 := p.FrameToRoomAngle(50, 0.5, 0)
	
	// After body rotates to 0.3:
	// If head compensates to 0.2 (to keep looking at same spot)
	// Room angle = 0.3 + 0.2 + 0 = 0.5
	roomAngle2 := p.FrameToRoomAngle(50, 0.2, 0.3)
	
	if math.Abs(roomAngle1-roomAngle2) > 0.01 {
		t.Errorf("Room angle should be maintained: got %v and %v", roomAngle1, roomAngle2)
	}
	
	if math.Abs(roomAngle1-0.5) > 0.01 {
		t.Errorf("Expected room angle 0.5, got %v", roomAngle1)
	}
}

