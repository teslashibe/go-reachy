# Voice Package

The `voice` package provides a unified interface for real-time voice AI pipelines, supporting multiple providers with configurable TTS (text-to-speech) and STT (speech-to-text) models.

## Supported Providers

| Provider | Description | Latency |
|----------|-------------|---------|
| **OpenAI** | GPT-4o Realtime API (native speech-to-speech) | ~400-600ms |
| **ElevenLabs** | Conversational AI with customizable LLM + voice | ~1.2-1.6s |
| **Gemini** | Google's Gemini Live API (native multimodal) | ~300-500ms |

## ElevenLabs Configuration

### LLM Models (Benchmarked Results)

Based on TTFA (Time To First Audio) benchmarks with 5 loops each:

| Rank | Model | Avg TTFA | Notes |
|------|-------|----------|-------|
| 🥇 | `gpt-5-mini` | **1.25s** | Fastest overall |
| 🥈 | `gpt-4.1-mini` | 1.26s | Great balance |
| 🥉 | `gpt-4.1` | 1.27s | High quality |
| 4 | `gemini-2.0-flash` | 1.28s | Most consistent (70ms variance) |
| 5 | `claude-3.5-sonnet` | 1.28s | Good for complex tasks |
| 6 | `gpt-4o-mini` | 1.28s | Reliable |
| 7 | `claude-haiku-4.5` | 1.30s | Fast Claude option |
| 8 | `qwen3-30b-a3b` | 1.30s | Open-source alternative |

### TTS Models (Text-to-Speech)

| Model ID | Latency | Languages | Use Case |
|----------|---------|-----------|----------|
| `eleven_flash_v2_5` | **~75ms** ⚡ | 32 | Real-time agents, lowest latency |
| `eleven_flash_v2` | ~75ms | English | English-only, fast |
| `eleven_turbo_v2_5` | ~250-300ms | 32 | Balanced quality/speed |
| `eleven_turbo_v2` | ~250-300ms | English | English-only, balanced |
| `eleven_multilingual_v2` | ~400ms+ | 29 | Best quality, emotional range |
| `eleven_v3` | Higher | 70+ | Most expressive (alpha) |

**Specialty Models:**
- `eleven_multilingual_sts_v2` - Speech-to-speech (29 languages)
- `eleven_english_sts_v2` - Speech-to-speech (English)
- `eleven_multilingual_ttv_v2` - Text-to-voice design
- `eleven_ttv_v3` - Text-to-voice design v3

### STT Models (Speech-to-Text)

| Model ID | Latency | Languages | Use Case |
|----------|---------|-----------|----------|
| `scribe_v2_realtime` | **~150ms** ⚡ | 90 | Real-time transcription |
| `scribe_v1` | ~300ms | 99 | Higher accuracy, more languages |

## Optimal Configuration

For the **fastest response time**, use:

```go
cfg := voice.DefaultElevenLabsConfig()
cfg.ElevenLabsKey = os.Getenv("ELEVENLABS_API_KEY")
cfg.ElevenLabsVoiceID = "21m00Tcm4TlvDq8ikWAM" // Rachel voice
cfg.LLMModel = "gpt-5-mini"                     // Fastest LLM (1.25s TTFA)
cfg.TTSModel = voice.ElevenLabsTTSFlashV2       // eleven_flash_v2 (~75ms, English)
cfg.STTModel = voice.ElevenLabsSTTRealtime      // scribe_v2_realtime (~150ms)
```

> **Note:** ElevenLabs Agents requires `eleven_flash_v2` or `eleven_turbo_v2` (v2, not v2.5) for English-only agents. Use v2.5 models for multilingual agents.

**Benchmark Results (5 loops):**
- Average TTFA: **1.28s**
- Minimum TTFA: 1.20s
- Maximum TTFA: 1.36s
- Variance: 162ms

**Expected latency breakdown:**
- STT: ~150ms
- LLM: ~800-1000ms  
- TTS: ~75ms
- **Total TTFA: ~1.2-1.3s**

## Usage Example

```go
package main

import (
    "context"
    "os"
    
    "github.com/teslashibe/go-reachy/pkg/voice"
)

func main() {
    // Create optimal config for ElevenLabs
    cfg := voice.DefaultElevenLabsConfig()
    cfg.ElevenLabsKey = os.Getenv("ELEVENLABS_API_KEY")
    cfg.ElevenLabsVoiceID = os.Getenv("ELEVENLABS_VOICE_ID")
    cfg.LLMModel = "gpt-5-mini"
    cfg.TTSModel = voice.ElevenLabsTTSFlash
    cfg.STTModel = voice.ElevenLabsSTTRealtime
    cfg.SystemPrompt = "You are Eva, a helpful robot assistant."

    // Create pipeline
    pipeline, err := voice.New(voice.ProviderElevenLabs, cfg)
    if err != nil {
        panic(err)
    }

    // Set up callbacks
    pipeline.OnAudioOut(func(pcm16 []byte) {
        // Play audio to speaker
    })

    pipeline.OnTranscript(func(text string, isFinal bool) {
        // Handle transcription
    })

    // Start the pipeline
    ctx := context.Background()
    if err := pipeline.Start(ctx); err != nil {
        panic(err)
    }
    defer pipeline.Stop()

    // Send audio
    pipeline.SendAudio(audioBytes)
}
```

## Constants Reference

```go
// TTS Models
voice.ElevenLabsTTSFlash         // "eleven_flash_v2_5" - Fastest
voice.ElevenLabsTTSFlashV2       // "eleven_flash_v2" - Fast, English
voice.ElevenLabsTTSTurbo         // "eleven_turbo_v2_5" - Balanced
voice.ElevenLabsTTSTurboV2       // "eleven_turbo_v2" - Balanced, English
voice.ElevenLabsTTSMultilingual  // "eleven_multilingual_v2" - Best quality
voice.ElevenLabsTTSV3            // "eleven_v3" - Most expressive

// STT Models
voice.ElevenLabsSTTRealtime      // "scribe_v2_realtime" - Fastest
voice.ElevenLabsSTTV1            // "scribe_v1" - More accurate
```

## Running Benchmarks

```bash
# Test all ElevenLabs LLM models
go run ./cmd/test-voice --elevenlabs-models --loops 5

# Test optimal configuration
go run ./cmd/test-voice --optimal --loops 5

# Test specific provider
go run ./cmd/test-voice --provider elevenlabs --loops 3
```

## Environment Variables

| Variable | Description |
|----------|-------------|
| `ELEVENLABS_API_KEY` | ElevenLabs API key |
| `ELEVENLABS_VOICE_ID` | Voice ID (e.g., `21m00Tcm4TlvDq8ikWAM` for Rachel) |
| `OPENAI_API_KEY` | OpenAI API key (for OpenAI provider) |
| `GOOGLE_API_KEY` | Google API key (for Gemini provider) |

## Latency Optimization Tips

1. **Use Flash TTS** (`eleven_flash_v2_5`) - saves ~200-400ms vs other models
2. **Use Realtime STT** (`scribe_v2_realtime`) - ~150ms faster than v1
3. **Choose fast LLM** - `gpt-5-mini` or `gemini-2.0-flash` are fastest
4. **Reduce chunk size** - 50ms chunks can reduce perceived latency
5. **Use semantic VAD** - Better turn detection than silence-based

## Architecture

```
┌─────────────┐   ┌─────────────┐   ┌─────────────┐   ┌─────────────┐
│   Audio     │ → │     STT     │ → │     LLM     │ → │     TTS     │
│   Input     │   │ (Scribe v2) │   │ (gpt-5-mini)│   │  (Flash v2) │
│             │   │   ~150ms    │   │  ~800-1000ms│   │    ~75ms    │
└─────────────┘   └─────────────┘   └─────────────┘   └─────────────┘
                                                              │
                                                              ▼
                                                      ┌─────────────┐
                                                      │   Audio     │
                                                      │   Output    │
                                                      └─────────────┘
```

**Total Pipeline Latency (TTFA): ~1.0-1.3s**

