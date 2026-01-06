package audioio

// Resample converts audio from one sample rate to another using linear interpolation.
// This is a simple resampler suitable for speech audio.
// For higher quality, consider using a polyphase filter.
func Resample(samples []int16, fromRate, toRate int) []int16 {
	if fromRate == toRate {
		return samples
	}

	if len(samples) == 0 {
		return samples
	}

	ratio := float64(fromRate) / float64(toRate)
	newLen := int(float64(len(samples)) / ratio)

	if newLen == 0 {
		return []int16{}
	}

	result := make([]int16, newLen)

	for i := 0; i < newLen; i++ {
		srcPos := float64(i) * ratio
		srcIdx := int(srcPos)
		frac := srcPos - float64(srcIdx)

		if srcIdx >= len(samples)-1 {
			result[i] = samples[len(samples)-1]
		} else {
			// Linear interpolation
			s1 := float64(samples[srcIdx])
			s2 := float64(samples[srcIdx+1])
			result[i] = int16(s1 + frac*(s2-s1))
		}
	}

	return result
}

// ResampleBytes resamples raw PCM16 bytes.
func ResampleBytes(data []byte, fromRate, toRate int) []byte {
	samples := BytesToSamples(data)
	resampled := Resample(samples, fromRate, toRate)
	return SamplesToBytes(resampled)
}

// BytesToSamples converts raw PCM16 little-endian bytes to int16 samples.
func BytesToSamples(data []byte) []int16 {
	samples := make([]int16, len(data)/2)
	for i := range samples {
		samples[i] = int16(data[i*2]) | int16(data[i*2+1])<<8
	}
	return samples
}

// SamplesToBytes converts int16 samples to raw PCM16 little-endian bytes.
func SamplesToBytes(samples []int16) []byte {
	data := make([]byte, len(samples)*2)
	for i, s := range samples {
		data[i*2] = byte(s)
		data[i*2+1] = byte(s >> 8)
	}
	return data
}

// MonoToStereo duplicates mono samples to stereo.
func MonoToStereo(samples []int16) []int16 {
	stereo := make([]int16, len(samples)*2)
	for i, s := range samples {
		stereo[i*2] = s
		stereo[i*2+1] = s
	}
	return stereo
}

// StereoToMono averages stereo samples to mono.
func StereoToMono(samples []int16) []int16 {
	mono := make([]int16, len(samples)/2)
	for i := range mono {
		left := int32(samples[i*2])
		right := int32(samples[i*2+1])
		mono[i] = int16((left + right) / 2)
	}
	return mono
}

// NormalizeSamples scales samples to use the full dynamic range.
func NormalizeSamples(samples []int16) []int16 {
	if len(samples) == 0 {
		return samples
	}

	// Find peak
	var peak int16
	for _, s := range samples {
		if s > peak {
			peak = s
		}
		if -s > peak {
			peak = -s
		}
	}

	if peak == 0 {
		return samples
	}

	// Scale factor to reach max amplitude
	scale := float64(32767) / float64(peak)

	result := make([]int16, len(samples))
	for i, s := range samples {
		result[i] = int16(float64(s) * scale)
	}

	return result
}

// CalculateRMS calculates the root mean square of samples.
// Returns a value between 0.0 and 1.0.
func CalculateRMS(samples []int16) float64 {
	if len(samples) == 0 {
		return 0
	}

	var sum float64
	for _, s := range samples {
		sum += float64(s) * float64(s)
	}

	rms := sum / float64(len(samples))
	// Normalize to 0-1 range (32767^2 = max possible)
	return rms / (32767 * 32767)
}

