package emotions

import (
	"context"
	"sort"
	"sync"
	"time"
)

// Player handles emotion playback with keyframe interpolation.
type Player struct {
	mu       sync.RWMutex
	state    PlaybackState
	emotion  *Emotion
	opts     PlayerOptions
	startAt  time.Time
	pausedAt time.Duration
	stopCh   chan struct{}
}

// NewPlayer creates a new emotion player.
func NewPlayer() *Player {
	return &Player{
		state:  StateStopped,
		opts:   DefaultPlayerOptions(),
		stopCh: make(chan struct{}),
	}
}

// Play starts playback of an emotion.
// The callback is called for each interpolated frame at the configured framerate.
// Blocks until playback completes or is stopped.
func (p *Player) Play(ctx context.Context, emotion *Emotion, callback PlayerCallback) error {
	return p.PlayWithOptions(ctx, emotion, callback, DefaultPlayerOptions())
}

// PlayWithOptions starts playback with custom options.
func (p *Player) PlayWithOptions(ctx context.Context, emotion *Emotion, callback PlayerCallback, opts PlayerOptions) error {
	p.mu.Lock()
	if p.state == StatePlaying {
		p.mu.Unlock()
		return ErrAlreadyPlaying
	}

	p.emotion = emotion
	p.opts = opts
	p.state = StatePlaying
	p.startAt = time.Now()
	p.pausedAt = 0
	p.stopCh = make(chan struct{})
	p.mu.Unlock()

	defer func() {
		p.mu.Lock()
		p.state = StateStopped
		p.mu.Unlock()
	}()

	frameDuration := time.Duration(float64(time.Second) / opts.FrameRate)
	ticker := time.NewTicker(frameDuration)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-p.stopCh:
			return nil

		case <-ticker.C:
			p.mu.RLock()
			if p.state == StatePaused {
				p.mu.RUnlock()
				continue
			}
			p.mu.RUnlock()

			elapsed := time.Since(p.startAt)
			if p.pausedAt > 0 {
				elapsed = p.pausedAt + time.Since(p.startAt)
			}

			// Apply speed multiplier
			elapsed = time.Duration(float64(elapsed) * opts.Speed)

			// Check if we've reached the end
			if elapsed >= emotion.Duration {
				if opts.Loop {
					// Reset for next loop
					p.mu.Lock()
					p.startAt = time.Now()
					p.pausedAt = 0
					p.mu.Unlock()
					elapsed = 0
				} else {
					// Play final frame and exit
					pose := p.evaluateAt(emotion, emotion.Duration-time.Millisecond)
					callback(pose, emotion.Duration)
					return nil
				}
			}

			pose := p.evaluateAt(emotion, elapsed)
			if !callback(pose, elapsed) {
				return nil
			}
		}
	}
}

// evaluateAt returns the interpolated pose at a given time.
func (p *Player) evaluateAt(emotion *Emotion, elapsed time.Duration) Pose {
	t := elapsed.Seconds()

	// Handle edge cases
	if len(emotion.Keyframes) == 0 {
		return Pose{}
	}
	if len(emotion.Keyframes) == 1 {
		return KeyframeToPose(emotion.Keyframes[0])
	}

	// Find the keyframe interval using binary search
	timestamps := emotion.Timestamps
	idx := sort.Search(len(timestamps), func(i int) bool {
		return timestamps[i] > t
	})

	// Clamp to valid range
	if idx == 0 {
		return KeyframeToPose(emotion.Keyframes[0])
	}
	if idx >= len(timestamps) {
		return KeyframeToPose(emotion.Keyframes[len(emotion.Keyframes)-1])
	}

	// Get surrounding keyframes
	idxPrev := idx - 1
	idxNext := idx

	tPrev := timestamps[idxPrev]
	tNext := timestamps[idxNext]

	// Calculate interpolation factor
	var alpha float64
	if tNext == tPrev {
		alpha = 0
	} else {
		alpha = (t - tPrev) / (tNext - tPrev)
	}

	// Interpolate keyframes
	kfInterp := InterpolateKeyframes(emotion.Keyframes[idxPrev], emotion.Keyframes[idxNext], alpha)

	return KeyframeToPose(kfInterp)
}

// Stop halts playback immediately.
func (p *Player) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.state == StatePlaying || p.state == StatePaused {
		close(p.stopCh)
		p.state = StateStopped
	}
}

// Pause temporarily stops playback.
func (p *Player) Pause() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.state == StatePlaying {
		p.pausedAt = time.Since(p.startAt)
		p.state = StatePaused
	}
}

// Resume continues paused playback.
func (p *Player) Resume() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.state == StatePaused {
		p.startAt = time.Now()
		p.state = StatePlaying
	}
}

// State returns the current playback state.
func (p *Player) State() PlaybackState {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.state
}

// CurrentEmotion returns the currently playing emotion, if any.
func (p *Player) CurrentEmotion() *Emotion {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.emotion
}

// Elapsed returns how much time has passed in the current playback.
func (p *Player) Elapsed() time.Duration {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.state == StateStopped {
		return 0
	}
	if p.state == StatePaused {
		return p.pausedAt
	}
	return time.Since(p.startAt)
}



