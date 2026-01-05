// Package voice provides a unified interface for voice conversation pipelines.
//
// The voice package abstracts different speech-to-speech providers behind a common
// Pipeline interface, enabling easy switching between providers and consistent
// latency measurement across all implementations.
//
// # Supported Providers
//
// The package supports three bundled providers, each offering different tradeoffs:
//
//   - OpenAI Realtime: GPT-4o with built-in TTS (~300-500ms latency)
//   - ElevenLabs Conversational: Custom cloned voices with choice of LLM (~200-400ms)
//   - Gemini Live: Google's native speech-to-speech API (~150-300ms)
//
// # Usage
//
// Create a pipeline using one of the bundled providers:
//
//	import "github.com/teslashibe/go-reachy/pkg/voice"
//
//	// Create pipeline with default config
//	pipeline, err := voice.New("openai", voice.DefaultConfig())
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Register tools
//	pipeline.RegisterTool(voice.Tool{
//	    Name:        "describe_scene",
//	    Description: "Describes what the robot sees",
//	    Handler: func(args map[string]any) (string, error) {
//	        return "I see a person waving", nil
//	    },
//	})
//
//	// Wire callbacks
//	pipeline.OnAudioOut(func(pcm []byte) {
//	    speaker.Write(pcm)
//	})
//
//	pipeline.OnTranscript(func(text string, final bool) {
//	    fmt.Printf("User said: %s\n", text)
//	})
//
//	// Start the pipeline
//	if err := pipeline.Start(ctx); err != nil {
//	    log.Fatal(err)
//	}
//	defer pipeline.Stop()
//
//	// Stream audio from microphone
//	for audio := range microphone {
//	    pipeline.SendAudio(audio)
//	}
//
// # Latency Metrics
//
// All pipelines track per-stage latency metrics:
//
//	metrics := pipeline.Metrics()
//	fmt.Printf("VAD: %dms, ASR: %dms, LLM: %dms, TTS: %dms, Total: %dms\n",
//	    metrics.VADLatency.Milliseconds(),
//	    metrics.ASRLatency.Milliseconds(),
//	    metrics.LLMFirstToken.Milliseconds(),
//	    metrics.TTSFirstAudio.Milliseconds(),
//	    metrics.TotalLatency.Milliseconds(),
//	)
//
// # Configuration
//
// All pipelines support runtime configuration updates:
//
//	cfg := pipeline.Config()
//	cfg.VADThreshold = 0.8
//	cfg.VADSilenceDuration = 500 * time.Millisecond
//	pipeline.UpdateConfig(cfg)
package voice

