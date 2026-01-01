package tracking

import (
	"sync"
	"time"
)

// TrackedEntity represents a person or object being tracked in world coordinates
type TrackedEntity struct {
	ID            string    // Unique identifier
	WorldAngle    float64   // Position in world coords (radians from Eva's forward)
	Confidence    float64   // 0-1, decays over time when not seen
	LastSeen      time.Time // When last detected
	LastPosition  float64   // Previous frame position (for velocity)
	Velocity      float64   // Estimated angular velocity (rad/sec)
	FramePosition float64   // Last known position in frame (0-100)
}

// WorldModel maintains a spatial map of tracked entities
type WorldModel struct {
	entities    map[string]*TrackedEntity
	focusTarget string // ID of entity Eva is focusing on
	mu          sync.RWMutex

	// Configuration
	confidenceDecay float64       // How fast confidence decays per second
	forgetThreshold float64       // Remove entities below this confidence
	forgetTimeout   time.Duration // Remove entities not seen for this long
}

// NewWorldModel creates a new world model
func NewWorldModel() *WorldModel {
	return &WorldModel{
		entities:        make(map[string]*TrackedEntity),
		confidenceDecay: 0.3,            // Lose 30% confidence per second
		forgetThreshold: 0.1,            // Forget below 10% confidence
		forgetTimeout:   10 * time.Second, // Forget after 10 seconds
	}
}

// UpdateEntity updates or creates an entity based on a detection
func (w *WorldModel) UpdateEntity(id string, worldAngle float64, framePosition float64) {
	w.mu.Lock()
	defer w.mu.Unlock()

	now := time.Now()

	if entity, exists := w.entities[id]; exists {
		// Calculate velocity based on position change
		dt := now.Sub(entity.LastSeen).Seconds()
		if dt > 0 && dt < 1.0 { // Only calculate if reasonable time delta
			entity.Velocity = (worldAngle - entity.WorldAngle) / dt
		}

		// Update with smoothing (weighted average of old and new)
		smoothing := 0.7 // Weight of new reading
		entity.WorldAngle = smoothing*worldAngle + (1-smoothing)*entity.WorldAngle
		entity.FramePosition = framePosition
		entity.LastSeen = now
		entity.Confidence = 1.0
	} else {
		// New entity
		w.entities[id] = &TrackedEntity{
			ID:            id,
			WorldAngle:    worldAngle,
			Confidence:    1.0,
			LastSeen:      now,
			FramePosition: framePosition,
			Velocity:      0,
		}
		// Auto-focus on first entity
		if w.focusTarget == "" {
			w.focusTarget = id
		}
	}
}

// GetFocusTarget returns the entity Eva should be looking at
func (w *WorldModel) GetFocusTarget() *TrackedEntity {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.focusTarget == "" {
		return nil
	}

	entity, exists := w.entities[w.focusTarget]
	if !exists || entity.Confidence < w.forgetThreshold {
		return nil
	}

	return entity
}

// GetTargetWorldAngle returns the world angle Eva should look at
// If no target, returns (0, false) to indicate no valid target
func (w *WorldModel) GetTargetWorldAngle() (float64, bool) {
	entity := w.GetFocusTarget()
	if entity == nil {
		return 0, false
	}

	// Only predict if recently seen (within 500ms)
	dt := time.Since(entity.LastSeen).Seconds()
	if dt > 0.5 {
		// Face lost for too long - just return last known position
		// Don't predict, as velocity estimation may be wrong
		return entity.WorldAngle, true
	}

	// Predict current position based on velocity (only for very recent detections)
	predictedAngle := entity.WorldAngle + entity.Velocity*dt*0.5 // Dampen prediction

	return predictedAngle, true
}

// DecayConfidence reduces confidence of all entities over time
func (w *WorldModel) DecayConfidence(dt float64) {
	w.mu.Lock()
	defer w.mu.Unlock()

	toDelete := []string{}

	for id, entity := range w.entities {
		// Decay confidence
		entity.Confidence -= w.confidenceDecay * dt
		if entity.Confidence < 0 {
			entity.Confidence = 0
		}

		// Mark for deletion if forgotten
		if entity.Confidence < w.forgetThreshold ||
			time.Since(entity.LastSeen) > w.forgetTimeout {
			toDelete = append(toDelete, id)
		}
	}

	// Delete forgotten entities
	for _, id := range toDelete {
		delete(w.entities, id)
		if w.focusTarget == id {
			w.focusTarget = ""
			// Try to focus on another entity
			for newID := range w.entities {
				w.focusTarget = newID
				break
			}
		}
	}
}

// HasTarget returns true if there's a valid target to track
func (w *WorldModel) HasTarget() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()

	entity, exists := w.entities[w.focusTarget]
	return exists && entity.Confidence >= w.forgetThreshold
}

// GetAllEntities returns a copy of all tracked entities
func (w *WorldModel) GetAllEntities() []*TrackedEntity {
	w.mu.RLock()
	defer w.mu.RUnlock()

	result := make([]*TrackedEntity, 0, len(w.entities))
	for _, entity := range w.entities {
		// Create a copy
		copy := *entity
		result = append(result, &copy)
	}
	return result
}

// SetFocusTarget sets which entity Eva should focus on
func (w *WorldModel) SetFocusTarget(id string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.focusTarget = id
}

// Clear removes all tracked entities
func (w *WorldModel) Clear() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.entities = make(map[string]*TrackedEntity)
	w.focusTarget = ""
}

