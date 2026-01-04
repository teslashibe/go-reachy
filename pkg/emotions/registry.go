package emotions

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// Registry manages a collection of emotions and provides playback.
type Registry struct {
	mu       sync.RWMutex
	emotions map[string]*Emotion
	player   *Player
	callback PlayerCallback
}

// NewRegistry creates a new emotion registry.
func NewRegistry() *Registry {
	return &Registry{
		emotions: make(map[string]*Emotion),
		player:   NewPlayer(),
	}
}

// LoadBuiltIn loads all embedded emotions into the registry.
func (r *Registry) LoadBuiltIn() error {
	names, err := ListEmbedded()
	if err != nil {
		return fmt.Errorf("failed to list embedded emotions: %w", err)
	}

	for _, name := range names {
		emotion, err := LoadEmbedded(name)
		if err != nil {
			return fmt.Errorf("failed to load emotion %q: %w", name, err)
		}
		r.Register(emotion)
	}

	return nil
}

// LoadCustomDir loads emotions from a custom directory.
func (r *Registry) LoadCustomDir(dir string) error {
	emotions, err := LoadFromDirectory(dir)
	if err != nil {
		return err
	}

	for _, emotion := range emotions {
		r.Register(emotion)
	}

	return nil
}

// Register adds an emotion to the registry.
func (r *Registry) Register(emotion *Emotion) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.emotions[emotion.Name] = emotion
}

// Unregister removes an emotion from the registry.
func (r *Registry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.emotions, name)
}

// Get retrieves an emotion by name.
func (r *Registry) Get(name string) (*Emotion, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	emotion, ok := r.emotions[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	return emotion, nil
}

// List returns all registered emotion names, sorted alphabetically.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.emotions))
	for name := range r.emotions {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ListWithDescriptions returns all emotions with their descriptions.
func (r *Registry) ListWithDescriptions() map[string]string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]string, len(r.emotions))
	for name, emotion := range r.emotions {
		result[name] = emotion.Description
	}
	return result
}

// Count returns the number of registered emotions.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.emotions)
}

// SetCallback sets the callback function for playback.
// This should be set before calling Play.
func (r *Registry) SetCallback(cb PlayerCallback) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.callback = cb
}

// Play starts playing an emotion by name.
// Returns immediately; playback happens in a goroutine.
// Use the callback set by SetCallback to receive pose updates.
func (r *Registry) Play(ctx context.Context, name string) error {
	emotion, err := r.Get(name)
	if err != nil {
		return err
	}

	r.mu.RLock()
	cb := r.callback
	r.mu.RUnlock()

	if cb == nil {
		return fmt.Errorf("no callback set; call SetCallback first")
	}

	go func() {
		_ = r.player.Play(ctx, emotion, cb)
	}()

	return nil
}

// PlaySync plays an emotion and blocks until complete.
func (r *Registry) PlaySync(ctx context.Context, name string) error {
	emotion, err := r.Get(name)
	if err != nil {
		return err
	}

	r.mu.RLock()
	cb := r.callback
	r.mu.RUnlock()

	if cb == nil {
		return fmt.Errorf("no callback set; call SetCallback first")
	}

	return r.player.Play(ctx, emotion, cb)
}

// PlayWithOptions plays an emotion with custom options.
func (r *Registry) PlayWithOptions(ctx context.Context, name string, opts PlayerOptions) error {
	emotion, err := r.Get(name)
	if err != nil {
		return err
	}

	r.mu.RLock()
	cb := r.callback
	r.mu.RUnlock()

	if cb == nil {
		return fmt.Errorf("no callback set; call SetCallback first")
	}

	go func() {
		_ = r.player.PlayWithOptions(ctx, emotion, cb, opts)
	}()

	return nil
}

// Stop halts the currently playing emotion.
func (r *Registry) Stop() {
	r.player.Stop()
}

// Pause pauses the currently playing emotion.
func (r *Registry) Pause() {
	r.player.Pause()
}

// Resume resumes a paused emotion.
func (r *Registry) Resume() {
	r.player.Resume()
}

// State returns the current playback state.
func (r *Registry) State() PlaybackState {
	return r.player.State()
}

// IsPlaying returns true if an emotion is currently playing.
func (r *Registry) IsPlaying() bool {
	return r.player.State() == StatePlaying
}

// CurrentEmotion returns the name of the currently playing emotion.
func (r *Registry) CurrentEmotion() string {
	if e := r.player.CurrentEmotion(); e != nil {
		return e.Name
	}
	return ""
}

// Categories groups emotions by common prefixes for easier browsing.
func (r *Registry) Categories() map[string][]string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	categories := make(map[string][]string)

	for name := range r.emotions {
		// Extract category from name (e.g., "yes1" -> "yes", "dance3" -> "dance")
		category := extractCategory(name)
		categories[category] = append(categories[category], name)
	}

	// Sort each category
	for cat := range categories {
		sort.Strings(categories[cat])
	}

	return categories
}

// extractCategory gets the base name without trailing numbers.
func extractCategory(name string) string {
	// Find where trailing digits start
	i := len(name)
	for i > 0 && name[i-1] >= '0' && name[i-1] <= '9' {
		i--
	}
	if i == 0 {
		return name
	}
	return name[:i]
}

// Search finds emotions matching a query (case-insensitive substring match).
func (r *Registry) Search(query string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var matches []string
	for name, emotion := range r.emotions {
		if containsIgnoreCase(name, query) || containsIgnoreCase(emotion.Description, query) {
			matches = append(matches, name)
		}
	}
	sort.Strings(matches)
	return matches
}

func containsIgnoreCase(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && containsIgnoreCaseHelper(s, substr)))
}

func containsIgnoreCaseHelper(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			c1, c2 := s[i+j], substr[j]
			if c1 >= 'A' && c1 <= 'Z' {
				c1 += 32
			}
			if c2 >= 'A' && c2 <= 'Z' {
				c2 += 32
			}
			if c1 != c2 {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

