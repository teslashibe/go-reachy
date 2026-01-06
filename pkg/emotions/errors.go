package emotions

import "errors"

var (
	// ErrNotFound is returned when an emotion is not found.
	ErrNotFound = errors.New("emotion not found")

	// ErrAlreadyPlaying is returned when trying to play while already playing.
	ErrAlreadyPlaying = errors.New("emotion already playing")

	// ErrInvalidEmotion is returned when an emotion file is malformed.
	ErrInvalidEmotion = errors.New("invalid emotion data")

	// ErrRegistryNotInitialized is returned when using registry before Init().
	ErrRegistryNotInitialized = errors.New("emotion registry not initialized")
)


