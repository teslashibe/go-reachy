package worldmodel

import (
	"testing"
)

func TestEstimateDepth(t *testing.T) {
	tests := []struct {
		name      string
		faceWidth float64
		wantMin   float64
		wantMax   float64
	}{
		{
			name:      "close face (40% of frame)",
			faceWidth: 0.4,
			wantMin:   0.4,
			wantMax:   0.6,
		},
		{
			name:      "medium face (20% of frame)",
			faceWidth: 0.2,
			wantMin:   0.9,
			wantMax:   1.1,
		},
		{
			name:      "far face (10% of frame)",
			faceWidth: 0.1,
			wantMin:   1.8,
			wantMax:   2.2,
		},
		{
			name:      "very far face (5% of frame)",
			faceWidth: 0.05,
			wantMin:   3.5,
			wantMax:   5.0, // Clamped to max
		},
		{
			name:      "invalid zero width",
			faceWidth: 0,
			wantMin:   0,
			wantMax:   0,
		},
		{
			name:      "invalid negative width",
			faceWidth: -0.1,
			wantMin:   0,
			wantMax:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EstimateDepth(tt.faceWidth)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("EstimateDepth(%v) = %v, want between %v and %v",
					tt.faceWidth, got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestDistanceCategory(t *testing.T) {
	tests := []struct {
		distance float64
		want     string
	}{
		{0, "unknown"},
		{0.3, "very close"},
		{0.7, "close"},
		{1.5, "nearby"},
		{2.5, "moderate"},
		{4.0, "far"},
	}

	for _, tt := range tests {
		got := DistanceCategory(tt.distance)
		if got != tt.want {
			t.Errorf("DistanceCategory(%v) = %q, want %q", tt.distance, got, tt.want)
		}
	}
}

