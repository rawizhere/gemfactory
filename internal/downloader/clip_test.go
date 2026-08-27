package downloader

import (
	"testing"
)

func TestCalculateBitrateCapKbps(t *testing.T) {
	tests := []struct {
		name        string
		durationSec float64
		audio       string
		gif         bool
		minExpected float64
		maxExpected float64
	}{
		{
			name:        "zero duration",
			durationSec: 0,
			audio:       "192k",
			gif:         false,
			minExpected: 0,
			maxExpected: 0,
		},
		{
			name:        "30 seconds video",
			durationSec: 30,
			audio:       "192k",
			gif:         false,
			minExpected: 12500,
			maxExpected: 13500,
		},
		{
			name:        "60 seconds video",
			durationSec: 60,
			audio:       "192k",
			gif:         false,
			minExpected: 6200,
			maxExpected: 6800,
		},
		{
			name:        "120 seconds video",
			durationSec: 120,
			audio:       "192k",
			gif:         false,
			minExpected: 3000,
			maxExpected: 3300,
		},
		{
			name:        "60 seconds gif (no audio)",
			durationSec: 60,
			audio:       "192k",
			gif:         true,
			minExpected: 6400,
			maxExpected: 7000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateBitrateCapKbps(tt.durationSec, tt.audio, 49, tt.gif)
			if tt.minExpected == 0 && tt.maxExpected == 0 {
				if got != 0 {
					t.Fatalf("expected 0, got %f", got)
				}
				return
			}
			if got < tt.minExpected || got > tt.maxExpected {
				t.Fatalf("expected bitrate between %f and %f kbps, got %f", tt.minExpected, tt.maxExpected, got)
			}
		})
	}
}
