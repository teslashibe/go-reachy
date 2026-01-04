package audio

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"sync"
	"time"
)

// Player handles streaming audio playback to the robot.
type Player struct {
	robotIP   string
	sshUser   string
	sshPass   string
	openaiKey string // For TTS

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

// NewPlayer creates a new audio player for the robot.
func NewPlayer(robotIP, sshUser, sshPass string) *Player {
	return &Player{
		robotIP: robotIP,
		sshUser: sshUser,
		sshPass: sshPass,
	}
}

// SetOpenAIKey sets the OpenAI API key for TTS.
func (p *Player) SetOpenAIKey(key string) {
	p.openaiKey = key
}

// AppendAudio streams audio data directly to the robot (base64 encoded PCM16 at 24kHz).
func (p *Player) AppendAudio(base64Audio string) error {
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

// startStream starts the GStreamer pipeline for streaming audio.
func (p *Player) startStream() error {
	// GStreamer pipeline that reads from stdin and plays audio
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

// FlushAndPlay signals end of audio stream and waits for playback to complete.
func (p *Player) FlushAndPlay() error {
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

// stopStreamLocked stops the streaming pipeline (must hold streamMu).
func (p *Player) stopStreamLocked() {
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

// Cancel stops any current playback immediately.
func (p *Player) Cancel() {
	p.streamMu.Lock()
	defer p.streamMu.Unlock()
	p.stopStreamLocked()

	if p.OnPlaybackEnd != nil {
		p.OnPlaybackEnd()
	}
}

// Clear is deprecated - use Cancel instead.
func (p *Player) Clear() {
	p.Cancel()
}

// IsPlaying returns whether audio is currently playing.
func (p *Player) IsPlaying() bool {
	p.streamMu.Lock()
	defer p.streamMu.Unlock()
	return p.streaming
}

// IsSpeaking returns whether Eva is currently speaking.
func (p *Player) IsSpeaking() bool {
	p.speakingMu.Lock()
	defer p.speakingMu.Unlock()
	return p.speaking
}

// AppendPCMChunk appends a raw PCM audio chunk for streaming playback.
// Used by WebSocket TTS for low-latency streaming.
func (p *Player) AppendPCMChunk(pcmData []byte) error {
	if len(pcmData) == 0 {
		return nil
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
		_, err := p.streamStdin.Write(pcmData)
		if err != nil {
			// Pipeline died, try to restart
			p.stopStreamLocked()
			return fmt.Errorf("write to stream: %w", err)
		}
	}

	return nil
}

// PlayPCM plays raw PCM16 audio data at 24kHz via UDP to the robot's audio system.
func (p *Player) PlayPCM(pcmData []byte) error {
	if len(pcmData) == 0 {
		return nil
	}

	p.streamMu.Lock()
	defer p.streamMu.Unlock()

	// Trigger callbacks
	if p.OnPlaybackStart != nil {
		p.speakingMu.Lock()
		p.speaking = true
		p.speakingMu.Unlock()
		p.OnPlaybackStart()
	}

	// GStreamer pipeline - same format as streaming audio
	pipeline := `gst-launch-1.0 -q fdsrc fd=0 ! rawaudioparse format=pcm pcm-format=s16le sample-rate=24000 num-channels=1 ! audioconvert ! audioresample ! audio/x-raw,rate=48000,channels=1,layout=interleaved ! queue ! opusenc frame-size=20 ! rtpopuspay pt=96 ! udpsink host=127.0.0.1 port=5000 sync=true`

	cmd := exec.Command("sshpass", "-p", p.sshPass,
		"ssh", "-o", "StrictHostKeyChecking=no",
		fmt.Sprintf("%s@%s", p.sshUser, p.robotIP),
		pipeline)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start playback: %w", err)
	}

	// Write PCM data
	stdin.Write(pcmData)
	stdin.Close()

	// Wait for playback to complete
	cmd.Wait()

	if p.OnPlaybackEnd != nil {
		p.speakingMu.Lock()
		p.speaking = false
		p.speakingMu.Unlock()
		p.OnPlaybackEnd()
	}

	return nil
}

// SpeakText uses OpenAI TTS to speak text directly (for timer announcements, etc.).
func (p *Player) SpeakText(text string) error {
	if p.openaiKey == "" {
		fmt.Println("🔔 Error: OpenAI API key not set for TTS")
		return fmt.Errorf("OpenAI API key not set")
	}

	fmt.Printf("🔔 Speaking: %s\n", text)
	fmt.Println("🔔 Calling OpenAI TTS...")

	// Call OpenAI TTS API
	payload := map[string]interface{}{
		"model": "tts-1",
		"voice": "shimmer",
		"input": text,
	}
	jsonData, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", "https://api.openai.com/v1/audio/speech", bytes.NewReader(jsonData))
	req.Header.Set("Authorization", "Bearer "+p.openaiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("TTS request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("TTS error (status %d): %s", resp.StatusCode, string(body))
	}

	// Read the MP3 audio
	audioData, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("🔔 Error reading TTS audio: %v\n", err)
		return fmt.Errorf("failed to read audio: %w", err)
	}
	fmt.Printf("🔔 Got %d bytes of audio from TTS\n", len(audioData))

	// Play via SSH and GStreamer
	cmd := exec.Command("sshpass", "-p", p.sshPass,
		"ssh", "-o", "StrictHostKeyChecking=no",
		fmt.Sprintf("%s@%s", p.sshUser, p.robotIP),
		"gst-launch-1.0 fdsrc fd=0 ! mpegaudioparse ! mpg123audiodec ! audioconvert ! audioresample ! alsasink device=default")

	stdin, err := cmd.StdinPipe()
	if err != nil {
		fmt.Printf("🔔 Error getting stdin pipe: %v\n", err)
		return fmt.Errorf("failed to get stdin: %w", err)
	}

	if err := cmd.Start(); err != nil {
		fmt.Printf("🔔 Error starting playback: %v\n", err)
		return fmt.Errorf("failed to start playback: %w", err)
	}

	fmt.Println("🔔 Playing timer audio...")

	// Trigger callbacks
	if p.OnPlaybackStart != nil {
		p.OnPlaybackStart()
	}

	stdin.Write(audioData)
	stdin.Close()

	if err := cmd.Wait(); err != nil {
		fmt.Printf("🔔 Playback error: %v\n", err)
	} else {
		fmt.Println("🔔 Timer audio complete")
	}

	if p.OnPlaybackEnd != nil {
		p.OnPlaybackEnd()
	}

	return nil
}

// ConvertPCM16ToInt16 converts byte slice to int16 samples.
func ConvertPCM16ToInt16(data []byte) []int16 {
	samples := make([]int16, len(data)/2)
	for i := 0; i < len(samples); i++ {
		samples[i] = int16(binary.LittleEndian.Uint16(data[i*2:]))
	}
	return samples
}

// ConvertInt16ToPCM16 converts int16 samples to byte slice.
func ConvertInt16ToPCM16(samples []int16) []byte {
	data := make([]byte, len(samples)*2)
	for i, s := range samples {
		binary.LittleEndian.PutUint16(data[i*2:], uint16(s))
	}
	return data
}

// Resample resamples audio from srcRate to dstRate.
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




