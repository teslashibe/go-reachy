//go:build ignore

// NOTE: This file is excluded from the build. It contains an incomplete
// experimental speech handler that depends on robot.Reachy which is also excluded.

package speech

import (
	"context"
	"log"

	"github.com/teslashibe/go-reachy/pkg/robot"
)

// Handler manages speech recognition and synthesis
type Handler struct {
	apiKey string
	robot  *robot.Reachy
}

// NewHandler creates a new speech handler
func NewHandler(apiKey string, r *robot.Reachy) *Handler {
	return &Handler{
		apiKey: apiKey,
		robot:  r,
	}
}

// Run starts the speech processing loop
func (h *Handler) Run(ctx context.Context) {
	log.Println("Speech handler started")
	
	// TODO: Implement OpenAI Realtime API WebSocket connection
	// This would:
	// 1. Connect to OpenAI's Realtime API via WebSocket
	// 2. Stream audio from microphone
	// 3. Receive transcription and AI responses
	// 4. Play audio responses
	// 5. Trigger robot actions based on AI tool calls

	// For now, just wait for context cancellation
	<-ctx.Done()
	log.Println("Speech handler stopped")
}

// ProcessCommand handles a voice command and returns a response
func (h *Handler) ProcessCommand(command string) (string, error) {
	// TODO: Implement command processing
	// This would send the command to OpenAI and handle tool calls
	
	// Example tool handling:
	// - "dance" -> h.robot.Dance("random")
	// - "be happy" -> h.robot.PlayEmotion("happy")
	// - "look left" -> h.robot.SetHead(...)
	
	return "Command received: " + command, nil
}

