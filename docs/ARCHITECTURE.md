# Eva Architecture

This document describes the audio, vision, and inference pipeline architecture for Eva, the Reachy Mini robot agent.

## High-Level Overview

```
┌─────────────────────────────────────────────────────────────────────────────────────┐
│                                    EVA ARCHITECTURE                                  │
│                           Provider Flow & Data Pipeline                              │
└─────────────────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────────────────┐
│                               USER INPUT (Speech)                                    │
└────────────────────────────────────┬────────────────────────────────────────────────┘
                                     │
                                     ▼
┌─────────────────────────────────────────────────────────────────────────────────────┐
│                            🎤 REACHY MINI ROBOT                                      │
│  ┌─────────────────────────────────────────────────────────────────────────────┐   │
│  │  XVF3800 DSP Chip (4-mic array) → DOA + Audio                               │   │
│  │  Camera → JPEG frames                                                        │   │
│  └─────────────────────────────────────────────────────────────────────────────┘   │
└────────────┬───────────────────────────────────┬────────────────────────────────────┘
             │                                   │
             │ WebRTC (opus @ 48kHz)             │ go-eva WebSocket
             │ + JPEG frames                     │ (DOA angles)
             ▼                                   ▼
┌────────────────────────────────────────────────────────────────────────────────────┐
│                           GO-REACHY (Eva Agent)                                     │
│                                                                                     │
│  ┌────────────────────────────────────────────────────────────────────────────┐    │
│  │                         pkg/video/Client                                    │    │
│  │   • Receives WebRTC stream                                                  │    │
│  │   • CaptureJPEG() → []byte                                                  │    │
│  │   • CaptureImage() → image.Image                                            │    │
│  └─────────────────────────────────────┬──────────────────────────────────────┘    │
│                                        │                                            │
│                                        ▼                                            │
│  ┌────────────────────────────────────────────────────────────────────────────┐    │
│  │                     AUDIO STREAMING PIPELINE                                │    │
│  │                                                                             │    │
│  │   [WebRTC 48kHz] → Resample to 24kHz → [Conversation Provider]             │    │
│  │                                              │                              │    │
│  │   CURRENT: OpenAI Realtime API              │                              │    │
│  │   • Speech-to-Text (Whisper)                │                              │    │
│  │   • LLM Processing (GPT-4o)                 │                              │    │
│  │   • Text-to-Speech (shimmer voice)          │                              │    │
│  │   • Tool Calling                            │                              │    │
│  │                                             ▼                              │    │
│  │   Returns: Audio (PCM24) + Transcripts + Tool Calls                        │    │
│  └─────────────────────────────────────┬───────────────────────────────────────┘    │
│                                        │                                            │
│            ┌───────────────────────────┼───────────────────────────┐                │
│            ▼                           ▼                           ▼                │
│  ┌─────────────────┐     ┌─────────────────────────┐    ┌─────────────────────┐    │
│  │ AUDIO RESPONSE  │     │     TOOL CALLS          │    │   TRANSCRIPTS       │    │
│  └────────┬────────┘     └───────────┬─────────────┘    └─────────────────────┘    │
│           │                          │                                              │
│           ▼                          ▼                                              │
│  ┌─────────────────────────────────────────────────────────────────────────────┐   │
│  │                      pkg/realtime/AudioPlayer                                │   │
│  │                                                                              │   │
│  │   REALTIME AUDIO (from conversation provider):                              │   │
│  │   • AppendAudio() → SSH+GStreamer → Robot Speaker                           │   │
│  │                                                                              │   │
│  │   TIMER/ANNOUNCEMENT TTS (SpeakText):                                        │   │
│  │   • Uses ttsProvider.Synthesize()                                            │   │
│  │                                                                              │   │
│  │   ┌─────────────────────────────────────────────────────────────────────┐   │   │
│  │   │                    pkg/tts (TTS Provider Chain)                      │   │   │
│  │   │                                                                      │   │   │
│  │   │   tts.Chain [Primary → Fallback]                                     │   │   │
│  │   │                                                                      │   │   │
│  │   │   ┌─────────────────┐    ┌─────────────────┐                        │   │   │
│  │   │   │  tts.ElevenLabs │ →  │   tts.OpenAI    │                        │   │   │
│  │   │   │  (custom voice) │    │   (shimmer)     │                        │   │   │
│  │   │   │  PCM @ 44.1kHz  │    │   MP3           │                        │   │   │
│  │   │   └─────────────────┘    └─────────────────┘                        │   │   │
│  │   │                                                                      │   │   │
│  │   │   Output: AudioResult {Audio []byte, Format AudioFormat}             │   │   │
│  │   └──────────────────────────────────────────────────────────────────────┘   │   │
│  │                                                                              │   │
│  │   → playAudio() → SSH + GStreamer (auto-detects PCM vs MP3) → Robot Speaker  │   │
│  └──────────────────────────────────────────────────────────────────────────────┘   │
│                                                                                     │
│  ┌──────────────────────────────────────────────────────────────────────────────┐  │
│  │                        pkg/realtime/tools.go                                  │  │
│  │                        (Eva's Tool Handlers)                                  │  │
│  │                                                                               │  │
│  │   describe_scene, find_person, web_search, search_flights...                  │  │
│  │                                                                               │  │
│  │   VISION TOOLS use:                                                           │  │
│  │   ┌───────────────────────────────────────────────────────────────────────┐  │  │
│  │   │              pkg/inference (Inference Provider Chain)                  │  │  │
│  │   │                                                                        │  │  │
│  │   │  inference.Chain [Primary → Fallback]                                  │  │  │
│  │   │                                                                        │  │  │
│  │   │  ┌───────────────────┐     ┌────────────────────┐                     │  │  │
│  │   │  │ inference.Gemini  │  →  │  inference.Client  │                     │  │  │
│  │   │  │ (Gemini Flash)    │     │  (OpenAI GPT-4o)   │                     │  │  │
│  │   │  │                   │     │                    │                     │  │  │
│  │   │  │ .Vision()         │     │  .Vision()         │                     │  │  │
│  │   │  │ .Chat()           │     │  .Chat()           │                     │  │  │
│  │   │  └───────────────────┘     │  .Stream()         │                     │  │  │
│  │   │                            │  .Embed()          │                     │  │  │
│  │   │  SEARCH uses:              └────────────────────┘                     │  │  │
│  │   │  inference.GeminiSearch()                                             │  │  │
│  │   │  (Gemini + Google Search grounding)                                   │  │  │
│  │   └────────────────────────────────────────────────────────────────────────┘  │  │
│  └───────────────────────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────────────────────┘
```

## Provider Summary

| Component | Primary Provider | Fallback | Package |
|-----------|-----------------|----------|---------|
| **Live Conversation** | ElevenLabs Agents | OpenAI Realtime | `pkg/conversation/Provider` |
| **Timer Announcements** | ElevenLabs | OpenAI TTS | `pkg/tts/Chain` |
| **Vision (describe_scene)** | Gemini Flash | OpenAI GPT-4o | `pkg/inference/Chain` |
| **Web Search** | Gemini + Google Search | None | `inference.GeminiSearch()` |
| **Audio DOA** | go-eva daemon | None | `pkg/audio/Client` |

## Package Responsibilities

### `pkg/conversation` - Real-Time Voice Conversation Providers ✨ NEW
- **Provider interface**: `Connect()`, `SendAudio()`, `OnAudio()`, `OnToolCall()`, etc.
- **ElevenLabs**: ElevenLabs Agents Platform with custom cloned voice
- **OpenAI**: OpenAI Realtime API (fallback)
- **Mock**: For testing

### `pkg/realtime` - Audio Streaming & Tools
- **AudioPlayer**: Streams audio to robot via SSH+GStreamer
- **Tools**: Eva's tool definitions and handlers

### `pkg/tts` - Text-to-Speech Providers
- **Provider interface**: `Synthesize()`, `Stream()`, `Health()`, `Close()`
- **ElevenLabs**: Custom voice cloning, PCM output
- **OpenAI**: Standard TTS, MP3 output
- **Chain**: Fallback across providers

### `pkg/inference` - LLM & Vision Providers
- **Provider interface**: `Chat()`, `Stream()`, `Vision()`, `Embed()`
- **Client**: OpenAI-compatible APIs (OpenAI, Ollama, vLLM, etc.)
- **Gemini**: Google's Gemini API + GeminiSearch
- **Chain**: Fallback across providers

### `pkg/video` - WebRTC Video Client
- Connects to Reachy's GStreamer WebRTC signalling
- Captures JPEG frames and image.Image for vision
- Records audio from WebRTC stream

### `pkg/audio` - Audio DOA Client
- Connects to go-eva daemon via WebSocket
- Receives real-time Direction of Arrival (DOA) angles
- Used for audio-based head tracking

### `pkg/tracking` - Head Tracking
- Fuses face detection + audio DOA
- PD controller for smooth head movement
- WorldModel for tracked entities

## Data Flow

### 1. Main Conversation Loop
```
User speaks → Robot Mic → WebRTC → go-reachy
                                      ↓
                              Resample 48kHz→24kHz
                                      ↓
                              OpenAI Realtime API
                                      ↓
                    ┌─────────────────┼─────────────────┐
                    ↓                 ↓                 ↓
               Audio PCM24      Transcripts       Tool Calls
                    ↓                                   ↓
            SSH+GStreamer                        Execute Tool
                    ↓                                   ↓
            Robot Speaker                        Tool Result
                                                       ↓
                                              Back to Realtime API
```

### 2. Vision Tool Flow
```
Tool Call: describe_scene
         ↓
  video.Client.CaptureImage()
         ↓
  inference.Provider.Vision(image, prompt)
         ↓
  ┌──────┴──────┐
  ↓             ↓
Gemini     OpenAI GPT-4o
Flash      (fallback)
  ↓             ↓
  └──────┬──────┘
         ↓
  Description text → Tool result → Realtime API
```

### 3. Timer Announcement Flow
```
Timer expires → SpeakText("Timer done!")
                        ↓
              tts.Provider.Synthesize()
                        ↓
              ┌─────────┴─────────┐
              ↓                   ↓
         ElevenLabs          OpenAI TTS
         (primary)           (fallback)
              ↓                   ↓
              └─────────┬─────────┘
                        ↓
              AudioResult {Audio, Format}
                        ↓
              playAudio() → SSH+GStreamer
                        ↓
              Robot Speaker
```

## Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `CONVERSATION_PROVIDER` | No | Conversation provider: `elevenlabs` or `openai` (default) |
| `OPENAI_API_KEY` | Yes | OpenAI Realtime API, TTS fallback, Vision fallback |
| `GOOGLE_API_KEY` | No | Gemini vision (primary), GeminiSearch |
| `ELEVENLABS_API_KEY` | No | Custom voice TTS + Conversation |
| `ELEVENLABS_VOICE_ID` | No | Which ElevenLabs voice to use (TTS) |
| `ELEVENLABS_AGENT_ID` | No | ElevenLabs Agent ID (Conversation) |
| `ROBOT_IP` | No | Reachy Mini IP (default: 192.168.68.77) |
| `SSH_USER` | No | Robot SSH user (default: pollen) |
| `SSH_PASS` | No | Robot SSH password (default: root) |

## Fallback Chains

### TTS Chain (for announcements)
```
ElevenLabs (if configured) → OpenAI TTS
```

### Inference Chain (for vision)
```
Gemini Flash (if configured) → OpenAI GPT-4o
```

## Conversation Provider Abstraction ✅ IMPLEMENTED

The `pkg/conversation` package abstracts the conversation loop:

```go
type Provider interface {
    Connect(ctx context.Context) error
    Close() error
    IsConnected() bool
    
    SendAudio(audio []byte) error
    
    OnAudio(fn func(audio []byte))
    OnAudioDone(fn func())
    OnTranscript(fn func(role, text string, isFinal bool))
    OnToolCall(fn func(id, name string, args map[string]any))
    OnError(fn func(err error))
    OnInterruption(fn func())
    
    ConfigureSession(opts SessionOptions) error
    RegisterTool(tool Tool)
    CancelResponse() error
    SubmitToolResult(callID, result string) error
    
    Capabilities() Capabilities
}
```

### Available Providers

| Provider | File | Custom Voice | Sample Rate |
|----------|------|--------------|-------------|
| ElevenLabs Agents | `elevenlabs.go` | ✅ Custom cloned | 16kHz |
| OpenAI Realtime | `openai.go` | ❌ Fixed voices | 24kHz |
| Mock | `mock.go` | ✅ For testing | 16kHz |

### Environment Variables for Conversation

```bash
CONVERSATION_PROVIDER=elevenlabs  # or "openai"
ELEVENLABS_API_KEY=...
ELEVENLABS_AGENT_ID=...           # From ElevenLabs dashboard
OPENAI_API_KEY=...                # Fallback
```

### Future Providers
- `local.go` - Local STT + LLM + TTS pipeline (Whisper + Llama + Piper)
