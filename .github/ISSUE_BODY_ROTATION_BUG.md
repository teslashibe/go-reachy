# Body Rotation Bug at Mechanical Limits

## Summary

When body rotation reaches its extreme limit (currently set to ±1.0 rad), the robot stops tracking properly. The log shows:
```
🔄 Body rotation: -1.00 → -1.00 rad
```
...and Eva becomes stuck, unable to continue following a face that's off to the side.

## Root Cause Analysis

This is a **compounding error** caused by three interacting issues:

### 1. Head Counter-Rotation Runs Even When Body Can't Move

In `checkBodyAlignment()` (tracker.go:721-812):
```go
handler(bodyDelta)  // Body tries to move but is clamped at limit

// Counter-rotate head to maintain gaze
if reason == "centering head" {
    t.controller.AdjustForBodyRotation(bodyDelta)  // ← BUG: Runs with INTENDED delta, not ACTUAL
}
```

When body is at -1.0 rad and can't move further right:
- `handler(bodyDelta)` is called with negative delta
- Body gets clamped, no actual movement occurs
- But `AdjustForBodyRotation(bodyDelta)` still runs with the intended delta
- Head gets pushed in the WRONG direction (away from face!)

### 2. No Pre-Check for Body Limits

`checkBodyAlignment()` doesn't check if body is already at its limit before trying to move it further in the same direction. It blindly calculates a delta and calls the handler.

### 3. Handler Doesn't Report Actual Movement

The `BodyRotationHandler` in main.go:356-368 clamps the body position but doesn't return whether the body actually moved:
```go
headTracker.SetBodyRotationHandler(func(direction float64) {
    // Clamps newBody but doesn't tell caller if movement occurred
    handler(bodyDelta)  // Returns nothing
})
```

### The Cascading Failure

1. Face is to the right → Head turns right (negative yaw)
2. Body alignment kicks in → tries to rotate body right
3. Body already at -1.0 → clamped, no actual movement
4. Head counter-rotates anyway → pushed LEFT (wrong direction!)
5. Face detection sees face even more off-center → bigger error
6. Loop repeats → System appears "stuck"

## Discovery: 1.0 rad Limit is NOT Hardware-Enforced

The current ±1.0 rad (±57°) limit is arbitrary. Python reachy uses:
```python
max_angle = 0.9 * np.pi  # ≈ 2.83 rad ≈ 162°
```

The hardware supports **much larger rotation** than we're allowing.

## Proposed Fix

### Part 1: Define Proper Hardware Limits

Create `pkg/tracking/limits.go`:
- Central configuration for all mechanical limits
- Default body limit: ±2.0 rad (±115°) - conservative but allows full range

### Part 2: Fix Handler to Return Actual Delta

Change `BodyRotationHandler` signature:
```go
// Before
type BodyRotationHandler func(direction float64)

// After - returns actual delta (0 if at limit)
type BodyRotationHandler func(direction float64) float64
```

Update main.go handler to return `actualDelta := newBody - currentBody`.

### Part 3: World Model Limit Awareness

Add to `WorldModel`:
```go
func (w *WorldModel) IsBodyAtLimit(direction float64) bool
func (w *WorldModel) CanBodyRotate(direction float64) bool
```

### Part 4: Pre-Check in checkBodyAlignment()

```go
func (t *Tracker) checkBodyAlignment() {
    // ... calculate bodyDelta ...
    
    // PRE-CHECK: Can body actually move in this direction?
    if !t.world.CanBodyRotate(bodyDelta) {
        return // Body at limit, let head track alone
    }
    
    actualDelta := handler(bodyDelta)
    
    // ONLY counter-rotate if body ACTUALLY moved
    if reason == "centering head" && math.Abs(actualDelta) > 0.001 {
        t.controller.AdjustForBodyRotation(actualDelta)
    }
}
```

## Files to Modify

| File | Change |
|------|--------|
| `pkg/tracking/limits.go` | **NEW** - Central limits configuration |
| `pkg/tracking/tracker.go` | Update handler signature, fix checkBodyAlignment() |
| `pkg/worldmodel/worldmodel.go` | Add limit tracking and query methods |
| `cmd/eva/main.go` | Update handler to return actual delta |
| `pkg/tracking/config.go` | Add BodyYawLimit to Config |

## Testing Plan

1. **Limit boundary test:** Face at extreme right, verify body stops cleanly and head continues tracking
2. **Return to neutral test:** Face returns to center, body gradually returns to 0
3. **Full range test:** Face at 150° - verify combined head+body coverage

## Labels

- bug
- tracking
- world-model
- priority: medium

## Related Code

- `pkg/tracking/tracker.go:721-812` - checkBodyAlignment()
- `pkg/tracking/controller.go:347-359` - AdjustForBodyRotation()
- `cmd/eva/main.go:356-368` - BodyRotationHandler callback


