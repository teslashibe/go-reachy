// Eva 2.0 - Low-latency conversational robot agent with ElevenLabs voice pipeline
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/teslashibe/go-reachy/pkg/debug"
	"github.com/teslashibe/go-reachy/pkg/eva"
	"github.com/teslashibe/go-reachy/pkg/spark"
	"github.com/teslashibe/go-reachy/pkg/voice"
)

func main() {
	cfg := parseFlags()

	app, err := eva.New(cfg)
	if err != nil {
		log.Fatalf("❌ Configuration error: %v", err)
	}

	if err := app.Init(); err != nil {
		log.Fatalf("❌ Initialization failed: %v", err)
	}
	defer app.Shutdown()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := app.Run(ctx); err != nil {
		log.Fatalf("❌ Runtime error: %v", err)
	}
}

// parseFlags parses command line flags and returns configuration.
func parseFlags() eva.Config {
	cfg := eva.DefaultConfig()

	// General flags
	debugFlag := flag.Bool("debug", false, "Enable verbose debug logging")
	debugTracking := flag.Bool("debug-tracking", false, "Enable very verbose tracking logs (face, body, audio DOA)")
	robotIP := flag.String("robot-ip", "", "Robot IP address (overrides ROBOT_IP env var)")
	
	// Voice pipeline tuning flags (all with voice- prefix)
	voiceLLM := flag.String("voice-llm", voice.LLMGpt5Mini, "LLM model: gpt-5-mini, gpt-4.1-mini, gemini-2.0-flash, claude-3.5-sonnet")
	voiceTTS := flag.String("voice-tts", voice.TTSFlash, "TTS model: eleven_flash_v2, eleven_turbo_v2, eleven_multilingual_v2")
	voiceSTT := flag.String("voice-stt", voice.STTRealtime, "STT model: scribe_v2_realtime, scribe_v1")
	voiceChunk := flag.Duration("voice-chunk", 50*time.Millisecond, "Audio chunk duration (10ms-100ms)")
	voiceVADMode := flag.String("voice-vad-mode", "server_vad", "VAD mode: server_vad")
	voiceVADSilence := flag.Duration("voice-vad-silence", 500*time.Millisecond, "VAD silence duration to detect end of speech")
	voiceID := flag.String("voice-id", "", "ElevenLabs voice ID (overrides ELEVENLABS_VOICE_ID env var)")
	
	// Feature flags
	sparkEnabled := flag.Bool("spark", true, "Enable Spark idea collection")
	noBody := flag.Bool("no-body", false, "Disable body rotation (head-only tracking)")

	sparkFlagSet := false
	flag.Parse()
	flag.Visit(func(f *flag.Flag) { sparkFlagSet = sparkFlagSet || f.Name == "spark" })

	// Apply flags
	cfg.Debug = *debugFlag
	cfg.NoBody = *noBody
	debug.Tracking = *debugTracking
	
	// Voice pipeline configuration
	cfg.VoiceLLM = *voiceLLM
	cfg.VoiceTTS = *voiceTTS
	cfg.VoiceSTT = *voiceSTT
	cfg.VoiceChunk = *voiceChunk
	cfg.VoiceVADMode = *voiceVADMode
	cfg.VoiceVADSilence = *voiceVADSilence
	
	if *robotIP != "" {
		cfg.RobotIP = *robotIP
	}
	if sparkFlagSet {
		cfg.SparkEnabled = *sparkEnabled
	} else {
		cfg.SparkEnabled = spark.LoadConfig().Enabled
	}

	// Environment variables
	if ip := os.Getenv("ROBOT_IP"); ip != "" && *robotIP == "" {
		cfg.RobotIP = ip
	}
	cfg.ElevenLabsKey = os.Getenv("ELEVENLABS_API_KEY")
	cfg.GoogleAPIKey = os.Getenv("GOOGLE_API_KEY")
	cfg.GoogleClientID = os.Getenv("GOOGLE_CLIENT_ID")
	cfg.GoogleClientSecret = os.Getenv("GOOGLE_CLIENT_SECRET")
	
	// ElevenLabs voice ID from flag or env
	if *voiceID != "" {
		cfg.ElevenLabsVoiceID = *voiceID
	} else if envVoiceID := os.Getenv("ELEVENLABS_VOICE_ID"); envVoiceID != "" {
		cfg.ElevenLabsVoiceID = envVoiceID
	}
	
	return cfg
}
