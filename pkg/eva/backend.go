package eva

// Backend represents a conversational AI backend (OpenAI Realtime, Gemini Live, etc.)
// The interface is defined where it's consumed (idiomatic Go), not where implemented.
// Existing openai.Client already satisfies this interface.
type Backend interface {
	// Connect establishes connection to the backend API.
	Connect() error

	// Close terminates the connection.
	Close()

	// SendAudio sends PCM16 audio data to the backend.
	SendAudio(pcm16Data []byte) error

	// IsConnected returns true if the connection is established.
	IsConnected() bool

	// IsReady returns true if the session is ready for conversation.
	IsReady() bool

	// ConfigureSession sets up the session with instructions and voice.
	ConfigureSession(instructions, voice string) error

	// RegisterTool registers a tool the AI can invoke.
	RegisterTool(tool Tool)

	// CancelResponse interrupts the current AI response.
	CancelResponse() error
}

// BackendCallbacks are callbacks the backend invokes during conversation.
// Set these on the backend implementation before calling Connect().
type BackendCallbacks struct {
	// OnTranscript is called with transcript text.
	// isFinal=true for user's completed speech, isFinal=false for streaming AI response.
	OnTranscript func(text string, isFinal bool)

	// OnTranscriptDone is called when the AI's transcript is complete.
	OnTranscriptDone func()

	// OnAudioDelta is called with base64-encoded audio chunks.
	OnAudioDelta func(audioBase64 string)

	// OnAudioDone is called when audio streaming is complete.
	OnAudioDone func()

	// OnSpeechStarted is called when user starts speaking (VAD).
	OnSpeechStarted func()

	// OnSpeechStopped is called when user stops speaking (VAD).
	OnSpeechStopped func()

	// OnSessionCreated is called when the session is established.
	OnSessionCreated func()

	// OnError is called on errors.
	OnError func(err error)
}

