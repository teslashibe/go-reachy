package voice

import (
	"fmt"
	"sync"
	"time"
)

// Metrics tracks latency at each stage of the voice pipeline.
// Measures 5 stages: Capture → Send → Pipeline → Receive → Playback
//
// ┌─────────────────────────────────────────────────────────────────────────────┐
// │                           EVA LATENCY PIPELINE                               │
// ├─────────────┬─────────────┬─────────────┬─────────────┬─────────────────────┤
// │  CAPTURE    │    SEND     │   PIPELINE  │   RECEIVE   │     PLAYBACK        │
// │  WebRTC →   │  Buffer →   │  Provider   │  WS →       │  GStreamer →        │
// │  Buffer     │  WebSocket  │  Processing │  Callback   │  Robot Speaker      │
// └─────────────┴─────────────┴─────────────┴─────────────┴─────────────────────┘
type Metrics struct {
	// Stage 1: Audio Capture (WebRTC → Eva buffer)
	CaptureStartTime time.Time     // When WebRTC delivered audio
	CaptureEndTime   time.Time     // When buffered and ready to send
	CaptureLatency   time.Duration // Time to buffer incoming audio

	// Stage 2: Audio Send (Eva buffer → Pipeline WebSocket)
	SendStartTime time.Time     // When we started sending to pipeline
	SendEndTime   time.Time     // When WebSocket write completed
	SendLatency   time.Duration // Time to send to pipeline

	// Stage 3: Pipeline Processing (provider-internal VAD/ASR/LLM/TTS)
	PipelineStartTime time.Time     // Last audio sent (user stopped speaking)
	PipelineEndTime   time.Time     // First audio received (response started)
	PipelineLatency   time.Duration // Provider processing time (the big one!)

	// Stage 4: Audio Receive (Pipeline WebSocket → Eva callback)
	ReceiveStartTime time.Time     // When WebSocket received data
	ReceiveEndTime   time.Time     // When callback completed
	ReceiveLatency   time.Duration // Time to process incoming audio

	// Stage 5: Audio Playback (Eva → GStreamer → Robot speaker)
	PlaybackStartTime time.Time     // When we sent to GStreamer
	PlaybackEndTime   time.Time     // When audio started playing (estimated)
	PlaybackLatency   time.Duration // Time to start audio output

	// Response complete
	ResponseDoneTime time.Time     // When response fully delivered
	TotalLatency     time.Duration // End-to-end user experience

	// Counts for this conversation turn
	AudioChunksIn   int // Number of audio chunks sent to pipeline
	AudioChunksOut  int // Number of audio chunks received from pipeline
	TokensGenerated int // Number of LLM tokens (if available)
}

// MetricsCollector collects latency metrics during a conversation turn.
// It is goroutine-safe and can be used from multiple callbacks.
type MetricsCollector struct {
	mu      sync.Mutex
	current Metrics
	history []Metrics // Recent turns for averaging

	// Callbacks for metrics updates
	onUpdate func(Metrics)
}

// NewMetricsCollector creates a new metrics collector.
func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{
		history: make([]Metrics, 0, 100),
	}
}

// OnUpdate sets a callback that fires whenever metrics are updated.
func (m *MetricsCollector) OnUpdate(fn func(Metrics)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onUpdate = fn
}

// Reset clears all metrics for a new conversation turn.
func (m *MetricsCollector) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.current = Metrics{}
}

// --- Stage 1: Capture (WebRTC → Eva buffer) ---

// MarkCaptureStart records when WebRTC delivered audio to Eva.
func (m *MetricsCollector) MarkCaptureStart() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.current.CaptureStartTime = time.Now()
}

// MarkCaptureEnd records when audio is buffered and ready to send.
func (m *MetricsCollector) MarkCaptureEnd() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.current.CaptureEndTime = time.Now()
	if !m.current.CaptureStartTime.IsZero() {
		m.current.CaptureLatency = m.current.CaptureEndTime.Sub(m.current.CaptureStartTime)
	}
}

// --- Stage 2: Send (Eva buffer → Pipeline WebSocket) ---

// MarkSendStart records when we started sending audio to the pipeline.
func (m *MetricsCollector) MarkSendStart() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.current.SendStartTime = time.Now()
}

// MarkSendEnd records when WebSocket write completed.
func (m *MetricsCollector) MarkSendEnd() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.current.SendEndTime = time.Now()
	if !m.current.SendStartTime.IsZero() {
		m.current.SendLatency = m.current.SendEndTime.Sub(m.current.SendStartTime)
	}
	// Also update pipeline start time (last audio sent = when user stopped speaking)
	m.current.PipelineStartTime = m.current.SendEndTime
}

// --- Stage 3: Pipeline Processing (provider-internal) ---

// MarkPipelineStart records when we last sent audio (user stopped speaking).
// This is typically called automatically by MarkSendEnd.
func (m *MetricsCollector) MarkPipelineStart() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.current.PipelineStartTime = time.Now()
}

// MarkPipelineEnd records when we received first audio from the pipeline.
func (m *MetricsCollector) MarkPipelineEnd() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.current.PipelineEndTime.IsZero() {
		m.current.PipelineEndTime = time.Now()
		if !m.current.PipelineStartTime.IsZero() {
			m.current.PipelineLatency = m.current.PipelineEndTime.Sub(m.current.PipelineStartTime)
		}
		m.notify()
	}
}

// --- Stage 4: Receive (Pipeline WebSocket → Eva callback) ---

// MarkReceiveStart records when WebSocket received audio data.
func (m *MetricsCollector) MarkReceiveStart() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.current.ReceiveStartTime.IsZero() {
		m.current.ReceiveStartTime = time.Now()
	}
}

// MarkReceiveEnd records when we finished processing received audio.
func (m *MetricsCollector) MarkReceiveEnd() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.current.ReceiveEndTime = time.Now()
	if !m.current.ReceiveStartTime.IsZero() {
		m.current.ReceiveLatency = m.current.ReceiveEndTime.Sub(m.current.ReceiveStartTime)
	}
}

// --- Stage 5: Playback (Eva → GStreamer → Robot) ---

// MarkPlaybackStart records when we sent audio to GStreamer.
func (m *MetricsCollector) MarkPlaybackStart() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.current.PlaybackStartTime.IsZero() {
		m.current.PlaybackStartTime = time.Now()
	}
}

// MarkPlaybackEnd records when audio started playing (or estimated).
func (m *MetricsCollector) MarkPlaybackEnd() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.current.PlaybackEndTime = time.Now()
	if !m.current.PlaybackStartTime.IsZero() {
		m.current.PlaybackLatency = m.current.PlaybackEndTime.Sub(m.current.PlaybackStartTime)
	}
}

// --- Response Complete ---

// MarkResponseDone records when the response is fully delivered.
func (m *MetricsCollector) MarkResponseDone() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.current.ResponseDoneTime = time.Now()
	if !m.current.PipelineStartTime.IsZero() {
		m.current.TotalLatency = m.current.ResponseDoneTime.Sub(m.current.PipelineStartTime)
	}
	// Archive this turn
	m.history = append(m.history, m.current)
	if len(m.history) > 100 {
		m.history = m.history[1:]
	}
	m.notify()
}

// --- Legacy methods for backward compatibility ---

// MarkSpeechEnd is an alias for MarkPipelineStart (legacy compatibility).
func (m *MetricsCollector) MarkSpeechEnd() {
	m.MarkPipelineStart()
}

// MarkFirstAudio is an alias for MarkPipelineEnd (legacy compatibility).
func (m *MetricsCollector) MarkFirstAudio() {
	m.MarkPipelineEnd()
}

// MarkTranscript records transcript time (for pipelines that provide it).
func (m *MetricsCollector) MarkTranscript() {
	// No-op for now, could be extended for modular pipelines
}

// MarkFirstToken records first LLM token (for pipelines that provide it).
func (m *MetricsCollector) MarkFirstToken() {
	// No-op for now, could be extended for modular pipelines
}

// IncrementAudioIn increments the count of audio chunks received.
func (m *MetricsCollector) IncrementAudioIn() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.current.AudioChunksIn++
}

// AudioInChunks returns the current count of audio chunks received.
func (m *MetricsCollector) AudioInChunks() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.current.AudioChunksIn
}

// IncrementAudioOut increments the count of audio chunks sent.
func (m *MetricsCollector) IncrementAudioOut() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.current.AudioChunksOut++
}

// Current returns the current metrics snapshot.
func (m *MetricsCollector) Current() Metrics {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.current
}

// Average returns average metrics over recent turns.
func (m *MetricsCollector) Average() Metrics {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.history) == 0 {
		return Metrics{}
	}

	var avg Metrics
	for _, h := range m.history {
		avg.CaptureLatency += h.CaptureLatency
		avg.SendLatency += h.SendLatency
		avg.PipelineLatency += h.PipelineLatency
		avg.ReceiveLatency += h.ReceiveLatency
		avg.PlaybackLatency += h.PlaybackLatency
		avg.TotalLatency += h.TotalLatency
	}

	n := time.Duration(len(m.history))
	avg.CaptureLatency /= n
	avg.SendLatency /= n
	avg.PipelineLatency /= n
	avg.ReceiveLatency /= n
	avg.PlaybackLatency /= n
	avg.TotalLatency /= n

	return avg
}

// notify calls the update callback if set.
// Must be called with mutex held.
func (m *MetricsCollector) notify() {
	if m.onUpdate != nil {
		// Copy to avoid races
		metrics := m.current
		go m.onUpdate(metrics)
	}
}

// FormatLatency returns a formatted string of current latencies.
// Shows all 5 stages: CAPTURE | SEND | PIPELINE | RECEIVE | PLAYBACK | TOTAL
func (m *Metrics) FormatLatency() string {
	return fmt.Sprintf("CAP:%s | SEND:%s | PIPE:%s | RECV:%s | PLAY:%s | TOTAL:%s",
		formatDurationShort(m.CaptureLatency),
		formatDurationShort(m.SendLatency),
		formatDurationShort(m.PipelineLatency),
		formatDurationShort(m.ReceiveLatency),
		formatDurationShort(m.PlaybackLatency),
		formatDurationShort(m.TotalLatency))
}

// FormatLatencyCompact returns a shorter format for display.
func (m *Metrics) FormatLatencyCompact() string {
	if m.PipelineLatency == 0 {
		return "---"
	}
	return fmt.Sprintf("PIPELINE:%s | TOTAL:%s",
		formatDurationShort(m.PipelineLatency),
		formatDurationShort(m.TotalLatency))
}

func formatDurationShort(d time.Duration) string {
	if d == 0 {
		return "---"
	}
	ms := d.Milliseconds()
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	return fmt.Sprintf("%.1fs", float64(ms)/1000)
}

func formatDuration(d time.Duration) string {
	if d == 0 {
		return "---ms"
	}
	return d.Round(time.Millisecond).String()
}
