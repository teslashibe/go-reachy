# Speech Package

Speech-synchronized head movement animations for natural speaking gestures.

## Overview

The `Wobbler` analyzes audio amplitude and generates head movement offsets (roll, pitch, yaw) that sync with speech. This makes the robot appear more alive and engaged while talking.

## Integration Flow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           SPEECH WOBBLE FLOW                                │
└─────────────────────────────────────────────────────────────────────────────┘

┌──────────────┐    PCM Audio     ┌──────────────┐    Offsets     ┌──────────────┐
│   ElevenLabs │   (24kHz int16)  │   Speech     │  (roll,pitch,  │   Head       │
│   TTS Stream │ ────────────────▶│   Wobbler    │───yaw degrees)▶│   Tracker    │
└──────────────┘                  └──────────────┘                └──────────────┘
       │                                 │                               │
       │                                 │                               │
       ▼                                 ▼                               ▼
┌──────────────┐                  ┌──────────────┐                ┌──────────────┐
│ OnAudio      │                  │ Audio DSP    │                │ outputPose() │
│ Callback     │                  │ Pipeline     │                │              │
└──────────────┘                  └──────────────┘                └──────────────┘
       │                                 │                               │
       │ bytes→int16                     │                               │
       ▼                                 ▼                               ▼
┌──────────────┐                  ┌──────────────┐                ┌──────────────┐
│ Feed()       │                  │ 1. RMS dB    │                │ finalPitch = │
│ samples,     │                  │ 2. VAD       │                │   tracking   │
│ sampleRate   │                  │ 3. Envelope  │                │ + speech.Pitch│
└──────────────┘                  │ 4. Oscillate │                │              │
                                  └──────────────┘                │ finalYaw =   │
                                         │                        │   tracking   │
                                         │                        │ + speech.Yaw │
                                         ▼                        └──────────────┘
                                  ┌──────────────┐                       │
                                  │ onOffset()   │                       │
                                  │ callback     │                       ▼
                                  └──────────────┘                ┌──────────────┐
                                         │                        │ SetHeadPose  │
                                         │                        │ (roll,pitch, │
                                         ▼                        │  yaw)        │
                                  ┌──────────────┐                └──────────────┘
                                  │ SetSpeech-   │                       │
                                  │ Offsets()    │                       │
                                  └──────────────┘                       ▼
                                                                  ┌──────────────┐
                                                                  │  🤖 Robot    │
                                                                  │  Head Moves  │
                                                                  └──────────────┘
```

## Data Flow Summary

```
TTS Audio → Feed() → RMS/VAD → Envelope → Oscillators → Offsets → Tracker → Robot
                           ↓
                    [Voice detected?]
                     Yes → Scale up
                     No  → Fade out
```

## Usage

```go
import "github.com/teslashibe/go-reachy/pkg/speech"

// Create wobbler with callback
wobbler := speech.NewWobbler(func(roll, pitch, yaw float64) {
    // Apply offsets to head tracker
    headTracker.SetSpeechOffsets(roll, pitch, yaw)
})

// Feed audio samples from TTS
wobbler.Feed(audioSamples, 24000) // int16 samples at 24kHz

// When speech ends
wobbler.Reset()
headTracker.ClearSpeechOffsets()
```

## How It Works

1. **Audio Analysis**: Computes RMS dB from audio frames (20ms window)
2. **Voice Activity Detection (VAD)**: Hysteresis-based detection (-35dB on, -45dB off)
3. **Envelope Follower**: Smooth attack (40ms) / release (250ms) ramping
4. **Oscillators**: Sinusoidal oscillators at different frequencies for organic motion

### Movement Characteristics

| Axis | Frequency | Amplitude | Purpose |
|------|-----------|-----------|---------|
| Pitch | 2.2 Hz | ~4.5° | Nodding motion |
| Yaw | 0.6 Hz | ~7.5° | Side-to-side looking |
| Roll | 1.3 Hz | ~2.25° | Head tilt |

The amplitudes are scaled by:
- **Loudness**: Louder speech = larger movements
- **Envelope**: Smooth attack/release prevents jarring starts/stops

## Parameters (from Python reference)

Ported from `reachy/src/reachy_mini_conversation_app/audio/speech_tapper.py`:

- Sample rate: 24kHz (ElevenLabs default)
- Frame size: 20ms for RMS calculation
- Hop size: 10ms between updates
- VAD thresholds: -35dB on, -45dB off
- Attack/Release: 40ms/250ms

## Thread Safety

The `Wobbler` is thread-safe. You can call `Feed()` from a TTS audio callback goroutine while the tracker runs in its own goroutine.
