// Package tts voice presets for ElevenLabs.
package tts

// ElevenLabsVoices maps friendly preset names to ElevenLabs voice IDs.
// Use ResolveElevenLabsVoice to look up a voice by name or pass through raw IDs.
var ElevenLabsVoices = map[string]string{
	"charlotte": "XB0fDUnXU5powFXDhCwa", // British female, warm
	"aria":      "9BWtsMINqrJLrRacOk9x", // American female, expressive
	"sarah":     "EXAVITQu4vr4xnSDxMaL", // American female, soft
	"lily":      "pFZP5JQG7iQjIQuC4Bku", // British female, warm
	"rachel":    "21m00Tcm4TlvDq8ikWAM", // American female, calm
	"domi":      "AZnzlk1XvdvUeBnXmlld", // American female, strong
	"bella":     "EXAVITQu4vr4xnSDxMaL", // American female, soft
	"elli":      "MF3mGyEYCl7XYWbV9V6O", // American female, young
	"josh":      "TxGEqnHWrfWFTfGW9XjX", // American male, deep
	"adam":      "pNInz6obpgDQGcFmaJgB", // American male, deep
	"sam":       "yoZ06aMxZJJ28mfd3POQ", // American male, raspy
}

// DefaultElevenLabsVoice is the default voice preset.
const DefaultElevenLabsVoice = "charlotte"

// ResolveElevenLabsVoice returns the voice ID for a preset name,
// or the input unchanged if it's already a voice ID.
func ResolveElevenLabsVoice(name string) string {
	if id, ok := ElevenLabsVoices[name]; ok {
		return id
	}
	return name // Assume it's already a voice ID
}

// IsElevenLabsPreset returns true if the name is a known preset.
func IsElevenLabsPreset(name string) bool {
	_, ok := ElevenLabsVoices[name]
	return ok
}

