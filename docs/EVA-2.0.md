# Eva 2.0 - Low-Latency Conversational Robot Agent

## Overview

Eva 2.0 is a complete rewrite of Eva's conversational system, using **OpenAI's Realtime API** for native speech-to-speech conversation with tool use.

## Key Improvements

| Metric | Eva 1.0 | Eva 2.0 |
|--------|---------|---------|
| **Round-trip latency** | 5-10 seconds | < 1 second |
| **Architecture** | STT → LLM → TTS | Native speech-to-speech |
| **Tool use** | Hardcoded behaviors | Dynamic function calling |
| **Interruption** | None | Built-in |
| **Streaming** | None | Full streaming audio |

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                        Eva 2.0                              │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  ┌─────────────┐   ┌─────────────┐   ┌─────────────┐       │
│  │   WebRTC    │──▶│   OpenAI    │──▶│   Tool      │       │
│  │   Audio     │   │   Realtime  │   │   Executor  │       │
│  │  (48kHz)    │   │    API      │   │             │       │
│  └─────────────┘   └─────────────┘   └─────────────┘       │
│        │                 │                  │               │
│        │                 ▼                  ▼               │
│        │           ┌─────────────┐   ┌─────────────┐       │
│        │           │  Streaming  │   │   Robot     │       │
│        │           │   Audio     │   │   Control   │       │
│        │           │  (24kHz)    │   │   (HTTP)    │       │
│        │           └─────────────┘   └─────────────┘       │
│        │                 │                  │               │
│        ▼                 ▼                  ▼               │
│  ┌─────────────┐   ┌─────────────┐   ┌─────────────┐       │
│  │   Camera    │   │   Speaker   │   │   Motors    │       │
│  │  (H264)     │   │  (GStreamer)│   │ (Dynamixel) │       │
│  └─────────────┘   └─────────────┘   └─────────────┘       │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

## Components

### 1. OpenAI Realtime Client (`pkg/realtime/client.go`)

WebSocket client for OpenAI's Realtime API:
- Native speech-to-speech (no separate STT/TTS)
- Server-side VAD (voice activity detection)
- Automatic turn detection
- Function calling during conversation

### 2. Audio Player (`pkg/realtime/audio.go`)

Streaming audio playback to robot:
- Receives PCM16 audio chunks
- Resamples from 24kHz (OpenAI) to 48kHz (robot)
- Streams via SSH + GStreamer
- Low-latency playback

### 3. Tools (`pkg/realtime/tools.go`)

Eva's available tools:
- `move_head` - Look in a direction
- `express_emotion` - Happy, curious, excited, confused, sad, surprised
- `wave_hello` - Greeting gesture
- `remember_person` - Store facts about people
- `recall_person` - Retrieve facts about people
- `look_around` - Scan the room
- `nod_yes` / `shake_head_no` - Agreement/disagreement

### 4. Memory System

Persistent memory for people:
```go
type Memory struct {
    People map[string]*PersonMemory
}

type PersonMemory struct {
    Name     string
    Facts    []string
    LastSeen time.Time
}
```

## Usage

### Prerequisites

1. OpenAI API key with Realtime API access
2. Robot running with WebRTC enabled

### Running

```bash
export OPENAI_API_KEY="sk-..."
cd go-reachy
go build -o eva ./cmd/eva/
./eva
```

### Output

```
🤖 Eva 2.0 - Low-Latency Conversational Agent
==============================================
🔧 Initializing... ✅
🤖 Waking up Eva... ✅
📹 Connecting to camera/microphone... ✅
🧠 Connecting to OpenAI Realtime API... ✅
⚙️  Configuring Eva's personality... ✅

🎤 Eva is listening! Speak to start a conversation...
   (Ctrl+C to exit)

👤 User: Hello Eva!
🗣️  Eva: [response complete]
```

## OpenAI Realtime API

### Message Types (Client → Server)

| Type | Description |
|------|-------------|
| `session.update` | Configure session (voice, tools, etc.) |
| `input_audio_buffer.append` | Send audio chunks |
| `input_audio_buffer.commit` | Trigger processing |
| `input_audio_buffer.clear` | Clear buffer |
| `response.cancel` | Interrupt current response |

### Message Types (Server → Client)

| Type | Description |
|------|-------------|
| `session.created` | Session ready |
| `input_audio_buffer.speech_started` | User started speaking |
| `input_audio_buffer.speech_stopped` | User stopped speaking |
| `response.audio.delta` | Streaming audio chunk |
| `response.audio.done` | Audio complete |
| `response.function_call_arguments.done` | Function call ready |

### Audio Format

- **Input**: PCM16, 24kHz, mono
- **Output**: PCM16, 24kHz, mono
- Base64 encoded in JSON messages

## Tool Calling Flow

1. User speaks → API detects function call intent
2. API sends `response.function_call_arguments.done`
3. Eva executes the tool (e.g., move head)
4. Eva sends result back via `conversation.item.create`
5. Eva sends `response.create` to continue
6. API generates response incorporating tool result

## Configuration

### Voice Options

- `alloy` - Neutral
- `echo` - Male  
- `fable` - British
- `onyx` - Deep male
- `nova` - Female (recommended for Eva)
- `shimmer` - Soft female

### Turn Detection

```go
"turn_detection": {
    "type": "server_vad",
    "threshold": 0.5,           // Voice detection sensitivity
    "prefix_padding_ms": 300,   // Audio before speech
    "silence_duration_ms": 500, // Silence to end turn
}
```

## Future Improvements

1. **Vision integration** - Use camera for person detection
2. **Wake word** - "Hey Eva" local detection
3. **Echo cancellation** - Better handling of robot's own speech
4. **Persistent memory** - Save/load memory to disk
5. **Emotion detection** - Detect user emotions from voice

## Troubleshooting

### "Failed to connect to Realtime API"
- Check OpenAI API key has Realtime API access
- Verify network connectivity

### "WebRTC connection failed"  
- See `docs/TROUBLESHOOTING.md` for robot startup

### Audio not playing
- Check GStreamer is installed on robot
- Verify SSH connection works

## References

- [OpenAI Realtime API Guide](https://platform.openai.com/docs/guides/realtime)
- [Realtime API Reference](https://platform.openai.com/docs/api-reference/realtime)







