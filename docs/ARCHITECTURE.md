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
│  │   [WebRTC 48kHz] → Resample → [pkg/conversation Provider]                  │    │
│  │                        ↓                                                    │    │
│  │   ┌─────────────────────────────────────────────────────────────────────┐  │    │
│  │   │              conversation.Provider (env: CONVERSATION_PROVIDER)      │  │    │
│  │   │                                                                      │  │    │
│  │   │  ┌────────────────────┐     ┌─────────────────────┐                 │  │    │
│  │   │  │ conversation.      │     │ conversation.       │                 │  │    │
│  │   │  │ ElevenLabs         │  OR │ OpenAI              │                 │  │    │
│  │   │  │ (custom voice)     │     │ (shimmer/alloy)     │                 │  │    │
│  │   │  │ 16kHz PCM          │     │ 24kHz PCM           │                 │  │    │
│  │   │  │ STT+LLM+TTS        │     │ Whisper+GPT-4o+TTS  │                 │  │    │
│  │   │  └────────────────────┘     └─────────────────────┘                 │  │    │
│  │   └─────────────────────────────────────────────────────────────────────┘  │    │
│  │                                             ↓                              │    │
│  │   Returns: Audio + Transcripts + Tool Calls                                │    │
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
User speaks → Robot Mic → WebRTC → go-reachy (cmd/eva/main.go)
                                      ↓
                              streamAudioToConversation()
                                      ↓
                              Resample 48kHz → Provider rate
                              (16kHz ElevenLabs, 24kHz OpenAI)
                                      ↓
                              convProvider.SendAudio()
                                      ↓
                    ┌─────────────────┼─────────────────┐
                    │                 │                 │
                    ▼                 ▼                 ▼
       ┌────────────────┐  ┌─────────────────┐  ┌─────────────────┐
       │ OnAudio()      │  │ OnTranscript()  │  │ OnToolCall()    │
       │ Audio bytes    │  │ user/agent text │  │ id, name, args  │
       └───────┬────────┘  └────────┬────────┘  └────────┬────────┘
               ↓                    ↓                    ↓
       AppendAudioBytes()      Print to           toolHandlers[name]()
               ↓               console                   ↓
       SSH + GStreamer                           Execute Tool
               ↓                                         ↓
       Robot Speaker                             SubmitToolResult()
                                                         ↓
                                                 Back to Provider
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

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| **Conversation** ||||
| `CONVERSATION_PROVIDER` | No | `openai` | Provider: `openai` or `elevenlabs` |
| `CONVERSATION_VOICE` | No | `shimmer` | Voice for OpenAI conversation |
| `OPENAI_API_KEY` | Yes | - | OpenAI Realtime API + fallbacks |
| `ELEVENLABS_API_KEY` | No | - | ElevenLabs TTS + Conversation |
| `ELEVENLABS_AGENT_ID` | No | - | ElevenLabs Agent ID (Conversation) |
| `ELEVENLABS_VOICE_ID` | No | - | ElevenLabs Voice ID (TTS only) |
| **Vision** ||||
| `GOOGLE_API_KEY` | No | - | Gemini vision + GeminiSearch |
| **Robot** ||||
| `ROBOT_IP` | No | `192.168.68.77` | Reachy Mini IP |
| `SSH_USER` | No | `pollen` | Robot SSH user |
| `SSH_PASS` | No | `root` | Robot SSH password |

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

## Main.go Integration

The `cmd/eva/main.go` file wires everything together:

### Initialization Flow

```
main()
  ├── initialize()
  │     └── Create robot, memory, audioPlayer
  │
  ├── connectWebRTC()
  │     └── video.NewClient() → WebRTC stream
  │
  ├── tracking.New()
  │     └── Face detection + audio DOA
  │
  ├── initConversationProvider()
  │     └── Based on CONVERSATION_PROVIDER env:
  │           ├── "elevenlabs" → conversation.NewElevenLabs()
  │           └── "openai"     → conversation.NewOpenAI()
  │
  ├── connectConversation()
  │     ├── initTTSProvider()      → tts.Chain (ElevenLabs → OpenAI)
  │     ├── initInferenceProvider() → inference.Chain (Gemini → OpenAI)
  │     ├── Build toolHandlers map from realtime.EvaTools()
  │     ├── Set up callbacks (OnAudio, OnTranscript, OnToolCall, etc.)
  │     └── convProvider.Connect()
  │
  └── Start goroutines:
        ├── streamAudioToConversation()
        ├── headTracker.Run()
        ├── startWebDashboard()
        └── streamCameraToWeb()
```

### Key Functions

| Function | Purpose |
|----------|---------|
| `initConversationProvider()` | Creates ElevenLabs or OpenAI provider based on env |
| `connectConversation()` | Sets up callbacks and connects |
| `streamAudioToConversation()` | Captures WebRTC audio, resamples, sends to provider |
| `initTTSProvider()` | Creates ElevenLabs → OpenAI TTS chain |
| `initInferenceProvider()` | Creates Gemini → OpenAI inference chain |

### Callback Wiring

```go
// Audio from agent → Robot speaker
convProvider.OnAudio(func(audio []byte) {
    audioPlayer.AppendAudioBytes(audio)
})

// Transcript events → Console + Dashboard
convProvider.OnTranscript(func(role, text string, isFinal bool) {
    if role == "user" { fmt.Printf("👤 User: %s\n", text) }
    if role == "agent" { fmt.Print(text) }
})

// Tool calls → Execute → Return result
convProvider.OnToolCall(func(callID, name string, args map[string]any) {
    result, _ := toolHandlers[name](args)
    convProvider.SubmitToolResult(callID, result)
})

// Interruption → Cancel audio
convProvider.OnInterruption(func() {
    audioPlayer.Cancel()
    convProvider.CancelResponse()
})
```

## Complete Provider Stack

```
┌──────────────────────────────────────────────────────────────────┐
│                         EVA PROVIDERS                             │
├──────────────────────────────────────────────────────────────────┤
│                                                                   │
│  CONVERSATION (full voice loop)                                   │
│  ├── pkg/conversation/ElevenLabs (custom voice) ← PREFERRED      │
│  └── pkg/conversation/OpenAI (fixed voices)     ← FALLBACK       │
│                                                                   │
│  TTS (timer announcements)                                        │
│  ├── pkg/tts/ElevenLabs (custom voice)          ← PREFERRED      │
│  └── pkg/tts/OpenAI (shimmer)                   ← FALLBACK       │
│                                                                   │
│  INFERENCE (vision + search)                                      │
│  ├── pkg/inference/Gemini (fast, grounded)      ← PREFERRED      │
│  └── pkg/inference/Client (OpenAI compatible)   ← FALLBACK       │
│                                                                   │
│  AUDIO DOA (spatial tracking)                                     │
│  └── pkg/audio/Client → go-eva daemon           ← REQUIRED       │
│                                                                   │
└──────────────────────────────────────────────────────────────────┘
```
