# Tracking Tuning API

Live runtime tuning for Eva's head tracking without restarting.

## Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/tracking/params` | Get all current tuning parameters |
| POST | `/api/tracking/params` | Update tuning parameters (partial updates allowed) |
| POST | `/api/tracking/tuning-mode` | Enable/disable tuning mode |

## Quick Start

```bash
# Get current params
curl http://localhost:3000/api/tracking/params | jq

# Update a single param
curl -X POST http://localhost:3000/api/tracking/params \
  -H "Content-Type: application/json" \
  -d '{"kp_pitch": 0.05}'

# Update multiple params
curl -X POST http://localhost:3000/api/tracking/params \
  -H "Content-Type: application/json" \
  -d '{"kp_pitch": 0.04, "kd_pitch": 0.15, "pitch_dead_zone": 0.10}'

# Enable tuning mode (disables breathing, audio switch, scanning)
curl -X POST http://localhost:3000/api/tracking/tuning-mode \
  -H "Content-Type: application/json" \
  -d '{"enabled": true}'
```

## Parameters Reference

### PD Controller (Yaw)

| Parameter | Default | Range | Description |
|-----------|---------|-------|-------------|
| `kp` | 0.10 | 0.01-0.30 | Proportional gain - how aggressively to chase errors |
| `kd` | 0.12 | 0.05-0.25 | Derivative gain - damping to reduce oscillation |
| `control_dead_zone` | 0.05 | 0.02-0.15 | Ignore errors smaller than this (radians, ~3°) |
| `response_scale` | 0.45 | 0.1-1.0 | Overall response scaling (lower = smoother) |

### Pitch-Specific

Pitch has tighter coupling than yaw (face moves opposite to head tilt immediately), so separate gains help prevent oscillation.

| Parameter | Default | Range | Description |
|-----------|---------|-------|-------------|
| `kp_pitch` | 0.04 | 0.01-0.15 | Pitch proportional gain (lower than yaw) |
| `kd_pitch` | 0.15 | 0.05-0.25 | Pitch derivative gain (higher = more damping) |
| `pitch_dead_zone` | 0.10 | 0.05-0.20 | Pitch dead zone (~6°, larger than yaw) |
| `pitch_range_up` | 0.523 | 0.1-1.0 | Max upward pitch in radians (30°) |
| `pitch_range_down` | 0.523 | 0.1-1.0 | Max downward pitch in radians (30°) |

### Audio Tracking

Turn toward voices detected by go-eva's DOA (Direction of Arrival).

| Parameter | Default | Range | Description |
|-----------|---------|-------|-------------|
| `audio_switch_enabled` | true | bool | Enable turning toward voices |
| `audio_switch_threshold` | 0.52 | 0.2-1.5 | Angle difference to trigger turn (~30°) |
| `audio_switch_min_confidence` | 0.6 | 0.3-0.9 | DOA confidence threshold |
| `audio_switch_look_duration` | 1.5 | 0.5-5.0 | Seconds to look for face at audio direction |

### Breathing Animation

Idle animation when not actively tracking.

| Parameter | Default | Range | Description |
|-----------|---------|-------|-------------|
| `breathing_enabled` | true | bool | Enable idle breathing animation |
| `breathing_amplitude` | 0.05 | 0.02-0.15 | Pitch oscillation amplitude (~3°) |
| `breathing_frequency` | 0.08 | 0.03-0.2 | Breath cycles per second |
| `breathing_antenna_amp` | 0.087 | 0.02-0.2 | Antenna sway amplitude (~5°) |

### Range/Speed Limits

| Parameter | Default | Range | Description |
|-----------|---------|-------|-------------|
| `max_speed` | 0.15 | 0.05-0.30 | Movement speed (radians per tick) |
| `yaw_range` | 2.83 | 1.0-3.14 | Head yaw limit (±162°) |
| `body_yaw_limit` | 2.83 | 1.0-3.14 | Body rotation limit (±162°) |

### Scan Behavior

When no face is detected, Eva scans left/right looking for someone.

| Parameter | Default | Range | Description |
|-----------|---------|-------|-------------|
| `scan_start_delay` | 2.0 | 0.5-10.0 | Seconds before scanning starts |
| `scan_speed` | 0.3 | 0.1-1.0 | Scan speed (rad/s) |
| `scan_range` | 1.0 | 0.5-2.0 | Scan extent (radians) |

### Body Alignment

Gradual body rotation to center the head when locked on a target.

| Parameter | Default | Range | Description |
|-----------|---------|-------|-------------|
| `body_alignment_enabled` | true | bool | Enable automatic body alignment |
| `body_alignment_delay` | 2.0 | 0.5-5.0 | Seconds before alignment starts |
| `body_alignment_threshold` | 0.15 | 0.05-0.5 | Min head yaw to trigger (~9°) |
| `body_alignment_speed` | 0.25 | 0.1-0.5 | Body rotation speed (rad/s) |
| `body_alignment_dead_zone` | 0.05 | 0.02-0.15 | Stop when head within this (~3°) |
| `body_alignment_cooldown` | 0.15 | 0.05-0.5 | Seconds between alignment steps |

### Smoothing

| Parameter | Default | Range | Description |
|-----------|---------|-------|-------------|
| `offset_smoothing_alpha` | 0.7 | 0.1-1.0 | EMA alpha for offsets (higher = more responsive) |
| `position_smoothing` | 0.6 | 0.1-1.0 | Frame position smoothing |
| `max_target_velocity` | 0.15 | 0.05-0.5 | Max target change per tick |

### Detection

| Parameter | Default | Range | Description |
|-----------|---------|-------|-------------|
| `detection_hz` | 20 | 1-20 | Face detection frequency |

## Tuning Mode

Tuning mode disables secondary features (breathing, audio switch, scanning) for clean face tracking evaluation:

```bash
# Enable tuning mode
curl -X POST http://localhost:3000/api/tracking/tuning-mode \
  -H "Content-Type: application/json" \
  -d '{"enabled": true}'

# Disable tuning mode (restore defaults)
curl -X POST http://localhost:3000/api/tracking/tuning-mode \
  -H "Content-Type: application/json" \
  -d '{"enabled": false}'
```

## Troubleshooting

### Pitch oscillation (head bobbing up/down)
```bash
curl -X POST http://localhost:3000/api/tracking/params \
  -H "Content-Type: application/json" \
  -d '{"kp_pitch": 0.03, "kd_pitch": 0.18, "pitch_dead_zone": 0.12}'
```

### Yaw too aggressive (overshoots when turning)
```bash
curl -X POST http://localhost:3000/api/tracking/params \
  -H "Content-Type: application/json" \
  -d '{"kp": 0.08, "kd": 0.15, "response_scale": 0.35}'
```

### Not tracking far enough (stops at limit)
```bash
curl -X POST http://localhost:3000/api/tracking/params \
  -H "Content-Type: application/json" \
  -d '{"yaw_range": 2.83, "body_yaw_limit": 2.83}'
```

### Audio triggering false positives
```bash
curl -X POST http://localhost:3000/api/tracking/params \
  -H "Content-Type: application/json" \
  -d '{"audio_switch_min_confidence": 0.75, "audio_switch_threshold": 0.7}'
```

### Disable features for debugging
```bash
curl -X POST http://localhost:3000/api/tracking/params \
  -H "Content-Type: application/json" \
  -d '{"breathing_enabled": false, "audio_switch_enabled": false}'
```

## Response Format

### GET /api/tracking/params

```json
{
  "kp": 0.10,
  "kd": 0.12,
  "kp_pitch": 0.04,
  "kd_pitch": 0.15,
  "pitch_dead_zone": 0.10,
  "control_dead_zone": 0.05,
  "response_scale": 0.45,
  "detection_hz": 20,
  "audio_switch_enabled": true,
  "audio_switch_threshold": 0.52,
  "breathing_enabled": true,
  ...
}
```

### POST /api/tracking/params

Request (partial update):
```json
{
  "kp_pitch": 0.05,
  "pitch_dead_zone": 0.12
}
```

Response:
```json
{
  "status": "ok",
  "updated": {
    "kp_pitch": 0.05,
    "pitch_dead_zone": 0.12
  }
}
```

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Web Dashboard (:3000)                     │
│  /api/tracking/params  →  OnSetTuningParams callback        │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                    pkg/tracking/tuning.go                    │
│  TuningParams struct  ←→  GetTuningParams / SetTuningParams │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│  Tracker.config     │  PDController     │  Perception       │
│  (Config struct)    │  (gains, limits)  │  (smoothing)      │
└─────────────────────────────────────────────────────────────┘
```

## Files

| File | Purpose |
|------|---------|
| `tuning.go` | TuningParams struct, Get/Set methods |
| `config.go` | Full Config struct with defaults |
| `limits.go` | Mechanical limit constants |
| `controller.go` | PD controller implementation |
| `perception.go` | Face position processing |
| `tracker.go` | Main tracking orchestration |


