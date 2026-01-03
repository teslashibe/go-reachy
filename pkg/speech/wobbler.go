// Package speech provides speech-synchronized head movement animations.
package speech

import (
	"math"
	"sync"
)

// Tunable parameters (ported from Python speech_tapper.py)
const (
	// Audio analysis
	SampleRate = 24000  // Expected sample rate (ElevenLabs default)
	FrameMS    = 20     // Frame size for RMS calculation (ms)
	HopMS      = 10     // Hop size between updates (ms)
	FrameSize  = SampleRate * FrameMS / 1000
	HopSize    = SampleRate * HopMS / 1000

	// Voice Activity Detection thresholds (dBFS)
	VADOnThreshold  = -35.0 // dB level to trigger VAD on
	VADOffThreshold = -45.0 // dB level to trigger VAD off
	VADAttackMS     = 40    // Attack time (ms)
	VADReleaseMS    = 250   // Release time (ms)

	// Envelope follower
	EnvFollowGain = 0.65 // Smoothing factor for envelope

	// Sway parameters
	SwayMaster = 1.5 // Master amplitude multiplier

	// Oscillator frequencies (Hz)
	SwayFreqPitch = 2.2
	SwayFreqYaw   = 0.6
	SwayFreqRoll  = 1.3

	// Oscillator amplitudes (degrees, converted to radians)
	SwayAmpPitchDeg = 4.5
	SwayAmpYawDeg   = 7.5
	SwayAmpRollDeg  = 2.25

	// Loudness mapping
	SwayDBLow     = -46.0
	SwayDBHigh    = -18.0
	LoudnessGamma = 0.9
	SensDBOffset  = 4.0

	// Sway attack/release
	SwayAttackMS  = 50
	SwayReleaseMS = 250
)

// Derived constants
var (
	vadAttackFrames   = max(1, VADAttackMS/HopMS)
	vadReleaseFrames  = max(1, VADReleaseMS/HopMS)
	swayAttackFrames  = max(1, SwayAttackMS/HopMS)
	swayReleaseFrames = max(1, SwayReleaseMS/HopMS)
)

// OffsetCallback is called with computed roll, pitch, yaw offsets (radians).
type OffsetCallback func(roll, pitch, yaw float64)

// Wobbler analyzes audio and produces head movement offsets.
type Wobbler struct {
	mu       sync.Mutex
	callback OffsetCallback

	// Audio buffer
	samples []float64

	// VAD state
	vadOn    bool
	vadAbove int
	vadBelow int

	// Envelope state
	swayEnv  float64
	swayUp   int
	swayDown int

	// Oscillator phases (radians)
	phasePitch float64
	phaseYaw   float64
	phaseRoll  float64

	// Time tracking
	t float64
}

// NewWobbler creates a new speech wobbler with the given callback.
func NewWobbler(callback OffsetCallback) *Wobbler {
	// Initialize with random phases for natural variation
	return &Wobbler{
		callback:   callback,
		samples:    make([]float64, 0, FrameSize*2),
		phasePitch: 0.7, // Fixed starting phases (deterministic)
		phaseYaw:   2.1,
		phaseRoll:  4.2,
	}
}

// Reset clears the wobbler state.
func (w *Wobbler) Reset() {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.samples = w.samples[:0]
	w.vadOn = false
	w.vadAbove = 0
	w.vadBelow = 0
	w.swayEnv = 0
	w.swayUp = 0
	w.swayDown = 0
	w.t = 0
}

// Feed processes audio samples and triggers callbacks with movement offsets.
// samples: int16 PCM audio samples
// sampleRate: sample rate of the audio (will resample if needed)
func (w *Wobbler) Feed(samples []int16, sampleRate int) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if len(samples) == 0 {
		return
	}

	// Convert int16 to float64 [-1, 1]
	floats := make([]float64, len(samples))
	for i, s := range samples {
		floats[i] = float64(s) / 32768.0
	}

	// Simple resampling if needed (linear interpolation)
	if sampleRate != SampleRate {
		floats = resampleLinear(floats, sampleRate, SampleRate)
	}

	// Append to buffer
	w.samples = append(w.samples, floats...)

	// Process in hop-sized chunks
	for len(w.samples) >= HopSize {
		w.processHop()
	}
}

// FeedFloat32 processes float32 audio samples (already normalized to [-1, 1]).
func (w *Wobbler) FeedFloat32(samples []float32, sampleRate int) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if len(samples) == 0 {
		return
	}

	// Convert float32 to float64
	floats := make([]float64, len(samples))
	for i, s := range samples {
		floats[i] = float64(s)
	}

	// Simple resampling if needed
	if sampleRate != SampleRate {
		floats = resampleLinear(floats, sampleRate, SampleRate)
	}

	// Append to buffer
	w.samples = append(w.samples, floats...)

	// Process in hop-sized chunks
	for len(w.samples) >= HopSize {
		w.processHop()
	}
}

// processHop processes one hop of audio and outputs movement.
func (w *Wobbler) processHop() {
	if len(w.samples) < HopSize {
		return
	}

	// Consume hop
	hop := w.samples[:HopSize]
	w.samples = w.samples[HopSize:]

	// Need at least one frame for RMS
	if len(w.samples) < FrameSize-HopSize {
		// Not enough data yet, just advance time
		w.t += float64(HopMS) / 1000.0
		return
	}

	// Compute RMS of recent frame
	frameStart := max(0, len(w.samples)-FrameSize)
	frame := w.samples[frameStart:]
	if len(frame) < FrameSize {
		// Pad with hop data
		frame = append(hop, frame...)
	}
	db := rmsDBFS(frame)

	// VAD with hysteresis
	if db >= VADOnThreshold {
		w.vadAbove++
		w.vadBelow = 0
		if !w.vadOn && w.vadAbove >= vadAttackFrames {
			w.vadOn = true
		}
	} else if db <= VADOffThreshold {
		w.vadBelow++
		w.vadAbove = 0
		if w.vadOn && w.vadBelow >= vadReleaseFrames {
			w.vadOn = false
		}
	}

	// Sway envelope
	if w.vadOn {
		w.swayUp = min(swayAttackFrames, w.swayUp+1)
		w.swayDown = 0
	} else {
		w.swayDown = min(swayReleaseFrames, w.swayDown+1)
		w.swayUp = 0
	}

	up := float64(w.swayUp) / float64(swayAttackFrames)
	down := 1.0 - float64(w.swayDown)/float64(swayReleaseFrames)
	var target float64
	if w.vadOn {
		target = up
	} else {
		target = down
	}
	w.swayEnv += EnvFollowGain * (target - w.swayEnv)
	w.swayEnv = clamp(w.swayEnv, 0, 1)

	// Loudness gain
	loud := loudnessGain(db) * SwayMaster
	env := w.swayEnv
	w.t += float64(HopMS) / 1000.0

	// Compute oscillator outputs
	pitch := degToRad(SwayAmpPitchDeg) * loud * env *
		math.Sin(2*math.Pi*SwayFreqPitch*w.t+w.phasePitch)
	yaw := degToRad(SwayAmpYawDeg) * loud * env *
		math.Sin(2*math.Pi*SwayFreqYaw*w.t+w.phaseYaw)
	roll := degToRad(SwayAmpRollDeg) * loud * env *
		math.Sin(2*math.Pi*SwayFreqRoll*w.t+w.phaseRoll)

	// Callback with offsets
	if w.callback != nil {
		w.callback(roll, pitch, yaw)
	}
}

// Helper functions

func rmsDBFS(samples []float64) float64 {
	if len(samples) == 0 {
		return -100.0
	}
	var sum float64
	for _, s := range samples {
		sum += s * s
	}
	rms := math.Sqrt(sum/float64(len(samples)) + 1e-12)
	return 20.0 * math.Log10(rms+1e-12)
}

func loudnessGain(db float64) float64 {
	t := (db + SensDBOffset - SwayDBLow) / (SwayDBHigh - SwayDBLow)
	t = clamp(t, 0, 1)
	if LoudnessGamma != 1.0 {
		t = math.Pow(t, LoudnessGamma)
	}
	return t
}

func resampleLinear(samples []float64, srIn, srOut int) []float64 {
	if srIn == srOut || len(samples) == 0 {
		return samples
	}
	nOut := int(math.Round(float64(len(samples)) * float64(srOut) / float64(srIn)))
	if nOut <= 1 {
		return nil
	}
	out := make([]float64, nOut)
	for i := range out {
		t := float64(i) / float64(nOut-1) * float64(len(samples)-1)
		idx := int(t)
		frac := t - float64(idx)
		if idx >= len(samples)-1 {
			out[i] = samples[len(samples)-1]
		} else {
			out[i] = samples[idx]*(1-frac) + samples[idx+1]*frac
		}
	}
	return out
}

func degToRad(deg float64) float64 {
	return deg * math.Pi / 180.0
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

