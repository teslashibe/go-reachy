package audioio

import (
	"testing"
)

func TestResample_SameRate(t *testing.T) {
	samples := []int16{100, 200, 300, 400, 500}
	result := Resample(samples, 24000, 24000)

	if len(result) != len(samples) {
		t.Errorf("Expected %d samples, got %d", len(samples), len(result))
	}

	for i, s := range samples {
		if result[i] != s {
			t.Errorf("Sample %d: expected %d, got %d", i, s, result[i])
		}
	}
}

func TestResample_Downsample(t *testing.T) {
	// 48kHz -> 24kHz (2:1 ratio)
	samples := make([]int16, 960) // 20ms at 48kHz
	for i := range samples {
		samples[i] = int16(i)
	}

	result := Resample(samples, 48000, 24000)

	// Should get approximately half the samples
	expectedLen := 480
	if len(result) != expectedLen {
		t.Errorf("Expected %d samples, got %d", expectedLen, len(result))
	}
}

func TestResample_Upsample(t *testing.T) {
	// 16kHz -> 24kHz (2:3 ratio)
	samples := make([]int16, 320) // 20ms at 16kHz
	for i := range samples {
		samples[i] = int16(i * 100)
	}

	result := Resample(samples, 16000, 24000)

	// Should get 1.5x samples
	expectedLen := 480
	if len(result) != expectedLen {
		t.Errorf("Expected %d samples, got %d", expectedLen, len(result))
	}
}

func TestResample_Empty(t *testing.T) {
	result := Resample(nil, 24000, 48000)
	if len(result) != 0 {
		t.Errorf("Expected empty result for nil input")
	}

	result = Resample([]int16{}, 24000, 48000)
	if len(result) != 0 {
		t.Errorf("Expected empty result for empty input")
	}
}

func TestBytesToSamples(t *testing.T) {
	data := []byte{0x02, 0x01, 0x04, 0x03}
	samples := BytesToSamples(data)

	if len(samples) != 2 {
		t.Fatalf("Expected 2 samples, got %d", len(samples))
	}

	if samples[0] != 0x0102 {
		t.Errorf("Sample 0: expected 0x0102, got 0x%04x", samples[0])
	}

	if samples[1] != 0x0304 {
		t.Errorf("Sample 1: expected 0x0304, got 0x%04x", samples[1])
	}
}

func TestSamplesToBytes(t *testing.T) {
	samples := []int16{0x0102, 0x0304}
	data := SamplesToBytes(samples)

	if len(data) != 4 {
		t.Fatalf("Expected 4 bytes, got %d", len(data))
	}

	expected := []byte{0x02, 0x01, 0x04, 0x03}
	for i, b := range expected {
		if data[i] != b {
			t.Errorf("Byte %d: expected 0x%02x, got 0x%02x", i, b, data[i])
		}
	}
}

func TestMonoToStereo(t *testing.T) {
	mono := []int16{100, 200, 300}
	stereo := MonoToStereo(mono)

	if len(stereo) != 6 {
		t.Fatalf("Expected 6 samples, got %d", len(stereo))
	}

	expected := []int16{100, 100, 200, 200, 300, 300}
	for i, s := range expected {
		if stereo[i] != s {
			t.Errorf("Sample %d: expected %d, got %d", i, s, stereo[i])
		}
	}
}

func TestStereoToMono(t *testing.T) {
	stereo := []int16{100, 200, 300, 400}
	mono := StereoToMono(stereo)

	if len(mono) != 2 {
		t.Fatalf("Expected 2 samples, got %d", len(mono))
	}

	// (100+200)/2 = 150, (300+400)/2 = 350
	expected := []int16{150, 350}
	for i, s := range expected {
		if mono[i] != s {
			t.Errorf("Sample %d: expected %d, got %d", i, s, mono[i])
		}
	}
}

func TestNormalizeSamples(t *testing.T) {
	samples := []int16{100, -200, 150}
	result := NormalizeSamples(samples)

	// Peak is 200, so scale is 32767/200 = 163.835
	// Result should have max absolute value of ~32767

	var maxAbs int16
	for _, s := range result {
		if s > maxAbs {
			maxAbs = s
		}
		if -s > maxAbs {
			maxAbs = -s
		}
	}

	if maxAbs < 32700 || maxAbs > 32767 {
		t.Errorf("Expected max ~32767, got %d", maxAbs)
	}
}

func TestNormalizeSamples_Empty(t *testing.T) {
	result := NormalizeSamples(nil)
	if len(result) != 0 {
		t.Errorf("Expected empty result")
	}
}

func TestNormalizeSamples_Silence(t *testing.T) {
	samples := []int16{0, 0, 0}
	result := NormalizeSamples(samples)

	for i, s := range result {
		if s != 0 {
			t.Errorf("Sample %d: expected 0, got %d", i, s)
		}
	}
}

func TestCalculateRMS(t *testing.T) {
	// Silence
	rms := CalculateRMS([]int16{0, 0, 0})
	if rms != 0 {
		t.Errorf("Expected RMS 0 for silence, got %f", rms)
	}

	// Full scale
	samples := []int16{32767, 32767, 32767}
	rms = CalculateRMS(samples)
	if rms < 0.99 || rms > 1.01 {
		t.Errorf("Expected RMS ~1.0 for full scale, got %f", rms)
	}

	// Empty
	rms = CalculateRMS(nil)
	if rms != 0 {
		t.Errorf("Expected RMS 0 for empty, got %f", rms)
	}
}

func TestResampleBytes(t *testing.T) {
	// Create 20ms of 48kHz audio (960 samples)
	samples := make([]int16, 960)
	for i := range samples {
		samples[i] = int16(i % 1000)
	}
	data := SamplesToBytes(samples)

	// Resample to 24kHz
	result := ResampleBytes(data, 48000, 24000)

	// Should get ~480 samples = 960 bytes
	expectedBytes := 480 * 2
	if len(result) != expectedBytes {
		t.Errorf("Expected %d bytes, got %d", expectedBytes, len(result))
	}
}

// Benchmarks

func BenchmarkResample_2x(b *testing.B) {
	samples := make([]int16, 960)
	for i := range samples {
		samples[i] = int16(i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Resample(samples, 48000, 24000)
	}
}

func BenchmarkBytesToSamples(b *testing.B) {
	data := make([]byte, 960)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = BytesToSamples(data)
	}
}

func BenchmarkSamplesToBytes(b *testing.B) {
	samples := make([]int16, 480)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = SamplesToBytes(samples)
	}
}

