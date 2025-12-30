package realtime

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"
)

// AudioPlayer handles streaming audio playback to the robot
type AudioPlayer struct {
	robotIP string
	sshUser string
	sshPass string

	// Streaming state
	streamCmd   *exec.Cmd
	streamStdin io.WriteCloser
	streamMu    sync.Mutex
	streaming   bool

	// Callbacks
	OnPlaybackStart func()
	OnPlaybackEnd   func()

	// State
	speaking   bool
	speakingMu sync.Mutex
}

// NewAudioPlayer creates a new audio player for the robot
func NewAudioPlayer(robotIP, sshUser, sshPass string) *AudioPlayer {
	return &AudioPlayer{
		robotIP: robotIP,
		sshUser: sshUser,
		sshPass: sshPass,
	}
}

// AppendAudio streams audio data directly to the robot (base64 encoded PCM16 at 24kHz)
func (p *AudioPlayer) AppendAudio(base64Audio string) error {
	decoded, err := base64.StdEncoding.DecodeString(base64Audio)
	if err != nil {
		return err
	}

	p.streamMu.Lock()
	defer p.streamMu.Unlock()

	// Start streaming pipeline if not already running
	if !p.streaming {
		if err := p.startStream(); err != nil {
			return fmt.Errorf("start stream: %w", err)
		}
	}

	// Write audio data directly to the pipeline
	if p.streamStdin != nil {
		_, err = p.streamStdin.Write(decoded)
		if err != nil {
			// Pipeline died, try to restart
			p.stopStreamLocked()
			return fmt.Errorf("write to stream: %w", err)
		}
	}

	return nil
}

// startStream starts the GStreamer pipeline for streaming audio
func (p *AudioPlayer) startStream() error {
	// GStreamer pipeline that reads from stdin and plays audio
	// Using fdsrc to read raw PCM from stdin
	// Added queue for buffering and sync=true to ensure proper playback timing
	pipeline := `gst-launch-1.0 -q fdsrc fd=0 ! queue max-size-time=5000000000 ! rawaudioparse format=pcm pcm-format=s16le sample-rate=24000 num-channels=1 ! audioconvert ! audioresample ! audio/x-raw,rate=48000,channels=1,layout=interleaved ! queue ! opusenc frame-size=20 ! rtpopuspay pt=96 ! udpsink host=127.0.0.1 port=5000 sync=true`

	p.streamCmd = exec.Command("bash", "-c", fmt.Sprintf(
		`sshpass -p "%s" ssh -o StrictHostKeyChecking=no %s@%s '%s'`,
		p.sshPass, p.sshUser, p.robotIP, pipeline))

	var err error
	p.streamStdin, err = p.streamCmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}

	if err := p.streamCmd.Start(); err != nil {
		return fmt.Errorf("start cmd: %w", err)
	}

	p.streaming = true

	// Notify playback started
	if p.OnPlaybackStart != nil {
		p.speakingMu.Lock()
		p.speaking = true
		p.speakingMu.Unlock()
		p.OnPlaybackStart()
	}

	return nil
}

// FlushAndPlay signals end of audio stream and waits for playback to complete
func (p *AudioPlayer) FlushAndPlay() error {
	p.streamMu.Lock()
	defer p.streamMu.Unlock()

	if !p.streaming {
		return nil
	}

	// Give GStreamer a moment to process buffered data before closing stdin
	time.Sleep(100 * time.Millisecond)

	// Close stdin to signal EOF to GStreamer
	if p.streamStdin != nil {
		p.streamStdin.Close()
		p.streamStdin = nil
	}

	// Wait for playback to complete (with timeout)
	done := make(chan error, 1)
	go func() {
		if p.streamCmd != nil {
			done <- p.streamCmd.Wait()
		} else {
			done <- nil
		}
	}()

	select {
	case <-done:
		// Playback completed
	case <-time.After(30 * time.Second):
		// Timeout - kill the process
		if p.streamCmd != nil && p.streamCmd.Process != nil {
			p.streamCmd.Process.Kill()
		}
	}

	p.streaming = false
	p.streamCmd = nil

	// Notify playback ended
	if p.OnPlaybackEnd != nil {
		p.speakingMu.Lock()
		p.speaking = false
		p.speakingMu.Unlock()
		p.OnPlaybackEnd()
	}

	return nil
}

// stopStreamLocked stops the streaming pipeline (must hold streamMu)
func (p *AudioPlayer) stopStreamLocked() {
	if p.streamStdin != nil {
		p.streamStdin.Close()
		p.streamStdin = nil
	}
	if p.streamCmd != nil && p.streamCmd.Process != nil {
		p.streamCmd.Process.Kill()
		p.streamCmd.Wait()
	}
	p.streaming = false
	p.streamCmd = nil

	p.speakingMu.Lock()
	p.speaking = false
	p.speakingMu.Unlock()
}

// Cancel stops any current playback immediately
func (p *AudioPlayer) Cancel() {
	p.streamMu.Lock()
	defer p.streamMu.Unlock()
	p.stopStreamLocked()

	if p.OnPlaybackEnd != nil {
		p.OnPlaybackEnd()
	}
}

// Clear is deprecated - use Cancel instead
func (p *AudioPlayer) Clear() {
	p.Cancel()
}

// IsPlaying returns whether audio is currently playing
func (p *AudioPlayer) IsPlaying() bool {
	p.streamMu.Lock()
	defer p.streamMu.Unlock()
	return p.streaming
}

// IsSpeaking returns whether Eva is currently speaking
func (p *AudioPlayer) IsSpeaking() bool {
	p.speakingMu.Lock()
	defer p.speakingMu.Unlock()
	return p.speaking
}

// ConvertPCM16ToInt16 converts byte slice to int16 samples
func ConvertPCM16ToInt16(data []byte) []int16 {
	samples := make([]int16, len(data)/2)
	for i := 0; i < len(samples); i++ {
		samples[i] = int16(binary.LittleEndian.Uint16(data[i*2:]))
	}
	return samples
}

// ConvertInt16ToPCM16 converts int16 samples to byte slice
func ConvertInt16ToPCM16(samples []int16) []byte {
	data := make([]byte, len(samples)*2)
	for i, s := range samples {
		binary.LittleEndian.PutUint16(data[i*2:], uint16(s))
	}
	return data
}

// Resample resamples audio from srcRate to dstRate
func Resample(samples []int16, srcRate, dstRate int) []int16 {
	if srcRate == dstRate {
		return samples
	}

	ratio := float64(dstRate) / float64(srcRate)
	newLen := int(float64(len(samples)) * ratio)
	result := make([]int16, newLen)

	for i := 0; i < newLen; i++ {
		srcIdx := float64(i) / ratio
		idx := int(srcIdx)
		if idx >= len(samples)-1 {
			result[i] = samples[len(samples)-1]
		} else {
			frac := srcIdx - float64(idx)
			result[i] = int16(float64(samples[idx])*(1-frac) + float64(samples[idx+1])*frac)
		}
	}

	return result
}
