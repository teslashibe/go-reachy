# tracking

Head tracking system for Reachy Mini with face detection and audio DOA.

## Overview

This package provides a complete head tracking system that:
- Detects faces using local YuNet model
- Tracks audio sources via Direction of Arrival (DOA)
- Maintains a world model with spatial awareness
- Uses PD control for smooth head movement
- Scans for faces when none are visible

## Architecture

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   Video     │────▶│  Detection  │────▶│   World     │
│   Source    │     │  (YuNet)    │     │   Model     │
└─────────────┘     └─────────────┘     └─────────────┘
                                              │
┌─────────────┐                               ▼
│   Audio     │────────────────────────▶┌─────────────┐
│   DOA       │                         │   Target    │
└─────────────┘                         │  Selection  │
                                        └─────────────┘
                                              │
                                              ▼
                                        ┌─────────────┐
                                        │ PD Control  │
                                        └─────────────┘
                                              │
                                              ▼
                                        ┌─────────────┐
                                        │   Robot     │
                                        │   Head      │
                                        └─────────────┘
```

## Usage

```go
config := tracking.DefaultConfig()
tracker, err := tracking.New(config, robotCtrl, videoSource, "yunet.onnx")
if err != nil {
    return err
}
defer tracker.Close()

// Start tracking loop
ctx, cancel := context.WithCancel(context.Background())
go tracker.Run(ctx)
```

## Configuration

```go
type Config struct {
    // Detection
    DetectionInterval time.Duration // How often to run detection (default: 100ms)
    
    // Movement
    MovementInterval  time.Duration // Control loop rate (default: 33ms)
    YawRange          float64       // Max head yaw in radians (default: 1.5)
    
    // PD Controller
    Kp                float64       // Proportional gain (default: 0.3)
    Kd                float64       // Derivative gain (default: 0.1)
    ControlDeadZone   float64       // Ignore small errors (default: 0.02)
    
    // Scanning
    ScanSpeed         float64       // Rad/sec when scanning (default: 0.3)
    ScanRange         float64       // Scan limits in radians (default: 1.0)
    ScanStartDelay    time.Duration // Delay before scanning (default: 2s)
}
```

## Offset Mode

For integration with a unified motion controller, use offset mode:

```go
tracker.SetOffsetHandler(func(offset robot.Offset) {
    rateController.SetTrackingOffset(offset)
})
```

## Sub-packages

- `detection/` - Face detection implementations (YuNet, YOLO)

