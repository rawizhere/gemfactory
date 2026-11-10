package downloader

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFormatSelectorShortsHQ(t *testing.T) {
	sel := formatSelector("", true, true, false, true)
	require.Equal(t, "bestvideo+bestaudio/best", sel, "shorts+hq must be uncapped")

	def := formatSelector("", false, true, false, true)
	require.Contains(t, def, "avc1", "default shorts path must stay avc1-first")
}

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

func TestFallbackFetchArgs(t *testing.T) {
	args := fallbackFetchArgs()
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "youtube:player_client=web_safari,android") {
		t.Fatalf("expected player_client override, got %q", joined)
	}
	if strings.Contains(joined, "mweb") {
		t.Fatalf("fallback must not keep mweb client, got %q", joined)
	}
	if !strings.Contains(joined, "-N 1") {
		t.Fatalf("expected concurrent fragments disabled (-N 1), got %q", joined)
	}
}

func TestParseDownloadProgress(t *testing.T) {
	upd, ok := parseYTDLPProgress("[download]  42.5% of ~ 25.40MiB at 4.12MiB/s ETA Unknown")
	require.True(t, ok)
	require.Equal(t, ProgressUpdate{Stage: StageDownload, Percent: 42, Size: "25.4 MB", Speed: "4.12MiB/s"}, upd)
	_, ok = parseYTDLPProgress("frame= 10 time=00:00:01.00")
	require.False(t, ok)

	line := "frame=  250 fps= 50 q=-1.0 size=    2048KiB time=00:00:05.00 bitrate=3355.4kbits/s speed=2.00x"
	upd, ok = parseSectionProgress(line, 10000)
	require.True(t, ok)
	require.Equal(t, ProgressUpdate{Stage: StageDownload, Percent: 50, Size: "2.0 MB", Speed: "839 KB/s"}, upd)
	upd, ok = parseSectionProgress("size=1KiB time=00:00:30.00 speed=N/A", 10000)
	require.True(t, ok)
	require.Equal(t, 100, upd.Percent)
	require.Equal(t, "N/A", upd.Speed)
	_, ok = parseSectionProgress(line, 0)
	require.False(t, ok, "no expected duration, no section progress")
}
