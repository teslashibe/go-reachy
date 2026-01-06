package movement

import (
	"math"
	"sort"
	"time"

	"github.com/teslashibe/go-reachy/pkg/emotions"
	"github.com/teslashibe/go-reachy/pkg/robot"
)

// ============================================================
// EmotionMove - Plays a keyframe-based emotion animation
// ============================================================

// EmotionMove wraps an Emotion for playback as a Move.
type EmotionMove struct {
	emotion *emotions.Emotion
}

// NewEmotionMove creates a Move from an emotion.
func NewEmotionMove(emotion *emotions.Emotion) *EmotionMove {
	return &EmotionMove{emotion: emotion}
}

// Name returns the emotion name.
func (m *EmotionMove) Name() string {
	return m.emotion.Name
}

// Duration returns the emotion's total duration.
func (m *EmotionMove) Duration() time.Duration {
	return m.emotion.Duration
}

// Evaluate returns the interpolated pose at time t.
func (m *EmotionMove) Evaluate(t time.Duration) Pose {
	e := m.emotion
	ts := t.Seconds()

	// Edge cases
	if len(e.Keyframes) == 0 {
		return Zero()
	}
	if len(e.Keyframes) == 1 {
		return emotionKeyframeToPose(e.Keyframes[0])
	}

	// Find keyframe interval
	idx := sort.Search(len(e.Timestamps), func(i int) bool {
		return e.Timestamps[i] > ts
	})

	if idx == 0 {
		return emotionKeyframeToPose(e.Keyframes[0])
	}
	if idx >= len(e.Timestamps) {
		return emotionKeyframeToPose(e.Keyframes[len(e.Keyframes)-1])
	}

	// Interpolate between keyframes
	idxPrev := idx - 1
	idxNext := idx
	tPrev := e.Timestamps[idxPrev]
	tNext := e.Timestamps[idxNext]

	var alpha float64
	if tNext == tPrev {
		alpha = 0
	} else {
		alpha = (ts - tPrev) / (tNext - tPrev)
	}

	kfInterp := emotions.InterpolateKeyframes(e.Keyframes[idxPrev], e.Keyframes[idxNext], alpha)
	return emotionKeyframeToPose(kfInterp)
}

// IsComplete returns true when the emotion has finished.
func (m *EmotionMove) IsComplete(t time.Duration) bool {
	return t >= m.emotion.Duration
}

// emotionKeyframeToPose converts an emotion Keyframe to a Pose.
func emotionKeyframeToPose(kf emotions.Keyframe) Pose {
	hp := emotions.MatrixToHeadPose(kf.Head)

	// Clamp head to safe limits (matches emotions.KeyframeToPose)
	hp.Roll = clamp(hp.Roll, -0.25, 0.25)   // ±14 degrees
	hp.Pitch = clamp(hp.Pitch, -0.30, 0.30) // ±17 degrees
	hp.Yaw = clamp(hp.Yaw, -0.50, 0.50)     // ±29 degrees

	return Pose{
		Head: robot.Offset{
			Roll:  hp.Roll,
			Pitch: hp.Pitch,
			Yaw:   hp.Yaw,
		},
		Antennas: kf.Antennas,
		BodyYaw:  kf.BodyYaw,
	}
}

// ============================================================
// BreathingMove - Continuous idle breathing animation
// ============================================================

// BreathingMove provides a gentle breathing animation when idle.
type BreathingMove struct {
	frequency   float64 // Cycles per second
	amplitude   float64 // Head pitch amplitude in radians
	rollAmp     float64 // Head roll amplitude in radians
	antennaAmp  float64 // Antenna sway amplitude in radians
}

// NewBreathingMove creates a breathing animation.
func NewBreathingMove() *BreathingMove {
	return &BreathingMove{
		frequency:  0.3, // ~3 second breath cycle
		amplitude:  0.02, // Subtle head pitch
		rollAmp:    0.01, // Very subtle roll
		antennaAmp: 0.1,  // More visible antenna sway
	}
}

// Name returns "breathing".
func (m *BreathingMove) Name() string {
	return "breathing"
}

// Duration returns 0 (infinite).
func (m *BreathingMove) Duration() time.Duration {
	return 0
}

// Evaluate returns the breathing pose at time t.
func (m *BreathingMove) Evaluate(t time.Duration) Pose {
	phase := t.Seconds() * m.frequency * 2 * math.Pi

	pitch := m.amplitude * math.Sin(phase)
	roll := m.rollAmp * math.Sin(phase*0.7) // Slightly different frequency

	antennaPhase := phase * 1.2 // Slightly faster
	antennaSway := m.antennaAmp * math.Sin(antennaPhase)

	return Pose{
		Head: robot.Offset{
			Roll:  roll,
			Pitch: pitch,
			Yaw:   0,
		},
		Antennas: [2]float64{antennaSway, -antennaSway}, // Opposite directions
		BodyYaw:  0,
	}
}

// IsComplete always returns false (continuous).
func (m *BreathingMove) IsComplete(t time.Duration) bool {
	return false
}

// ============================================================
// IdleMove - Holds a static pose
// ============================================================

// IdleMove holds a static pose (e.g., neutral position).
type IdleMove struct {
	pose Pose
}

// NewIdleMove creates a static pose move.
func NewIdleMove(pose Pose) *IdleMove {
	return &IdleMove{pose: pose}
}

// NewNeutralMove creates an idle move at neutral position.
func NewNeutralMove() *IdleMove {
	return &IdleMove{pose: Zero()}
}

// Name returns "idle".
func (m *IdleMove) Name() string {
	return "idle"
}

// Duration returns 0 (infinite).
func (m *IdleMove) Duration() time.Duration {
	return 0
}

// Evaluate returns the static pose.
func (m *IdleMove) Evaluate(t time.Duration) Pose {
	return m.pose
}

// IsComplete always returns false.
func (m *IdleMove) IsComplete(t time.Duration) bool {
	return false
}

// ============================================================
// InterpolatedMove - Smooth transition between poses
// ============================================================

// InterpolatedMove smoothly transitions from start to end pose.
type InterpolatedMove struct {
	start    Pose
	end      Pose
	duration time.Duration
}

// NewInterpolatedMove creates a smooth transition.
func NewInterpolatedMove(start, end Pose, duration time.Duration) *InterpolatedMove {
	return &InterpolatedMove{
		start:    start,
		end:      end,
		duration: duration,
	}
}

// Name returns "interpolate".
func (m *InterpolatedMove) Name() string {
	return "interpolate"
}

// Duration returns the transition duration.
func (m *InterpolatedMove) Duration() time.Duration {
	return m.duration
}

// Evaluate returns the interpolated pose at time t.
func (m *InterpolatedMove) Evaluate(t time.Duration) Pose {
	if t >= m.duration {
		return m.end
	}

	alpha := t.Seconds() / m.duration.Seconds()
	// Smooth easing
	alpha = smoothstep(alpha)

	return Pose{
		Head: robot.Offset{
			Roll:  lerp(m.start.Head.Roll, m.end.Head.Roll, alpha),
			Pitch: lerp(m.start.Head.Pitch, m.end.Head.Pitch, alpha),
			Yaw:   lerp(m.start.Head.Yaw, m.end.Head.Yaw, alpha),
		},
		Antennas: [2]float64{
			lerp(m.start.Antennas[0], m.end.Antennas[0], alpha),
			lerp(m.start.Antennas[1], m.end.Antennas[1], alpha),
		},
		BodyYaw: lerp(m.start.BodyYaw, m.end.BodyYaw, alpha),
	}
}

// IsComplete returns true when transition is done.
func (m *InterpolatedMove) IsComplete(t time.Duration) bool {
	return t >= m.duration
}

// lerp performs linear interpolation.
func lerp(a, b, t float64) float64 {
	return a + (b-a)*t
}

// smoothstep provides smooth easing (slow start/end).
func smoothstep(t float64) float64 {
	if t < 0 {
		return 0
	}
	if t > 1 {
		return 1
	}
	return t * t * (3 - 2*t)
}

