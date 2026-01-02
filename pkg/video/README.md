# video

Video streaming client for Reachy Mini camera.

## Overview

This package provides a client for capturing video frames from the Reachy Mini's camera via WebRTC or HTTP streaming.

## Usage

```go
client, err := video.NewClient("192.168.68.80")
if err != nil {
    return err
}
defer client.Close()

// Capture a JPEG frame
jpeg, err := client.CaptureJPEG()
if err != nil {
    return err
}

// Use the frame for detection, vision, etc.
faces, _ := detector.Detect(jpeg)
```

## Features

- JPEG frame capture
- H264 → JPEG decoding (via FFmpeg)
- WebRTC connection handling
- Automatic reconnection

## VideoSource Interface

Implements the interface used by tracking:

```go
type VideoSource interface {
    CaptureJPEG() ([]byte, error)
}
```

## Configuration

| Option | Description | Default |
|--------|-------------|---------|
| Robot IP | Camera source IP | Required |
| Port | WebRTC port | 8443 |

