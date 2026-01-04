# camera

Runtime-configurable camera settings for Eva.

## Overview

This package provides a configuration API for the IMX708 Wide camera sensor, following the same pattern as `pkg/tracking` for tunable parameters.

## Sensor Capabilities

| Property | Value |
|----------|-------|
| Sensor | Sony IMX708 Wide |
| Max Resolution | 4608 × 2592 (12MP) |
| Max Gain | 16x |
| Max Exposure | 120ms |
| Max Digital Zoom | 4x |
| Autofocus | PDAF (continuous) |

## Usage

```go
import "github.com/teslashibe/go-reachy/pkg/camera"

// Get default high-res config (1080p)
cfg := camera.DefaultConfig()

// Or use a preset
cfg := camera.GetPreset("night")

// Validate before applying
if errors := cfg.Validate(); errors != nil {
    log.Printf("Invalid config: %v", errors)
}
```

## Available Presets

| Preset | Resolution | Description |
|--------|------------|-------------|
| `default` | 1920×1080 | High-res for accurate tracking |
| `legacy` | 640×480 | Original low-res setting |
| `720p` | 1280×720 | Balanced quality/performance |
| `1080p` | 1920×1080 | Full HD |
| `4k` | 3840×2160 | Maximum quality (15fps) |
| `night` | 1280×720 | Optimized for low light |
| `bright` | 1920×1080 | Prevents highlight clipping |
| `zoom2x` | 1920×1080 | 2x digital zoom |
| `zoom4x` | 1920×1080 | 4x digital zoom |

## API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/camera/config` | GET | Get current config |
| `/api/camera/config` | POST | Update config |
| `/api/camera/presets` | GET | List presets |
| `/api/camera/capabilities` | GET | Sensor capabilities |

## Configuration Options

### Resolution
- `width`: Frame width (160-4608)
- `height`: Frame height (120-2592)
- `framerate`: Target FPS (1-120)
- `quality`: JPEG quality (1-100)

### Low Light
- `exposure_mode`: "normal", "short", "long"
- `constraint_mode`: "normal", "highlight", "shadows"
- `exposure_value`: EV compensation (-2.0 to +2.0)
- `brightness`: Brightness adjustment (-1.0 to +1.0)
- `analogue_gain`: Manual gain 1.0-16.0 (0=auto)
- `exposure_time`: Manual exposure µs (0=auto)

### Zoom
- `zoom_level`: Digital zoom 1.0-4.0
- `crop_x/y/width/height`: Manual crop region

### Autofocus
- `af_mode`: "manual", "auto", "continuous"

## Example

```bash
# Set 720p with night mode
curl -X POST http://localhost:8080/api/camera/config \
  -H "Content-Type: application/json" \
  -d '{"width": 1280, "height": 720, "exposure_mode": "long"}'

# Use preset
curl -X POST http://localhost:8080/api/camera/config \
  -d '{"preset": "night"}'
```


