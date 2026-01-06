package zenohclient

import (
	"testing"
)

func TestAudioChunk_EncodeDecode(t *testing.T) {
	original := AudioChunk{
		SampleRate: 24000,
		Channels:   1,
		Samples:    []int16{100, -200, 300, -400, 32767, -32768},
	}

	encoded := original.Encode()

	// Check minimum size: 12 bytes header + samples
	expectedSize := 12 + len(original.Samples)*2
	if len(encoded) != expectedSize {
		t.Errorf("Expected %d bytes, got %d", expectedSize, len(encoded))
	}

	var decoded AudioChunk
	if err := decoded.Decode(encoded); err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	if decoded.SampleRate != original.SampleRate {
		t.Errorf("SampleRate: expected %d, got %d", original.SampleRate, decoded.SampleRate)
	}

	if decoded.Channels != original.Channels {
		t.Errorf("Channels: expected %d, got %d", original.Channels, decoded.Channels)
	}

	if len(decoded.Samples) != len(original.Samples) {
		t.Fatalf("Samples length: expected %d, got %d", len(original.Samples), len(decoded.Samples))
	}

	for i, s := range original.Samples {
		if decoded.Samples[i] != s {
			t.Errorf("Sample %d: expected %d, got %d", i, s, decoded.Samples[i])
		}
	}
}

func TestAudioChunk_DecodeErrors(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{"too_short", []byte{1, 2, 3}},
		{"header_only", []byte{0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0}}, // declares 1 sample but no data
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var chunk AudioChunk
			if err := chunk.Decode(tt.data); err == nil {
				t.Error("Expected decode error, got nil")
			}
		})
	}
}

func TestAudioChunk_Empty(t *testing.T) {
	original := AudioChunk{
		SampleRate: 24000,
		Channels:   1,
		Samples:    []int16{},
	}

	encoded := original.Encode()

	var decoded AudioChunk
	if err := decoded.Decode(encoded); err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	if len(decoded.Samples) != 0 {
		t.Errorf("Expected empty samples, got %d", len(decoded.Samples))
	}
}

func TestTopics(t *testing.T) {
	topics := NewTopics("reachy_mini")

	tests := []struct {
		method   func() string
		expected string
	}{
		{topics.Command, "reachy_mini/command"},
		{topics.JointPositions, "reachy_mini/joint_positions"},
		{topics.HeadPose, "reachy_mini/head_pose"},
		{topics.DaemonStatus, "reachy_mini/daemon_status"},
		{topics.Task, "reachy_mini/task"},
		{topics.TaskProgress, "reachy_mini/task_progress"},
		{topics.RecordedData, "reachy_mini/recorded_data"},
		{topics.AudioMic, "reachy_mini/audio/mic"},
		{topics.AudioSpeaker, "reachy_mini/audio/speaker"},
		{topics.AudioDOA, "reachy_mini/audio/doa"},
	}

	for _, tt := range tests {
		result := tt.method()
		if result != tt.expected {
			t.Errorf("Expected %s, got %s", tt.expected, result)
		}
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name      string
		cfg       Config
		shouldErr bool
	}{
		{
			name:      "valid",
			cfg:       DefaultConfig(),
			shouldErr: false,
		},
		{
			name: "empty_endpoint",
			cfg: Config{
				Endpoint: "",
				Mode:     "client",
				Prefix:   "test",
			},
			shouldErr: true,
		},
		{
			name: "invalid_mode",
			cfg: Config{
				Endpoint: "tcp/localhost:7447",
				Mode:     "invalid",
				Prefix:   "test",
			},
			shouldErr: true,
		},
		{
			name: "empty_prefix",
			cfg: Config{
				Endpoint: "tcp/localhost:7447",
				Mode:     "client",
				Prefix:   "",
			},
			shouldErr: true,
		},
		{
			name: "peer_mode",
			cfg: Config{
				Endpoint: "tcp/localhost:7447",
				Mode:     "peer",
				Prefix:   "test",
			},
			shouldErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.shouldErr && err == nil {
				t.Error("Expected error, got nil")
			}
			if !tt.shouldErr && err != nil {
				t.Errorf("Expected no error, got: %v", err)
			}
		})
	}
}

// Benchmarks

func BenchmarkAudioChunk_Encode(b *testing.B) {
	chunk := AudioChunk{
		SampleRate: 24000,
		Channels:   1,
		Samples:    make([]int16, 480), // 20ms
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = chunk.Encode()
	}
}

func BenchmarkAudioChunk_Decode(b *testing.B) {
	chunk := AudioChunk{
		SampleRate: 24000,
		Channels:   1,
		Samples:    make([]int16, 480),
	}
	data := chunk.Encode()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var decoded AudioChunk
		_ = decoded.Decode(data)
	}
}

