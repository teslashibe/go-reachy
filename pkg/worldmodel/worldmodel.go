package worldmodel

import (
	"sync"
	"time"
)

// WorldModel maintains a spatial map of tracked entities
type WorldModel struct {
	entities    map[string]*TrackedEntity
	focusTarget string // ID of entity Eva is focusing on
	mu          sync.RWMutex

	// Body orientation in room coordinates (radians)
	bodyYaw float64

	// Audio source (from go-eva DOA)
	audioSource *AudioSource

	// Detected objects (non-face detections)
	objects map[string]*DetectedObject

	// Configuration
	confidenceDecay float64       // How fast confidence decays per second
	forgetThreshold float64       // Remove entities below this confidence
	forgetTimeout   time.Duration // Remove entities not seen for this long
}

// New creates a new world model
func New() *WorldModel {
	return &WorldModel{
		entities:        make(map[string]*TrackedEntity),
		objects:         make(map[string]*DetectedObject),
		confidenceDecay: 0.3,              // Lose 30% confidence per second
		forgetThreshold: 0.1,              // Forget below 10% confidence
		forgetTimeout:   10 * time.Second, // Forget after 10 seconds
	}
}

// UpdateEntity updates or creates an entity based on a detection
func (w *WorldModel) UpdateEntity(id string, worldAngle float64, framePosition float64) {
	w.UpdateEntityWithDepth(id, worldAngle, framePosition, 0)
}

// UpdateEntityWithDepth updates or creates an entity with depth estimation.
// faceWidth is the normalized face width (0-1) used to estimate distance.
func (w *WorldModel) UpdateEntityWithDepth(id string, worldAngle float64, framePosition float64, faceWidth float64) {
	w.mu.Lock()
	defer w.mu.Unlock()

	now := time.Now()
	distance := EstimateDepth(faceWidth)

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
		entity.FaceWidth = faceWidth
		if distance > 0 {
			// Smooth distance updates
			if entity.Distance > 0 {
				entity.Distance = smoothing*distance + (1-smoothing)*entity.Distance
			} else {
				entity.Distance = distance
			}
		}
	} else {
		// New entity
		w.entities[id] = &TrackedEntity{
			ID:            id,
			WorldAngle:    worldAngle,
			Confidence:    1.0,
			LastSeen:      now,
			FramePosition: framePosition,
			Velocity:      0,
			FaceWidth:     faceWidth,
			Distance:      distance,
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

// GetTargetWorldAngle returns the world angle Eva should look at.
// The returned angle is body-relative (for head movement).
// If no target, returns (0, false) to indicate no valid target.
func (w *WorldModel) GetTargetWorldAngle() (float64, bool) {
	entity := w.GetFocusTarget()
	if entity == nil {
		return 0, false
	}

	w.mu.RLock()
	bodyYaw := w.bodyYaw
	w.mu.RUnlock()

	// Only predict if recently seen (within 500ms)
	dt := time.Since(entity.LastSeen).Seconds()
	var roomAngle float64
	if dt > 0.5 {
		// Face lost for too long - just return last known position
		// Don't predict, as velocity estimation may be wrong
		roomAngle = entity.WorldAngle
	} else {
		// Predict current position based on velocity (only for very recent detections)
		roomAngle = entity.WorldAngle + entity.Velocity*dt*0.5 // Dampen prediction
	}

	// Convert room angle to body-relative angle for head movement
	bodyRelativeAngle := roomAngle - bodyYaw

	return bodyRelativeAngle, true
}

// GetTargetRoomAngle returns the target's angle in room coordinates (absolute).
// This is useful for understanding where the target is in the room.
func (w *WorldModel) GetTargetRoomAngle() (float64, bool) {
	entity := w.GetFocusTarget()
	if entity == nil {
		return 0, false
	}
	return entity.WorldAngle, true
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

// SetBodyYaw updates the body orientation in room coordinates
func (w *WorldModel) SetBodyYaw(yaw float64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.bodyYaw = yaw
}

// GetBodyYaw returns the current body orientation in room coordinates
func (w *WorldModel) GetBodyYaw() float64 {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.bodyYaw
}

// UpdateAudioSource updates the audio direction from go-eva
func (w *WorldModel) UpdateAudioSource(angle float64, confidence float64, speaking bool) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.audioSource = &AudioSource{
		Angle:      angle,
		Confidence: confidence,
		Speaking:   speaking,
		LastSeen:   time.Now(),
	}
}

// GetAudioSource returns the current audio source if valid
func (w *WorldModel) GetAudioSource() *AudioSource {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.audioSource == nil {
		return nil
	}

	// Audio is stale after 1 second
	if time.Since(w.audioSource.LastSeen) > 1*time.Second {
		return nil
	}

	return w.audioSource
}

// AssociationThreshold is the maximum angle difference (radians) to associate audio with a face
// ~15 degrees in radians
const AssociationThreshold = 0.26

// AssociateAudio matches audio DOA to the nearest face entity.
// If a face is within AssociationThreshold of the audio angle, it's marked as speaking.
// Returns the associated entity ID, or "" if no match.
func (w *WorldModel) AssociateAudio(audioAngle float64, speaking bool, confidence float64) string {
	if !speaking || confidence < 0.3 {
		return ""
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	var closest *TrackedEntity
	minDiff := AssociationThreshold

	// Find face entity closest to audio direction
	for _, entity := range w.entities {
		// Skip stale entities
		if entity.Confidence < w.forgetThreshold {
			continue
		}

		diff := abs(entity.WorldAngle - audioAngle)
		if diff < minDiff {
			minDiff = diff
			closest = entity
		}
	}

	if closest != nil {
		// Boost audio confidence for matched entity
		closest.AudioConfidence = confidence
		closest.LastAudioMatch = time.Now()
		return closest.ID
	}

	return ""
}

// GetSpeakingEntity returns the entity that is currently speaking (if any).
// An entity is considered speaking if it was matched to audio within the last second.
func (w *WorldModel) GetSpeakingEntity() *TrackedEntity {
	w.mu.RLock()
	defer w.mu.RUnlock()

	for _, entity := range w.entities {
		if entity.AudioConfidence > 0.3 &&
			time.Since(entity.LastAudioMatch) < 1*time.Second {
			return entity
		}
	}
	return nil
}

// GetTarget returns the best target angle using priority:
// 1. Face that is also speaking (audio-visual match)
// 2. Any visible face
// 3. Audio source alone
// Returns (angle, source, ok) where source is "face+audio", "face", "audio", or ""
func (w *WorldModel) GetTarget() (angle float64, source string, ok bool) {
	// Priority 1: Face that is speaking (audio-visual match)
	if speaking := w.GetSpeakingEntity(); speaking != nil {
		w.mu.RLock()
		bodyYaw := w.bodyYaw
		w.mu.RUnlock()
		return speaking.WorldAngle - bodyYaw, "face+audio", true
	}

	// Priority 2: Any face (visual tracking)
	if faceAngle, hasTarget := w.GetTargetWorldAngle(); hasTarget {
		return faceAngle, "face", true
	}

	// Priority 3: Audio alone (if speaking with good confidence)
	audio := w.GetAudioSource()
	if audio != nil && audio.Speaking && audio.Confidence > 0.3 {
		// Audio angle is already in Eva coordinates, but needs body adjustment
		w.mu.RLock()
		bodyYaw := w.bodyYaw
		w.mu.RUnlock()

		// Audio angle is relative to Eva's current body orientation
		// So we just use it directly as head offset
		return audio.Angle - bodyYaw, "audio", true
	}

	// No target
	return 0, "", false
}

// abs returns the absolute value of x
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// UpdateObject updates or adds a detected object to the world model
func (w *WorldModel) UpdateObject(className string, confidence, posX, posY, width, height float64, isAnimal bool) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.objects[className] = &DetectedObject{
		ClassName:  className,
		Confidence: confidence,
		PositionX:  posX,
		PositionY:  posY,
		Width:      width,
		Height:     height,
		IsAnimal:   isAnimal,
		LastSeen:   time.Now(),
	}
}

// UpdateObjects replaces all objects with a new set
func (w *WorldModel) UpdateObjects(objects []*DetectedObject) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Clear old objects
	w.objects = make(map[string]*DetectedObject)

	// Add new objects
	for _, obj := range objects {
		obj.LastSeen = time.Now()
		w.objects[obj.ClassName] = obj
	}
}

// GetObjects returns all currently detected objects
func (w *WorldModel) GetObjects() []*DetectedObject {
	w.mu.RLock()
	defer w.mu.RUnlock()

	result := make([]*DetectedObject, 0, len(w.objects))
	for _, obj := range w.objects {
		// Only include recently seen objects (within 2 seconds)
		if time.Since(obj.LastSeen) < 2*time.Second {
			copy := *obj
			result = append(result, &copy)
		}
	}
	return result
}

// GetAnimals returns all detected animals
func (w *WorldModel) GetAnimals() []*DetectedObject {
	objects := w.GetObjects()
	var animals []*DetectedObject
	for _, obj := range objects {
		if obj.IsAnimal {
			animals = append(animals, obj)
		}
	}
	return animals
}

// HasObject returns true if the specified object class is currently visible
func (w *WorldModel) HasObject(className string) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()

	obj, exists := w.objects[className]
	if !exists {
		return false
	}
	return time.Since(obj.LastSeen) < 2*time.Second
}

// GetObjectsSummary returns a human-readable summary of detected objects
func (w *WorldModel) GetObjectsSummary() string {
	objects := w.GetObjects()
	if len(objects) == 0 {
		return ""
	}

	summary := ""
	for i, obj := range objects {
		if i > 0 {
			summary += ", "
		}
		summary += obj.ClassName
	}
	return summary
}
