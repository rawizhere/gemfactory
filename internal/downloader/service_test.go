package downloader

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMaxSegmentDuration(t *testing.T) {
	svc := &Service{}
	ctx := context.Background()

	require.Equal(t, float64(30), svc.MaxSegmentDurationSeconds(ctx, true), "want 30s HQ limit")
	require.Equal(t, float64(300), svc.MaxSegmentDurationSeconds(ctx, false), "want 300s standard limit")

	t.Setenv("CLIP_MAX_DURATION_HQ_SECONDS", "45")
	t.Setenv("CLIP_MAX_DURATION_SECONDS", "600")

	require.Equal(t, float64(45), svc.MaxSegmentDurationSeconds(ctx, true), "want 45s HQ limit from env")
	require.Equal(t, float64(600), svc.MaxSegmentDurationSeconds(ctx, false), "want 600s standard limit from env")
}

func TestResolveSegmentBounds(t *testing.T) {
	svc := &Service{}
	ctx := context.Background()
	// 61-second video.
	meta := &SourceMeta{Duration: 61}

	start, end, err := svc.resolveSegmentBounds(ctx, meta, ClipRequest{Start: "0:06", End: "9:99"})
	require.NoError(t, err, "end past video length must clamp to the end")
	require.Equal(t, float64(6000), start)
	require.Equal(t, float64(61000), end)

	_, _, err = svc.resolveSegmentBounds(ctx, meta, ClipRequest{Start: "1:02", End: "9:99"})
	require.ErrorContains(t, err, "beyond the end", "start past the video must fail")

	// 10-minute video: clamping cannot rescue an overlong interval.
	long := &SourceMeta{Duration: 600}
	_, _, err = svc.resolveSegmentBounds(ctx, long, ClipRequest{Start: "0:20", End: "11:06"})
	require.ErrorContains(t, err, "is too long")

	_, _, err = svc.resolveSegmentBounds(ctx, long, ClipRequest{Start: "0:00", End: "4:59", HQ: true})
	require.ErrorContains(t, err, "is too long", "HQ limit is 30s")
}

func TestCheckFileSize(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "sample.mp4")

	require.Error(t, checkFileSize(filePath, 49*1024*1024), "expected error for non-existent file")

	require.NoError(t, os.WriteFile(filePath, make([]byte, 1024), 0644))
	require.NoError(t, checkFileSize(filePath, 49*1024*1024), "unexpected error for small file")

	t.Setenv("TG_FILE_LIMIT_MB", "1")
	largePath := filepath.Join(tmpDir, "large.mp4")
	require.NoError(t, os.WriteFile(largePath, make([]byte, 2*1024*1024), 0644))
	require.Error(t, checkFileSize(largePath, 1024*1024), "expected error for file exceeding size limit")
}

func TestMustParsePair(t *testing.T) {
	s, e := mustParsePair("00:10", "00:25.500")
	require.Equal(t, float64(10000), s)
	require.Equal(t, float64(25500), e)
}

func TestFriendlyError(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "bot check",
			raw:  "ERROR: [youtube] abc: Sign in to confirm you're not a bot. Use --cookies",
			want: "YouTube requires authentication from this server. Add youtube.com cookies in the web panel and try again.",
		},
		{
			name: "no js runtime",
			raw:  "WARNING: No supported JavaScript runtime could be found. Only deno is enabled by default",
			want: "No JavaScript runtime (deno/node) available for yt-dlp. Rebuild the image: docker compose build.",
		},
		{
			name: "private video",
			raw:  "ERROR: Private video. Sign in if you've been granted access",
			want: "Video is private or members-only. Cookies from an authorized account are required.",
		},
		{
			name: "generic truncated",
			raw:  strings.Repeat("x", 500),
			want: "Error: " + strings.Repeat("x", 300) + "...",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.want, FriendlyError(c.raw))
		})
	}
}

func TestGetEncodeOptions(t *testing.T) {
	svc := &Service{}

	// Default fallback values
	optsClip := svc.GetEncodeOptions(context.Background(), false)
	require.Equal(t, EncodeOptions{CRF: "20", Preset: "fast", AudioBitrate: "192k", MaxFileMB: 49}, optsClip, "unexpected default clip options")

	optsSubs := svc.GetEncodeOptions(context.Background(), true)
	require.Equal(t, EncodeOptions{CRF: "20", Preset: "fast", AudioBitrate: "192k", MaxFileMB: 49}, optsSubs, "unexpected default subs options")

	// Environment variable overrides
	t.Setenv("CLIP_CRF", "23")
	t.Setenv("SUBS_CRF", "18")
	t.Setenv("CLIP_PRESET", "veryfast")
	t.Setenv("CLIP_AUDIO_BITRATE", "128k")
	t.Setenv("TG_FILE_LIMIT_MB", "30")

	optsClipEnv := svc.GetEncodeOptions(context.Background(), false)
	require.Equal(t, EncodeOptions{CRF: "23", Preset: "veryfast", AudioBitrate: "128k", MaxFileMB: 30}, optsClipEnv, "unexpected env clip options")

	optsSubsEnv := svc.GetEncodeOptions(context.Background(), true)
	require.Equal(t, EncodeOptions{CRF: "18", Preset: "veryfast", AudioBitrate: "128k", MaxFileMB: 30}, optsSubsEnv, "unexpected env subs options")
}

func TestOutputPathFor(t *testing.T) {
	svc := &Service{
		dataDir: "/tmp/workbench",
	}

	fullJob := &Job{
		VideoID: "abc123XYZ",
		Request: ClipRequest{
			URL:    "https://youtube.com/shorts/abc123XYZ",
			Shorts: true,
		},
	}
	require.Equal(t, "/tmp/workbench/abc123XYZ/abc123XYZ_full.mp4", svc.outputPathFor(fullJob))

	clipJob := &Job{
		VideoID: "abc123XYZ",
		Request: ClipRequest{
			URL:   "https://youtube.com/shorts/abc123XYZ",
			Start: "0:10",
			End:   "0:30",
		},
	}
	require.Equal(t, "/tmp/workbench/abc123XYZ/abc123XYZ_000010-000-000030-000.mp4", svc.outputPathFor(clipJob))
}

func TestFormatSelector(t *testing.T) {
	horizontal := formatSelector("1080p", false, false, false, false)
	require.Equal(t, "bestvideo[height<=1080]+bestaudio/best[height<=1080]", horizontal)

	horizontal720 := formatSelector("720p", false, false, false, false)
	require.Equal(t, "bestvideo[height<=720]+bestaudio/best[height<=720]", horizontal720)

	horizontal720Raw := formatSelector("720", false, false, false, false)
	require.Equal(t, "bestvideo[height<=720]+bestaudio/best[height<=720]", horizontal720Raw)

	vertical := formatSelector("1080p", false, false, false, true)
	require.Equal(t, "bestvideo[width<=1080]+bestaudio/best[width<=1080]", vertical)

	vertical720 := formatSelector("720p", false, false, false, true)
	require.Equal(t, "bestvideo[width<=720]+bestaudio/best[width<=720]", vertical720)

	verticalHQ := formatSelector("1080p", true, false, false, true)
	require.Equal(t, "bestvideo[width<=1440]+bestaudio/best[width<=1440]", verticalHQ)
}
