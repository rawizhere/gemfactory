package downloader

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMaxSegmentDuration(t *testing.T) {
	svc := &Service{}

	if dur := svc.MaxSegmentDurationSeconds(true); dur != 30 {
		t.Errorf("want 30s HQ limit, got %v", dur)
	}
	if dur := svc.MaxSegmentDurationSeconds(false); dur != 300 {
		t.Errorf("want 300s standard limit, got %v", dur)
	}

	t.Setenv("CLIP_MAX_DURATION_HQ_SECONDS", "45")
	t.Setenv("CLIP_MAX_DURATION_SECONDS", "600")

	if dur := svc.MaxSegmentDurationSeconds(true); dur != 45 {
		t.Errorf("want 45s HQ limit from env, got %v", dur)
	}
	if dur := svc.MaxSegmentDurationSeconds(false); dur != 600 {
		t.Errorf("want 600s standard limit from env, got %v", dur)
	}
}

func TestCheckFileSize(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "sample.mp4")

	if err := checkFileSize(filePath); err == nil {
		t.Error("expected error for non-existent file")
	}

	if err := os.WriteFile(filePath, make([]byte, 1024), 0644); err != nil {
		t.Fatal(err)
	}
	if err := checkFileSize(filePath); err != nil {
		t.Errorf("unexpected error for small file: %v", err)
	}

	t.Setenv("TG_FILE_LIMIT_MB", "1")
	largePath := filepath.Join(tmpDir, "large.mp4")
	if err := os.WriteFile(largePath, make([]byte, 2*1024*1024), 0644); err != nil {
		t.Fatal(err)
	}
	if err := checkFileSize(largePath); err == nil {
		t.Error("expected error for file exceeding size limit")
	}
}

func TestMustParsePair(t *testing.T) {
	s, e := mustParsePair("00:10", "00:25.500")
	if s != 10000 || e != 25500 {
		t.Errorf("want (10000, 25500), got (%v, %v)", s, e)
	}
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
			if got := FriendlyError(c.raw); got != c.want {
				t.Errorf("FriendlyError() mismatch:\n got %q\nwant %q", got, c.want)
			}
		})
	}
}

func TestGetEncodeOptions(t *testing.T) {
	svc := &Service{}

	// Default fallback values
	optsClip := svc.GetEncodeOptions(context.Background(), false)
	if optsClip.CRF != "20" || optsClip.Preset != "fast" || optsClip.AudioBitrate != "192k" {
		t.Errorf("unexpected default clip options: %+v", optsClip)
	}

	optsSubs := svc.GetEncodeOptions(context.Background(), true)
	if optsSubs.CRF != "20" || optsSubs.Preset != "fast" || optsSubs.AudioBitrate != "192k" {
		t.Errorf("unexpected default subs options: %+v", optsSubs)
	}

	// Environment variable overrides
	t.Setenv("CLIP_CRF", "23")
	t.Setenv("SUBS_CRF", "18")
	t.Setenv("CLIP_PRESET", "veryfast")
	t.Setenv("CLIP_AUDIO_BITRATE", "128k")

	optsClipEnv := svc.GetEncodeOptions(context.Background(), false)
	if optsClipEnv.CRF != "23" || optsClipEnv.Preset != "veryfast" || optsClipEnv.AudioBitrate != "128k" {
		t.Errorf("unexpected env clip options: %+v", optsClipEnv)
	}

	optsSubsEnv := svc.GetEncodeOptions(context.Background(), true)
	if optsSubsEnv.CRF != "18" || optsSubsEnv.Preset != "veryfast" || optsSubsEnv.AudioBitrate != "128k" {
		t.Errorf("unexpected env subs options: %+v", optsSubsEnv)
	}
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
	if got := svc.outputPathFor(fullJob); got != "/tmp/workbench/abc123XYZ/abc123XYZ_full.mp4" {
		t.Errorf("want full path, got %q", got)
	}

	clipJob := &Job{
		VideoID: "abc123XYZ",
		Request: ClipRequest{
			URL:   "https://youtube.com/shorts/abc123XYZ",
			Start: "0:10",
			End:   "0:30",
		},
	}
	if got := svc.outputPathFor(clipJob); got != "/tmp/workbench/abc123XYZ/abc123XYZ_000010-000-000030-000.mp4" {
		t.Errorf("want clip path, got %q", got)
	}
}
