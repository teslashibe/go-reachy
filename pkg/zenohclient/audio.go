package zenohclient

import (
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	zenoh "github.com/teslashibe/zenoh-go"
)

// AudioChunk represents an audio chunk for Zenoh transmission.
// Wire format: [4 bytes sample_rate][4 bytes channels][4 bytes length][samples...]
type AudioChunk struct {
	SampleRate int
	Channels   int
	Samples    []int16
	Timestamp  time.Time
}

// Encode serializes the audio chunk for transmission.
func (c *AudioChunk) Encode() []byte {
	// Header: sample_rate (4) + channels (4) + length (4) = 12 bytes
	// Data: samples * 2 bytes
	buf := make([]byte, 12+len(c.Samples)*2)

	binary.LittleEndian.PutUint32(buf[0:4], uint32(c.SampleRate))
	binary.LittleEndian.PutUint32(buf[4:8], uint32(c.Channels))
	binary.LittleEndian.PutUint32(buf[8:12], uint32(len(c.Samples)))

	for i, s := range c.Samples {
		binary.LittleEndian.PutUint16(buf[12+i*2:14+i*2], uint16(s))
	}

	return buf
}

// Decode deserializes an audio chunk from wire format.
func (c *AudioChunk) Decode(data []byte) error {
	if len(data) < 12 {
		return fmt.Errorf("data too short: %d bytes", len(data))
	}

	c.SampleRate = int(binary.LittleEndian.Uint32(data[0:4]))
	c.Channels = int(binary.LittleEndian.Uint32(data[4:8]))
	length := int(binary.LittleEndian.Uint32(data[8:12]))

	expectedLen := 12 + length*2
	if len(data) < expectedLen {
		return fmt.Errorf("data too short for declared length: got %d, need %d", len(data), expectedLen)
	}

	c.Samples = make([]int16, length)
	for i := range c.Samples {
		c.Samples[i] = int16(binary.LittleEndian.Uint16(data[12+i*2 : 14+i*2]))
	}

	c.Timestamp = time.Now()
	return nil
}

// AudioPublisher publishes audio chunks to a Zenoh topic.
type AudioPublisher struct {
	client *Client
	topic  string
	logger *slog.Logger

	mu        sync.Mutex
	publisher zenoh.Publisher
	closed    bool

	// Stats
	chunksSent atomic.Int64
	bytesSent  atomic.Int64
}

// NewAudioPublisher creates a new audio publisher.
func NewAudioPublisher(client *Client, topic string, logger *slog.Logger) (*AudioPublisher, error) {
	if client == nil || !client.IsConnected() {
		return nil, fmt.Errorf("client not connected")
	}

	if logger == nil {
		logger = slog.Default()
	}

	session := client.Session()
	if session == nil {
		return nil, fmt.Errorf("session not available")
	}

	pub, err := session.Publisher(zenoh.KeyExpr(topic))
	if err != nil {
		return nil, fmt.Errorf("failed to create publisher: %w", err)
	}

	logger.Info("audio publisher created", "topic", topic)

	return &AudioPublisher{
		client:    client,
		topic:     topic,
		logger:    logger,
		publisher: pub,
	}, nil
}

// Publish sends an audio chunk.
func (p *AudioPublisher) Publish(chunk AudioChunk) error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return fmt.Errorf("publisher closed")
	}
	pub := p.publisher
	p.mu.Unlock()

	data := chunk.Encode()

	if err := pub.Put(data); err != nil {
		return fmt.Errorf("failed to publish audio: %w", err)
	}

	p.chunksSent.Add(1)
	p.bytesSent.Add(int64(len(data)))

	return nil
}

// PublishRaw sends raw PCM16 bytes with metadata.
func (p *AudioPublisher) PublishRaw(samples []int16, sampleRate, channels int) error {
	return p.Publish(AudioChunk{
		SampleRate: sampleRate,
		Channels:   channels,
		Samples:    samples,
		Timestamp:  time.Now(),
	})
}

// Close closes the publisher.
func (p *AudioPublisher) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}
	p.closed = true

	if p.publisher != nil {
		return p.publisher.Close()
	}
	return nil
}

// Stats returns publisher statistics.
func (p *AudioPublisher) Stats() AudioPublisherStats {
	return AudioPublisherStats{
		ChunksSent: p.chunksSent.Load(),
		BytesSent:  p.bytesSent.Load(),
	}
}

// AudioPublisherStats contains publisher statistics.
type AudioPublisherStats struct {
	ChunksSent int64 `json:"chunks_sent"`
	BytesSent  int64 `json:"bytes_sent"`
}

// AudioSubscriber subscribes to audio chunks from a Zenoh topic.
type AudioSubscriber struct {
	client *Client
	topic  string
	logger *slog.Logger

	mu         sync.Mutex
	subscriber zenoh.Subscriber
	closed     bool
	chunkCh    chan AudioChunk

	// Stats
	chunksReceived atomic.Int64
	bytesReceived  atomic.Int64
	decodeErrors   atomic.Int64
}

// NewAudioSubscriber creates a new audio subscriber.
func NewAudioSubscriber(client *Client, topic string, bufferSize int, logger *slog.Logger) (*AudioSubscriber, error) {
	if client == nil || !client.IsConnected() {
		return nil, fmt.Errorf("client not connected")
	}

	if logger == nil {
		logger = slog.Default()
	}

	if bufferSize <= 0 {
		bufferSize = 50 // ~1 second of 20ms chunks
	}

	s := &AudioSubscriber{
		client:  client,
		topic:   topic,
		logger:  logger,
		chunkCh: make(chan AudioChunk, bufferSize),
	}

	session := client.Session()
	if session == nil {
		return nil, fmt.Errorf("session not available")
	}

	sub, err := session.Subscribe(zenoh.KeyExpr(topic), func(sample zenoh.Sample) {
		s.bytesReceived.Add(int64(len(sample.Payload)))

		var chunk AudioChunk
		if err := chunk.Decode(sample.Payload); err != nil {
			s.decodeErrors.Add(1)
			s.logger.Debug("failed to decode audio chunk", "error", err)
			return
		}

		s.chunksReceived.Add(1)

		select {
		case s.chunkCh <- chunk:
		default:
			// Buffer full, drop oldest
			select {
			case <-s.chunkCh:
				s.chunkCh <- chunk
			default:
			}
		}
	})

	if err != nil {
		return nil, fmt.Errorf("failed to subscribe: %w", err)
	}

	s.subscriber = sub
	logger.Info("audio subscriber created", "topic", topic, "buffer_size", bufferSize)

	return s, nil
}

// Stream returns a channel that receives audio chunks.
func (s *AudioSubscriber) Stream() <-chan AudioChunk {
	return s.chunkCh
}

// Read reads the next audio chunk, blocking if necessary.
func (s *AudioSubscriber) Read(ctx context.Context) (AudioChunk, error) {
	select {
	case <-ctx.Done():
		return AudioChunk{}, ctx.Err()
	case chunk, ok := <-s.chunkCh:
		if !ok {
			return AudioChunk{}, fmt.Errorf("subscriber closed")
		}
		return chunk, nil
	}
}

// Close closes the subscriber.
func (s *AudioSubscriber) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}
	s.closed = true

	close(s.chunkCh)

	if s.subscriber != nil {
		return s.subscriber.Close()
	}
	return nil
}

// Stats returns subscriber statistics.
func (s *AudioSubscriber) Stats() AudioSubscriberStats {
	return AudioSubscriberStats{
		ChunksReceived: s.chunksReceived.Load(),
		BytesReceived:  s.bytesReceived.Load(),
		DecodeErrors:   s.decodeErrors.Load(),
	}
}

// AudioSubscriberStats contains subscriber statistics.
type AudioSubscriberStats struct {
	ChunksReceived int64 `json:"chunks_received"`
	BytesReceived  int64 `json:"bytes_received"`
	DecodeErrors   int64 `json:"decode_errors"`
}

