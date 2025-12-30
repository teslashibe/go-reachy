package realtime

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"os/exec"
	"sync"
)

// AudioPlayer handles streaming audio playback to the robot
type AudioPlayer struct {
	robotIP string
	sshUser string
	sshPass string

	buffer   bytes.Buffer
	bufferMu sync.Mutex

	playing bool
	playMu  sync.Mutex

	OnPlaybackStart func()
	OnPlaybackEnd   func()
}

// NewAudioPlayer creates a new audio player for the robot
func NewAudioPlayer(robotIP, sshUser, sshPass string) *AudioPlayer {
	return &AudioPlayer{
		robotIP: robotIP,
		sshUser: sshUser,
		sshPass: sshPass,
	}
}

// AppendAudio adds audio data to the buffer (base64 encoded PCM16)
func (p *AudioPlayer) AppendAudio(base64Audio string) error {
	decoded, err := base64.StdEncoding.DecodeString(base64Audio)
	if err != nil {
		return err
	}

	p.bufferMu.Lock()
	p.buffer.Write(decoded)
	p.bufferMu.Unlock()

	return nil
}

// StartStreaming begins playing buffered audio to the robot
func (p *AudioPlayer) StartStreaming() error {
	p.playMu.Lock()
	if p.playing {
		p.playMu.Unlock()
		return nil
	}
	p.playing = true
	p.playMu.Unlock()

	if p.OnPlaybackStart != nil {
		p.OnPlaybackStart()
	}

	go p.streamLoop()
	return nil
}

func (p *AudioPlayer) streamLoop() {
	defer func() {
		p.playMu.Lock()
		p.playing = false
		p.playMu.Unlock()

		if p.OnPlaybackEnd != nil {
			p.OnPlaybackEnd()
		}
	}()

	// Wait for buffer to fill
	for {
		p.bufferMu.Lock()
		size := p.buffer.Len()
		p.bufferMu.Unlock()
		if size > 4800 { // ~100ms of audio at 24kHz
			break
		}
	}

	// Stream to robot
	p.FlushAndPlay()
}

// FlushAndPlay plays all buffered audio immediately
func (p *AudioPlayer) FlushAndPlay() error {
	p.bufferMu.Lock()
	data := p.buffer.Bytes()
	p.buffer.Reset()
	p.bufferMu.Unlock()

	if len(data) == 0 {
		return nil
	}

	if p.OnPlaybackStart != nil {
		p.OnPlaybackStart()
	}

	defer func() {
		if p.OnPlaybackEnd != nil {
			p.OnPlaybackEnd()
		}
	}()

	// OpenAI Realtime outputs PCM16 at 24kHz mono
	// Robot expects audio via UDP port 5000 as Opus RTP
	// Pipeline: raw PCM → convert to 48kHz → Opus encode → RTP → UDP
	
	// First, save PCM to temp file on robot, then play via GStreamer
	cmd := exec.Command("bash", "-c", fmt.Sprintf(
		`sshpass -p "%s" ssh -o StrictHostKeyChecking=no %s@%s "cat > /tmp/eva_audio.raw && gst-launch-1.0 filesrc location=/tmp/eva_audio.raw ! rawaudioparse format=pcm pcm-format=s16le sample-rate=24000 num-channels=1 ! audioconvert ! audioresample ! audio/x-raw,rate=48000,channels=1,layout=interleaved ! opusenc ! rtpopuspay pt=96 ! udpsink host=127.0.0.1 port=5000 2>/dev/null"`,
		p.sshPass, p.sshUser, p.robotIP))

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start cmd: %w", err)
	}

	// Write PCM data
	_, err = stdin.Write(data)
	if err != nil {
		return fmt.Errorf("write data: %w", err)
	}
	stdin.Close()

	// Wait for playback
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("cmd wait: %w", err)
	}

	return nil
}

// Clear clears the audio buffer
func (p *AudioPlayer) Clear() {
	p.bufferMu.Lock()
	p.buffer.Reset()
	p.bufferMu.Unlock()
}

// IsPlaying returns whether audio is currently playing
func (p *AudioPlayer) IsPlaying() bool {
	p.playMu.Lock()
	defer p.playMu.Unlock()
	return p.playing
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
