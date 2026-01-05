// Eva 2.0 - Low-latency conversational robot agent with tool use
// Uses OpenAI Realtime API for speech-to-speech conversation
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/teslashibe/go-reachy/pkg/eva"
	"github.com/teslashibe/go-reachy/pkg/spark"
	"github.com/teslashibe/go-reachy/pkg/tts"
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

	debug := flag.Bool("debug", false, "Enable verbose debug logging")
	robotIP := flag.String("robot-ip", "", "Robot IP address (overrides ROBOT_IP env var)")
	ttsMode := flag.String("tts", cfg.TTSMode, "TTS provider: realtime, elevenlabs, elevenlabs-streaming, openai-tts")
	ttsVoice := flag.String("tts-voice", "", "Voice ID for ElevenLabs")
	sparkEnabled := flag.Bool("spark", true, "Enable Spark idea collection")
	noBody := flag.Bool("no-body", false, "Disable body rotation (head-only tracking)")

	sparkFlagSet := false
	flag.Parse()
	flag.Visit(func(f *flag.Flag) { sparkFlagSet = sparkFlagSet || f.Name == "spark" })

	cfg.Debug, cfg.TTSMode, cfg.NoBody = *debug, *ttsMode, *noBody
	if *robotIP != "" {
		cfg.RobotIP = *robotIP
	}
	if *ttsVoice != "" {
		cfg.TTSVoice = *ttsVoice
	} else if cfg.TTSVoice == "" {
		cfg.TTSVoice = tts.DefaultElevenLabsVoice
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
	cfg.OpenAIKey = os.Getenv("OPENAI_API_KEY")
	cfg.ElevenLabsKey = os.Getenv("ELEVENLABS_API_KEY")
	cfg.GoogleAPIKey = os.Getenv("GOOGLE_API_KEY")
	cfg.GoogleClientID = os.Getenv("GOOGLE_CLIENT_ID")
	cfg.GoogleClientSecret = os.Getenv("GOOGLE_CLIENT_SECRET")
	if *ttsVoice == "" {
		if voice := os.Getenv("ELEVENLABS_VOICE_ID"); voice != "" {
			cfg.TTSVoice = voice
		}
	}
	return cfg
}
