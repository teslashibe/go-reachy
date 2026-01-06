package zenohclient

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
)

// AudioBridge connects local audio I/O to Zenoh topics.
// It captures from a local microphone and publishes to Zenoh,
// and subscribes from Zenoh to play on a local speaker.
type AudioBridge struct {
	client *Client
	logger *slog.Logger

	micTopic     string
	speakerTopic string

	mu       sync.Mutex
	running  bool
	stopCh   chan struct{}
	micPub   *AudioPublisher
	spkSub   *AudioSubscriber

	// Callbacks
	onMicChunk     func(chunk AudioChunk)     // Called for each mic chunk before publishing
	onSpeakerChunk func(chunk AudioChunk)     // Called for each speaker chunk before playing

	// Stats
	micChunks     atomic.Int64
	speakerChunks atomic.Int64
}

// AudioBridgeConfig configures the AudioBridge.
type AudioBridgeConfig struct {
	// MicTopic is the Zenoh topic for microphone audio.
	// Default: "{prefix}/audio/mic"
	MicTopic string

	// SpeakerTopic is the Zenoh topic for speaker audio.
	// Default: "{prefix}/audio/speaker"
	SpeakerTopic string

	// SpeakerBufferSize is the number of chunks to buffer for playback.
	// Default: 50 (~1 second at 20ms chunks)
	SpeakerBufferSize int

	// OnMicChunk is called for each mic chunk before publishing.
	// Can be used for VAD, visualization, etc.
	OnMicChunk func(chunk AudioChunk)

	// OnSpeakerChunk is called for each speaker chunk before playing.
	// Can be used for visualization, level monitoring, etc.
	OnSpeakerChunk func(chunk AudioChunk)
}

// DefaultAudioBridgeConfig returns sensible defaults.
func DefaultAudioBridgeConfig(client *Client) AudioBridgeConfig {
	return AudioBridgeConfig{
		MicTopic:          client.Topics().AudioMic(),
		SpeakerTopic:      client.Topics().AudioSpeaker(),
		SpeakerBufferSize: 50,
	}
}

// NewAudioBridge creates a new audio bridge.
func NewAudioBridge(client *Client, cfg AudioBridgeConfig, logger *slog.Logger) (*AudioBridge, error) {
	if logger == nil {
		logger = slog.Default()
	}

	if cfg.MicTopic == "" {
		cfg.MicTopic = client.Topics().AudioMic()
	}
	if cfg.SpeakerTopic == "" {
		cfg.SpeakerTopic = client.Topics().AudioSpeaker()
	}
	if cfg.SpeakerBufferSize <= 0 {
		cfg.SpeakerBufferSize = 50
	}

	return &AudioBridge{
		client:         client,
		logger:         logger,
		micTopic:       cfg.MicTopic,
		speakerTopic:   cfg.SpeakerTopic,
		onMicChunk:     cfg.OnMicChunk,
		onSpeakerChunk: cfg.OnSpeakerChunk,
	}, nil
}

// StartMicPublisher starts publishing microphone audio to Zenoh.
// Returns a channel to send audio chunks to.
func (b *AudioBridge) StartMicPublisher(ctx context.Context) (chan<- AudioChunk, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.micPub != nil {
		return nil, nil // Already running
	}

	pub, err := NewAudioPublisher(b.client, b.micTopic, b.logger)
	if err != nil {
		return nil, err
	}

	b.micPub = pub
	inputCh := make(chan AudioChunk, 10)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case chunk, ok := <-inputCh:
				if !ok {
					return
				}

				if b.onMicChunk != nil {
					b.onMicChunk(chunk)
				}

				if err := pub.Publish(chunk); err != nil {
					b.logger.Debug("mic publish error", "error", err)
					continue
				}

				b.micChunks.Add(1)
			}
		}
	}()

	b.logger.Info("mic publisher started", "topic", b.micTopic)

	return inputCh, nil
}

// StartSpeakerSubscriber starts subscribing to speaker audio from Zenoh.
// Returns a channel that receives audio chunks.
func (b *AudioBridge) StartSpeakerSubscriber(ctx context.Context) (<-chan AudioChunk, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.spkSub != nil {
		return b.spkSub.Stream(), nil // Already running
	}

	sub, err := NewAudioSubscriber(b.client, b.speakerTopic, 50, b.logger)
	if err != nil {
		return nil, err
	}

	b.spkSub = sub
	outputCh := make(chan AudioChunk, 10)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case chunk, ok := <-sub.Stream():
				if !ok {
					close(outputCh)
					return
				}

				if b.onSpeakerChunk != nil {
					b.onSpeakerChunk(chunk)
				}

				b.speakerChunks.Add(1)

				select {
				case outputCh <- chunk:
				default:
					// Drop if buffer full
				}
			}
		}
	}()

	b.logger.Info("speaker subscriber started", "topic", b.speakerTopic)

	return outputCh, nil
}

// Close closes the audio bridge.
func (b *AudioBridge) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	var errs []error

	if b.micPub != nil {
		if err := b.micPub.Close(); err != nil {
			errs = append(errs, err)
		}
		b.micPub = nil
	}

	if b.spkSub != nil {
		if err := b.spkSub.Close(); err != nil {
			errs = append(errs, err)
		}
		b.spkSub = nil
	}

	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

// Stats returns bridge statistics.
func (b *AudioBridge) Stats() AudioBridgeStats {
	return AudioBridgeStats{
		MicChunksSent:       b.micChunks.Load(),
		SpeakerChunksRecv:   b.speakerChunks.Load(),
	}
}

// AudioBridgeStats contains bridge statistics.
type AudioBridgeStats struct {
	MicChunksSent     int64 `json:"mic_chunks_sent"`
	SpeakerChunksRecv int64 `json:"speaker_chunks_recv"`
}

