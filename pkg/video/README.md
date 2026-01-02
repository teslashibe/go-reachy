# pkg/video

WebRTC video and audio streaming from Reachy Mini.

## Overview

Connects to the robot's GStreamer WebRTC signalling server to receive:
- H.264 video stream (decoded to JPEG frames)
- Opus audio stream (decoded to PCM)

## Usage

```go
import "github.com/teslashibe/go-reachy/pkg/video"

client := video.NewClient("192.168.68.77")

if err := client.Connect(); err != nil {
    log.Fatal(err)
}
defer client.Close()

// Capture video frame
jpeg, err := client.CaptureJPEG()
if err == nil {
    ioutil.WriteFile("frame.jpg", jpeg, 0644)
}

// Capture as image.Image (for inference)
img, _ := client.CaptureImage()

// Record audio
client.StartRecording()
time.Sleep(time.Second)
pcmData := client.StopRecording() // []int16 at 48kHz
```

## Methods

| Method | Returns | Description |
|--------|---------|-------------|
| `Connect()` | error | Establish WebRTC connection |
| `Close()` | error | Close connection |
| `CaptureJPEG()` | []byte, error | Latest video frame as JPEG |
| `CaptureImage()` | image.Image, error | Latest frame as Go image |
| `GetFrame()` | []byte, error | Alias for CaptureJPEG |
| `WaitForFrame(timeout)` | []byte, error | Wait for next frame |
| `StartRecording()` | - | Start audio capture |
| `StopRecording()` | []int16 | Stop and return PCM samples |

## Audio Format

- **Input**: Opus @ 48kHz stereo (from WebRTC)
- **Output**: PCM16 @ 48kHz mono (decoded)

## WebRTC Details

- **Signalling**: `ws://{robotIP}:8443`
- **Video**: H.264 via RTP
- **Audio**: Opus via RTP
- **Decoding**: FFmpeg for H.264, gopkg.in/hraban/opus.v2 for audio

## Requirements

- Reachy Mini daemon with WebRTC enabled
- FFmpeg installed (for H.264 decoding)



