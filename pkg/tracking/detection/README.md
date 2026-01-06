# detection

Face and object detection implementations.

## Overview

This sub-package provides detection implementations for the tracking system.

## Detectors

### YuNet

Lightweight face detection using the YuNet ONNX model.

```go
config := detection.Config{
    ModelPath:        "yunet.onnx",
    ConfidenceThresh: 0.5,
    InputWidth:       320,
    InputHeight:      320,
}

detector, err := detection.NewYuNet(config)
if err != nil {
    return err
}
defer detector.Close()

faces, err := detector.Detect(jpegData)
```

### YOLO

Object detection using YOLO models (for detecting objects beyond faces).

```go
detector := detection.NewYOLO(yoloClient)
objects, err := detector.DetectObjects(jpegData)
```

## Interface

All detectors implement the `Detector` interface:

```go
type Detector interface {
    Detect(jpeg []byte) ([]Face, error)
    Close() error
}
```

## Face Result

```go
type Face struct {
    X, Y, W, H   int     // Bounding box
    Confidence   float32 // Detection confidence
    CenterX      float64 // Normalized center (0-1)
}
```




