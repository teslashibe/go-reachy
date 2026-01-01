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
│  │   │              conversation.Provider (UNIFIED INTERFACE)              │  │    │
│  │   │                                                                      │  │    │
│  │   │  ┌────────────────────────────┐   ┌─────────────────────────────┐   │  │    │
│  │   │  │ conversation.ElevenLabs    │   │ conversation.OpenAI         │   │  │    │
│  │   │  │ ⭐ PRIMARY (RECOMMENDED)   │OR │ 🔄 FALLBACK                 │   │  │    │
│  │   │  │                            │   │                             │   │  │    │
│  │   │  │ • Custom/cloned voice      │   │ • Fixed voices (shimmer)    │   │  │    │
│  │   │  │ • LLM: Gemini/Claude/GPT   │   │ • LLM: GPT-4o only          │   │  │    │
│  │   │  │ • 16kHz PCM                │   │ • 24kHz PCM                 │   │  │    │
│  │   │  │ • Programmatic config ✨    │   │ • Programmatic config       │   │  │    │
│  │   │  └────────────────────────────┘   └─────────────────────────────┘   │  │    │
│  │   │                                                                      │  │    │
│  │   │  Both implement identical Provider interface - DROP-IN REPLACEMENT   │  │    │
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
| **Live Conversation** | ElevenLabs Agents ⭐ | OpenAI Realtime | `pkg/conversation/Provider` |
| **Timer Announcements** | ElevenLabs | OpenAI TTS | `pkg/tts/Chain` |
| **Vision (describe_scene)** | Gemini Flash | OpenAI GPT-4o | `pkg/inference/Chain` |
| **Web Search** | Gemini + Google Search | None | `inference.GeminiSearch()` |
| **Audio DOA** | go-eva daemon | None | `pkg/audio/Client` |

## Conversation Provider: Drop-In Replacement

Both ElevenLabs and OpenAI implement the **identical `conversation.Provider` interface**, making them fully interchangeable:

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

### Provider Comparison

| Feature | ElevenLabs ⭐ | OpenAI |
|---------|--------------|--------|
| **Voice** | Custom/cloned | Fixed (shimmer, alloy, etc.) |
| **LLM Choice** | Gemini 2.5 Flash, Claude 3.5, GPT-4o | GPT-4o only |
| **Sample Rate** | 16kHz | 24kHz |
| **Latency** | ~200-400ms | ~300-500ms |
| **Programmatic Config** | ✅ Full (after refactor) | ✅ Full |
| **Tool Calling** | ✅ | ✅ |
| **Interruption** | ✅ | ✅ |
| **Custom Personality** | ✅ Code-defined | ✅ Code-defined |

### Why ElevenLabs is Preferred

1. **LLM Flexibility**: Use Gemini 2.5 Flash (faster, cheaper) or Claude 3.5 Sonnet (better reasoning)
2. **Voice Quality**: Custom cloned voices for unique robot personality
3. **Latency**: Slightly lower end-to-end latency
4. **Programmatic**: Full configuration via API (see [TICKET-ELEVENLABS-PROGRAMMATIC.md](./TICKET-ELEVENLABS-PROGRAMMATIC.md))

## Programmatic Configuration ✨ NEW

With the ElevenLabs refactor, **all configuration lives in Go code** - no dashboard required:

### Before (Dashboard Required)
```go
// ❌ Required creating agent in ElevenLabs dashboard
provider, _ := conversation.NewElevenLabs(
    conversation.WithAPIKey(os.Getenv("ELEVENLABS_API_KEY")),
    conversation.WithAgentID(os.Getenv("ELEVENLABS_AGENT_ID")), // From dashboard!
)
// System prompt, tools, LLM configured in dashboard - not in code
```

### After (Fully Programmatic)
```go
// ✅ Everything configured in code
provider, _ := conversation.NewElevenLabs(
    conversation.WithAPIKey(os.Getenv("ELEVENLABS_API_KEY")),
    conversation.WithVoiceID(os.Getenv("ELEVENLABS_VOICE_ID")),
    conversation.WithLLM("gemini-2.0-flash"),  // Or "claude-3-5-sonnet", "gpt-4o"
    conversation.WithSystemPrompt(evaInstructions),
    conversation.WithTools(evaTools...),
    conversation.WithAutoCreateAgent(true),
)
```

See [TICKET-ELEVENLABS-PROGRAMMATIC.md](./TICKET-ELEVENLABS-PROGRAMMATIC.md) for implementation details.

## Package Responsibilities

### `pkg/conversation` - Real-Time Voice Conversation Providers
- **Provider interface**: `Connect()`, `SendAudio()`, `OnAudio()`, `OnToolCall()`, etc.
- **ElevenLabs**: ElevenLabs Agents Platform with custom cloned voice + LLM choice ⭐
- **OpenAI**: OpenAI Realtime API (fallback, GPT-4o only)
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
  Description text → Tool result → Conversation Provider
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
| `CONVERSATION_PROVIDER` | No | `elevenlabs` | Provider: `elevenlabs` (recommended) or `openai` |
| `ELEVENLABS_API_KEY` | Yes* | - | ElevenLabs API key |
| `ELEVENLABS_VOICE_ID` | Yes* | - | Voice ID for ElevenLabs (from dashboard or API) |
| `ELEVENLABS_LLM` | No | `gemini-2.0-flash` | LLM: `gemini-2.0-flash`, `claude-3-5-sonnet`, `gpt-4o` |
| `OPENAI_API_KEY` | Yes | - | OpenAI API key (fallback + vision) |
| `CONVERSATION_VOICE` | No | `shimmer` | Voice for OpenAI conversation (if used) |
| **Vision** ||||
| `GOOGLE_API_KEY` | No | - | Gemini vision + GeminiSearch |
| **Robot** ||||
| `ROBOT_IP` | No | `192.168.68.77` | Reachy Mini IP |
| `SSH_USER` | No | `pollen` | Robot SSH user |
| `SSH_PASS` | No | `root` | Robot SSH password |

*Required if using ElevenLabs as conversation provider

### Deprecated Variables

| Variable | Status | Replacement |
|----------|--------|-------------|
| `ELEVENLABS_AGENT_ID` | Deprecated | Auto-created via API |

## Fallback Chains

### Conversation Chain
```
ElevenLabs Agents (if configured) → OpenAI Realtime
```

### TTS Chain (for announcements)
```
ElevenLabs (if configured) → OpenAI TTS
```

### Inference Chain (for vision)
```
Gemini Flash (if configured) → OpenAI GPT-4o
```

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
  │           ├── "elevenlabs" → conversation.NewElevenLabs() ⭐ DEFAULT
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
│  ├── pkg/conversation/ElevenLabs ⭐ PREFERRED                     │
│  │     • Custom voice                                             │
│  │     • LLM choice (Gemini/Claude/GPT-4o)                        │
│  │     • Programmatic config                                      │
│  └── pkg/conversation/OpenAI     🔄 FALLBACK                      │
│        • Fixed voices                                             │
│        • GPT-4o only                                              │
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

## Related Documents

- [TICKET-ELEVENLABS-PROGRAMMATIC.md](./TICKET-ELEVENLABS-PROGRAMMATIC.md) - Refactor ticket for programmatic config
- [EVA-2.0.md](./EVA-2.0.md) - Eva 2.0 overview and tool calling
- [SETUP.md](./SETUP.md) - Environment setup
- [TROUBLESHOOTING.md](./TROUBLESHOOTING.md) - Common issues
