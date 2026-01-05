package voice

import (
	"sync"
	"time"
)

// Metrics tracks latency at each stage of the voice pipeline.
// All durations are measured from the moment speech ends (user stops talking).
type Metrics struct {
	// Timestamps for key events
	SpeechEndTime   time.Time // When VAD detected end of speech
	TranscriptTime  time.Time // When ASR completed transcription
	FirstTokenTime  time.Time // When LLM generated first token
	FirstAudioTime  time.Time // When TTS generated first audio chunk
	ResponseDoneTime time.Time // When response fully delivered

	// Computed latencies (from speech end)
	VADLatency     time.Duration // Time to detect speech end
	ASRLatency     time.Duration // Time to complete transcription
	LLMFirstToken  time.Duration // Time to first LLM token
	TTSFirstAudio  time.Duration // Time to first audio chunk
	TotalLatency   time.Duration // Total end-to-end latency

	// Counts for this conversation turn
	AudioChunksIn  int // Number of audio chunks received
	AudioChunksOut int // Number of audio chunks sent
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

// MarkSpeechEnd records when the user stopped speaking.
// This is the reference point for all latency measurements.
func (m *MetricsCollector) MarkSpeechEnd() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.current = Metrics{} // Reset for new turn
	m.current.SpeechEndTime = time.Now()
}

// MarkTranscript records when transcription completed.
func (m *MetricsCollector) MarkTranscript() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.current.TranscriptTime = time.Now()
	if !m.current.SpeechEndTime.IsZero() {
		m.current.ASRLatency = m.current.TranscriptTime.Sub(m.current.SpeechEndTime)
	}
	m.notify()
}

// MarkFirstToken records when the LLM generated its first token.
func (m *MetricsCollector) MarkFirstToken() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.current.FirstTokenTime.IsZero() {
		m.current.FirstTokenTime = time.Now()
		if !m.current.SpeechEndTime.IsZero() {
			m.current.LLMFirstToken = m.current.FirstTokenTime.Sub(m.current.SpeechEndTime)
		}
		m.notify()
	}
}

// MarkFirstAudio records when the first audio chunk was generated.
func (m *MetricsCollector) MarkFirstAudio() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.current.FirstAudioTime.IsZero() {
		m.current.FirstAudioTime = time.Now()
		if !m.current.SpeechEndTime.IsZero() {
			m.current.TTSFirstAudio = m.current.FirstAudioTime.Sub(m.current.SpeechEndTime)
		}
		m.notify()
	}
}

// MarkResponseDone records when the response is fully delivered.
func (m *MetricsCollector) MarkResponseDone() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.current.ResponseDoneTime = time.Now()
	if !m.current.SpeechEndTime.IsZero() {
		m.current.TotalLatency = m.current.ResponseDoneTime.Sub(m.current.SpeechEndTime)
	}
	// Archive this turn
	m.history = append(m.history, m.current)
	if len(m.history) > 100 {
		m.history = m.history[1:]
	}
	m.notify()
}

// IncrementAudioIn increments the count of audio chunks received.
func (m *MetricsCollector) IncrementAudioIn() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.current.AudioChunksIn++
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
		avg.VADLatency += h.VADLatency
		avg.ASRLatency += h.ASRLatency
		avg.LLMFirstToken += h.LLMFirstToken
		avg.TTSFirstAudio += h.TTSFirstAudio
		avg.TotalLatency += h.TotalLatency
	}

	n := time.Duration(len(m.history))
	avg.VADLatency /= n
	avg.ASRLatency /= n
	avg.LLMFirstToken /= n
	avg.TTSFirstAudio /= n
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
func (m *Metrics) FormatLatency() string {
	return formatDuration(m.VADLatency) + " VAD | " +
		formatDuration(m.ASRLatency) + " ASR | " +
		formatDuration(m.LLMFirstToken) + " LLM | " +
		formatDuration(m.TTSFirstAudio) + " TTS | " +
		formatDuration(m.TotalLatency) + " TOTAL"
}

func formatDuration(d time.Duration) string {
	if d == 0 {
		return "---ms"
	}
	return d.Round(time.Millisecond).String()
}

