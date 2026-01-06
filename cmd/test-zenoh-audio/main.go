//go:build ignore

// test-zenoh-audio is a test command for the Zenoh audio streaming feature.
// It demonstrates capturing audio from the local microphone and streaming
// it over Zenoh, as well as receiving audio and playing it locally.
//
// Usage:
//
//	go run ./cmd/test-zenoh-audio -endpoint tcp/localhost:7447 -mode pub
//	go run ./cmd/test-zenoh-audio -endpoint tcp/localhost:7447 -mode sub
//
// This command requires zenoh-c to be installed. For testing without
// zenoh-c, use the mock backend:
//
//	go run -tags mock ./cmd/test-zenoh-audio
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/teslashibe/go-reachy/pkg/audioio"
	"github.com/teslashibe/go-reachy/pkg/zenohclient"
)

func main() {
	endpoint := flag.String("endpoint", "tcp/localhost:7447", "Zenoh endpoint")
	mode := flag.String("mode", "pub", "Mode: pub (publish mic), sub (subscribe speaker), both")
	prefix := flag.String("prefix", "reachy_mini", "Zenoh topic prefix")
	backend := flag.String("audio-backend", "auto", "Audio backend: auto, mock, alsa, coreaudio")
	duration := flag.Duration("duration", 30*time.Second, "Test duration")
	flag.Parse()

	// Setup logging
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	ctx, cancel := context.WithTimeout(context.Background(), *duration)
	defer cancel()

	// Handle signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		logger.Info("received signal, shutting down")
		cancel()
	}()

	// Create Zenoh client
	zenohCfg := zenohclient.Config{
		Endpoint:          *endpoint,
		Mode:              "client",
		Prefix:            *prefix,
		ReconnectInterval: 2 * time.Second,
	}

	client, err := zenohclient.New(zenohCfg, logger)
	if err != nil {
		logger.Error("failed to create zenoh client", "error", err)
		os.Exit(1)
	}
	defer client.Close()

	logger.Info("connecting to Zenoh", "endpoint", *endpoint)

	if err := client.ConnectWithRetry(ctx); err != nil {
		logger.Error("failed to connect", "error", err)
		os.Exit(1)
	}

	logger.Info("connected to Zenoh")

	// Audio configuration
	audioCfg := audioio.DefaultConfig()
	switch *backend {
	case "mock":
		audioCfg.Backend = audioio.BackendMock
	case "alsa":
		audioCfg.Backend = audioio.BackendALSA
	case "coreaudio":
		audioCfg.Backend = audioio.BackendCoreAudio
	default:
		audioCfg.Backend = audioio.BackendAuto
	}

	switch *mode {
	case "pub":
		runPublisher(ctx, client, audioCfg, logger)
	case "sub":
		runSubscriber(ctx, client, audioCfg, logger)
	case "both":
		go runPublisher(ctx, client, audioCfg, logger)
		runSubscriber(ctx, client, audioCfg, logger)
	default:
		logger.Error("invalid mode", "mode", *mode)
		os.Exit(1)
	}
}

func runPublisher(ctx context.Context, client *zenohclient.Client, audioCfg audioio.Config, logger *slog.Logger) {
	// Create audio source
	source, err := audioio.NewSource(audioCfg, logger)
	if err != nil {
		logger.Error("failed to create audio source", "error", err)
		return
	}
	defer source.Close()

	logger.Info("audio source created", "backend", source.Name())

	// Create audio publisher
	pub, err := zenohclient.NewAudioPublisher(client, client.Topics().AudioMic(), logger)
	if err != nil {
		logger.Error("failed to create audio publisher", "error", err)
		return
	}
	defer pub.Close()

	// Start capturing
	if err := source.Start(ctx); err != nil {
		logger.Error("failed to start audio source", "error", err)
		return
	}

	logger.Info("publishing mic audio", "topic", client.Topics().AudioMic())

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	chunkCount := 0
	for {
		select {
		case <-ctx.Done():
			logger.Info("publisher stopped", "chunks_sent", chunkCount)
			return
		case <-ticker.C:
			stats := pub.Stats()
			logger.Info("publisher stats", "chunks_sent", stats.ChunksSent, "bytes_sent", stats.BytesSent)
		case chunk, ok := <-source.Stream():
			if !ok {
				return
			}

			if err := pub.PublishRaw(chunk.Samples, chunk.SampleRate, chunk.Channels); err != nil {
				logger.Debug("publish error", "error", err)
				continue
			}
			chunkCount++
		}
	}
}

func runSubscriber(ctx context.Context, client *zenohclient.Client, audioCfg audioio.Config, logger *slog.Logger) {
	// Create audio sink
	sink, err := audioio.NewSink(audioCfg, logger)
	if err != nil {
		logger.Error("failed to create audio sink", "error", err)
		return
	}
	defer sink.Close()

	logger.Info("audio sink created", "backend", sink.Name())

	// Create audio subscriber
	sub, err := zenohclient.NewAudioSubscriber(client, client.Topics().AudioSpeaker(), 50, logger)
	if err != nil {
		logger.Error("failed to create audio subscriber", "error", err)
		return
	}
	defer sub.Close()

	// Start playback
	if err := sink.Start(ctx); err != nil {
		logger.Error("failed to start audio sink", "error", err)
		return
	}

	logger.Info("subscribing to speaker audio", "topic", client.Topics().AudioSpeaker())

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	chunkCount := 0
	for {
		select {
		case <-ctx.Done():
			logger.Info("subscriber stopped", "chunks_received", chunkCount)
			return
		case <-ticker.C:
			stats := sub.Stats()
			logger.Info("subscriber stats",
				"chunks_received", stats.ChunksReceived,
				"bytes_received", stats.BytesReceived,
				"decode_errors", stats.DecodeErrors,
			)
		case chunk, ok := <-sub.Stream():
			if !ok {
				return
			}

			audioChunk := audioio.AudioChunk{
				Samples:    chunk.Samples,
				SampleRate: chunk.SampleRate,
				Channels:   chunk.Channels,
			}

			if err := sink.Write(ctx, audioChunk); err != nil {
				logger.Debug("playback error", "error", err)
				continue
			}
			chunkCount++
		}
	}
}

func init() {
	// Print available backends
	backends := audioio.AvailableBackends()
	fmt.Printf("Available audio backends: %v\n", backends)
}

