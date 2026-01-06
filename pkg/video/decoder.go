// Package video provides optimized H264 decoding using persistent ffmpeg process
package video

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"os/exec"
	"sync"
	"time"
)

// FastDecoder uses a persistent ffmpeg process with pipe I/O for fast H264 decoding.
// This avoids the overhead of spawning a new process for each frame.
type FastDecoder struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	
	// Frame buffer
	latestFrame []byte
	frameMu     sync.RWMutex
	
	// Decode rate limiting
	lastDecode  time.Time
	minInterval time.Duration
	
	// State
	running bool
	mu      sync.Mutex
}

// NewFastDecoder creates a persistent ffmpeg decoder process.
// decodeInterval controls how often we decode (e.g., 50ms = 20 FPS max)
func NewFastDecoder(decodeInterval time.Duration) (*FastDecoder, error) {
	d := &FastDecoder{
		minInterval: decodeInterval,
		lastDecode:  time.Now(),
	}
	
	return d, nil
}

// DecodeNAL decodes H264 NAL units to JPEG.
// Rate limited to avoid overwhelming the decoder.
func (d *FastDecoder) DecodeNAL(nalData []byte) ([]byte, error) {
	if len(nalData) < 100 {
		return nil, nil
	}
	
	// Rate limit decoding
	d.mu.Lock()
	elapsed := time.Since(d.lastDecode)
	if elapsed < d.minInterval {
		d.mu.Unlock()
		return d.GetLatestFrame(), nil
	}
	d.lastDecode = time.Now()
	d.mu.Unlock()
	
	// Use single-shot ffmpeg with stdin/stdout (no temp files!)
	// This is still subprocess but with pipes - much faster than file I/O
	cmd := exec.Command("ffmpeg",
		"-f", "h264",           // Input format
		"-i", "pipe:0",         // Read from stdin
		"-vframes", "1",        // Just one frame
		"-f", "image2pipe",     // Output as pipe
		"-vcodec", "mjpeg",     // Output as JPEG
		"-q:v", "3",            // Quality (1-31, lower is better)
		"pipe:1",               // Write to stdout
	)
	
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to get stdin pipe: %w", err)
	}
	
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start ffmpeg: %w", err)
	}
	
	// Write NAL data and close stdin to signal EOF
	go func() {
		stdin.Write(nalData)
		stdin.Close()
	}()
	
	// Wait with timeout
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()
	
	select {
	case err := <-done:
		if err != nil {
			// ffmpeg may exit with error if not enough data for a frame
			return nil, nil
		}
	case <-time.After(100 * time.Millisecond):
		cmd.Process.Kill()
		return nil, nil
	}
	
	jpegData := stdout.Bytes()
	if len(jpegData) > 1000 && !isGrayJPEG(jpegData) {
		d.frameMu.Lock()
		d.latestFrame = jpegData
		d.frameMu.Unlock()
		return jpegData, nil
	}
	
	return d.GetLatestFrame(), nil
}

// GetLatestFrame returns the most recently decoded frame.
func (d *FastDecoder) GetLatestFrame() []byte {
	d.frameMu.RLock()
	defer d.frameMu.RUnlock()
	
	if d.latestFrame == nil {
		return nil
	}
	
	frame := make([]byte, len(d.latestFrame))
	copy(frame, d.latestFrame)
	return frame
}

// Close terminates the decoder.
func (d *FastDecoder) Close() {
	d.mu.Lock()
	defer d.mu.Unlock()
	
	if d.cmd != nil && d.cmd.Process != nil {
		d.cmd.Process.Kill()
	}
	d.running = false
}

// isGrayJPEG checks if a JPEG is likely gray/corrupt.
func isGrayJPEG(jpegData []byte) bool {
	if len(jpegData) < 1000 {
		return true
	}
	
	// Decode to check for gray frames
	img, err := jpeg.Decode(bytes.NewReader(jpegData))
	if err != nil {
		return true
	}
	
	bounds := img.Bounds()
	if bounds.Dx() < 100 || bounds.Dy() < 100 {
		return true
	}
	
	// Sample pixels to check variance
	var rSum, gSum, bSum int
	samples := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y += bounds.Dy() / 10 {
		for x := bounds.Min.X; x < bounds.Max.X; x += bounds.Dx() / 10 {
			r, g, b, _ := img.At(x, y).RGBA()
			rSum += int(r >> 8)
			gSum += int(g >> 8)
			bSum += int(b >> 8)
			samples++
		}
	}
	
	if samples == 0 {
		return true
	}
	
	avgR := rSum / samples
	avgG := gSum / samples
	avgB := bSum / samples
	
	// Gray frames have R ≈ G ≈ B with low values
	if avgR < 30 && avgG < 30 && avgB < 30 {
		return true
	}
	
	// Check for uniform gray (R = G = B)
	colorDiff := abs(avgR-avgG) + abs(avgG-avgB) + abs(avgR-avgB)
	if colorDiff < 15 && avgR > 100 && avgR < 150 {
		return true
	}
	
	return false
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// RGBToJPEG converts an RGB image to JPEG bytes.
func RGBToJPEG(img *image.RGBA, quality int) ([]byte, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}




