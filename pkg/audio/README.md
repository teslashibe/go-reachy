# audio

Audio processing and playback for Reachy Mini.

## Overview

This package provides audio utilities for:
- Audio playback via GStreamer over SSH
- Direction of Arrival (DOA) client for audio source localization
- Audio format conversion and resampling

## Components

### AudioPlayer

Plays audio on the robot via GStreamer over SSH.

```go
player := audio.NewAudioPlayer("192.168.68.80", 16000)
defer player.Close()

// Play PCM audio
player.Play(pcmData)

// Wait for playback to complete
player.WaitForPlayback()
```

### DOA Client

Receives Direction of Arrival data from the XVF3800 audio processor.

```go
client := audio.NewClient("192.168.68.80:8001")

doa, err := client.GetDOA()
if doa.Speaking {
    fmt.Printf("Voice at angle: %.2f rad\n", doa.Angle)
}
```

## Audio Format Utilities

### Resampling

```go
// Resample from 24kHz to 16kHz
resampled := audio.Resample(data, 24000, 16000)
```

### Format Conversion

```go
// Convert int16 samples to PCM16 bytes
pcm := audio.ConvertInt16ToPCM16(samples)
```

## Configuration

| Env Variable | Description | Default |
|--------------|-------------|---------|
| `ROBOT_IP` | Robot IP address | Required |
| `DOA_PORT` | DOA server port | 8001 |


