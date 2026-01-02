package detection

import (
	"testing"
)

func TestDetection_Center(t *testing.T) {
	tests := []struct {
		name    string
		det     Detection
		expectX float64
		expectY float64
	}{
		{
			name:    "center of image",
			det:     Detection{X: 0.25, Y: 0.25, W: 0.5, H: 0.5},
			expectX: 0.5,
			expectY: 0.5,
		},
		{
			name:    "top left corner",
			det:     Detection{X: 0, Y: 0, W: 0.2, H: 0.2},
			expectX: 0.1,
			expectY: 0.1,
		},
		{
			name:    "bottom right corner",
			det:     Detection{X: 0.8, Y: 0.8, W: 0.2, H: 0.2},
			expectX: 0.9,
			expectY: 0.9,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			x, y := tc.det.Center()
			if x != tc.expectX {
				t.Errorf("Center X: got %.2f, want %.2f", x, tc.expectX)
			}
			if y != tc.expectY {
				t.Errorf("Center Y: got %.2f, want %.2f", y, tc.expectY)
			}
		})
	}
}

func TestDetection_Area(t *testing.T) {
	tests := []struct {
		name   string
		det    Detection
		expect float64
	}{
		{
			name:   "quarter of image",
			det:    Detection{X: 0, Y: 0, W: 0.5, H: 0.5},
			expect: 0.25,
		},
		{
			name:   "small face",
			det:    Detection{X: 0, Y: 0, W: 0.1, H: 0.2},
			expect: 0.02,
		},
		{
			name:   "full image",
			det:    Detection{X: 0, Y: 0, W: 1.0, H: 1.0},
			expect: 1.0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			area := tc.det.Area()
			// Use tolerance for floating-point comparison
			diff := area - tc.expect
			if diff < -0.0001 || diff > 0.0001 {
				t.Errorf("Area: got %.4f, want %.4f", area, tc.expect)
			}
		})
	}
}

func TestSelectBest(t *testing.T) {
	tests := []struct {
		name       string
		detections []Detection
		expectNil  bool
		expectIdx  int // Expected index of best detection
	}{
		{
			name:       "empty list",
			detections: []Detection{},
			expectNil:  true,
		},
		{
			name: "single detection",
			detections: []Detection{
				{X: 0.4, Y: 0.4, W: 0.2, H: 0.2, Confidence: 0.9},
			},
			expectNil: false,
			expectIdx: 0,
		},
		{
			name: "high confidence beats larger area",
			detections: []Detection{
				{X: 0.0, Y: 0.0, W: 0.4, H: 0.4, Confidence: 0.5},  // Larger but low conf
				{X: 0.3, Y: 0.3, W: 0.2, H: 0.2, Confidence: 0.95}, // Smaller but high conf
			},
			expectNil: false,
			expectIdx: 1, // High confidence wins (0.95*0.7 + 0.25*0.3 = 0.74 vs 0.5*0.7 + 1.0*0.3 = 0.65)
		},
		{
			name: "similar confidence picks larger",
			detections: []Detection{
				{X: 0.0, Y: 0.0, W: 0.5, H: 0.5, Confidence: 0.8}, // Larger
				{X: 0.3, Y: 0.3, W: 0.1, H: 0.1, Confidence: 0.8}, // Smaller
			},
			expectNil: false,
			expectIdx: 0, // Same confidence, larger area wins
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			best := SelectBest(tc.detections)
			if tc.expectNil {
				if best != nil {
					t.Errorf("SelectBest: expected nil, got %+v", best)
				}
				return
			}

			if best == nil {
				t.Error("SelectBest: expected non-nil, got nil")
				return
			}

			expected := &tc.detections[tc.expectIdx]
			if best.Confidence != expected.Confidence || best.X != expected.X {
				t.Errorf("SelectBest: got %+v, want %+v", best, expected)
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.ModelPath == "" {
		t.Error("DefaultConfig: ModelPath should not be empty")
	}

	if cfg.ConfidenceThresh <= 0 || cfg.ConfidenceThresh > 1 {
		t.Errorf("DefaultConfig: ConfidenceThresh should be 0-1, got %f", cfg.ConfidenceThresh)
	}

	if cfg.InputWidth <= 0 {
		t.Errorf("DefaultConfig: InputWidth should be positive, got %d", cfg.InputWidth)
	}

	if cfg.InputHeight <= 0 {
		t.Errorf("DefaultConfig: InputHeight should be positive, got %d", cfg.InputHeight)
	}
}
