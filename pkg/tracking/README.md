# pkg/tracking

Head tracking system with face detection and audio DOA fusion.

## Overview

Multi-modal head tracking that fuses visual face detection with audio Direction of Arrival (DOA) to smoothly track people in the room.

## Components

### Tracker

Main tracking coordinator.

```go
config := tracking.DefaultConfig()
tracker, err := tracking.New(config, robot, videoClient, "models/yunet.onnx")
if err != nil {
    log.Fatal(err)
}

// Add audio DOA (optional)
audioClient := audio.NewClient(robotIP)
tracker.SetAudioClient(audioClient)

// Run tracking loop
go tracker.Run(ctx)
```

### WorldModel

Maintains a spatial map of tracked entities.

```go
world := tracking.NewWorldModel()

// Update from sensors
world.UpdateFace(faceResult)
world.UpdateAudio(doaResult)

// Get tracking target
target := world.GetTarget() // Prioritizes face over audio
```

### PDController

Proportional-Derivative controller for smooth head movement.

```go
controller := tracking.NewPDController(config)
output := controller.Update(currentYaw, targetYaw, dt)
```

### Perception

Processes camera frames and detects faces.

```go
perception := tracking.NewPerception(config, detector)
result := perception.Process(jpegFrame)
if result.FaceDetected {
    fmt.Printf("Face at (%.1f, %.1f)\n", result.NormX, result.NormY)
}
```

### detection/YuNet

OpenCV YuNet face detector via ONNX.

```go
detector, _ := detection.NewYuNet(detection.Config{
    ModelPath:        "models/yunet.onnx",
    ConfidenceThresh: 0.5,
})
faces, _ := detector.Detect(jpegFrame)
```

## Config

```go
type Config struct {
    FPS               int     // Tracking loop frequency
    SmoothingFactor   float64 // EMA smoothing (0-1)
    DeadZone          float64 // Min movement threshold
    MaxYaw            float64 // Max head yaw (radians)
    ScanSpeed         float64 // Scanning speed when no target
    ScanPauseTime     time.Duration
    FaceTimeout       time.Duration // How long before face is lost
    Kp, Kd            float64 // PD controller gains
}
```

## Tracking Priority

1. **Face** - Visual face detection (highest priority)
2. **Audio** - DOA from speaking person
3. **Scan** - Slow scanning when no target

## Dependencies

- YuNet ONNX model: `models/face_detection_yunet.onnx`
- go-eva daemon for audio DOA (optional)

