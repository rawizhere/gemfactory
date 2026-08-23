package downloader

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestVariantSuffix(t *testing.T) {
	cases := []struct {
		req  ClipRequest
		want string
	}{
		{ClipRequest{}, ""},
		{ClipRequest{HQ: true}, "-hq"},
		{ClipRequest{GIF: true}, "-gif"},
		{ClipRequest{SubsLang: "ru"}, "-ru"},
		{ClipRequest{HQ: true, GIF: true, SubsLang: "en-US"}, "-hq-gif-en-US"},
		{ClipRequest{SubsLang: "ru", SubsNoLLM: true}, "-ru-gt"},
	}
	for _, c := range cases {
		if got := variantSuffix(c.req); got != c.want {
			t.Errorf("variantSuffix(%+v) = %q, want %q", c.req, got, c.want)
		}
	}
}

func TestFileNameWithTimecodeVariant(t *testing.T) {
	got := fileNameWithTimecodeVariant("dQw4w9WgXcQ", 365900, 376100, "-ru")
	if !strings.HasPrefix(got, "dQw4w9WgXcQ_000605-900-000616-100-ru") {
		t.Errorf("unexpected name %q", got)
	}
}

func TestCacheMarkerRoundTrip(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "clip.mp4")
	if err := os.WriteFile(out, []byte("video"), 0644); err != nil {
		t.Fatal(err)
	}

	if readCacheHit(out) != nil {
		t.Error("cache hit must not exist without marker")
	}

	writeCacheMarker(out, cacheMarker{Title: "Test Title", Caption: "<b>Test Title</b>\n\n#tag"})
	m := readCacheHit(out)
	if m == nil || m.Title != "Test Title" || m.Caption != "<b>Test Title</b>\n\n#tag" {
		t.Fatalf("marker = %+v", m)
	}
}

func TestCacheHitRequiresVideoFile(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "clip.mp4")
	writeCacheMarker(out, cacheMarker{Title: "orphan"})
	if readCacheHit(out) != nil {
		t.Error("marker without video file must be ignored")
	}
}

func newTestService(t *testing.T) *Service {
	t.Helper()
	return NewService(nil, t.TempDir(), 1, zap.NewNop())
}

func TestCleanupOnceRemovesOldArtifactsOnly(t *testing.T) {
	s := newTestService(t)

	oldDir := filepath.Join(s.dataDir, "vidold")
	newDir := filepath.Join(s.dataDir, "vidnew")
	if err := os.MkdirAll(oldDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(newDir, 0755); err != nil {
		t.Fatal(err)
	}

	oldFile := filepath.Join(oldDir, "vidold_000605-900.mp4")
	newFile := filepath.Join(newDir, "vidnew_000605-900.mp4")
	if err := os.WriteFile(oldFile, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(oldFile, past, past); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newFile, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	removed, _ := s.CleanupOnce()

	if removed != 1 {
		t.Errorf("want 1 removed file, got %d", removed)
	}
	if _, err := os.Stat(newFile); err != nil {
		t.Error("fresh artifact must survive cleanup")
	}
	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Error("stale artifact must be removed")
	}
	// Empty directory pruned.
	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Error("empty dir after cleanup should be pruned")
	}
}

func TestSubmitRejectsOverlongIntervals(t *testing.T) {
	t.Setenv("CLIP_MAX_DURATION_SECONDS", "300")
	t.Setenv("CLIP_MAX_DURATION_HQ_SECONDS", "30")
	s := newTestService(t)

	// Invalid URL on purpose: duration is validated before the video-id lookup, so no download work starts.
	badURL := "https://example.com/video"

	_, err := s.Submit(context.Background(), ClipRequest{URL: badURL, Start: "0:00", End: "6:01"})
	if err == nil || !strings.Contains(err.Error(), "is too long") {
		t.Errorf("6-minute normal clip must be rejected, got: %v", err)
	}

	_, err = s.Submit(context.Background(), ClipRequest{URL: badURL, Start: "0:00", End: "4:59"})
	if err == nil || strings.Contains(err.Error(), "is too long") {
		t.Errorf("4:59 must pass duration check (next error should be url), got: %v", err)
	}

	_, err = s.Submit(context.Background(), ClipRequest{URL: badURL, Start: "0:00", End: "0:31", HQ: true})
	if err == nil || !strings.Contains(err.Error(), "is too long") {
		t.Errorf("31-second HQ clip must be rejected, got: %v", err)
	}
}

func TestCleanStorageProtectsActiveJobs(t *testing.T) {
	s := newTestService(t)

	activeDir := filepath.Join(s.dataDir, "activeVid")
	doneDir := filepath.Join(s.dataDir, "doneVid")
	if err := os.MkdirAll(activeDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(doneDir, 0755); err != nil {
		t.Fatal(err)
	}

	activeFile := filepath.Join(activeDir, "clip.mp4")
	doneFile := filepath.Join(doneDir, "clip.mp4")
	if err := os.WriteFile(activeFile, []byte("active"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(doneFile, []byte("done"), 0644); err != nil {
		t.Fatal(err)
	}

	s.mu.Lock()
	s.jobs["job-active"] = &Job{
		ID:      "job-active",
		VideoID: "activeVid",
		Status:  StatusProcessing,
	}
	s.jobs["job-done-old"] = &Job{
		ID:         "job-done-old",
		VideoID:    "doneVid",
		Status:     StatusDone,
		OutputDir:  doneFile,
		lastProgAt: time.Now().Add(-10 * time.Minute),
	}
	s.mu.Unlock()

	freed, removed, err := s.CleanStorage()
	if err != nil {
		t.Fatalf("CleanStorage failed: %v", err)
	}

	if removed != 1 {
		t.Errorf("want 1 removed file, got %d (freed: %d)", removed, freed)
	}

	if _, err := os.Stat(activeFile); os.IsNotExist(err) {
		t.Errorf("active job file was deleted by CleanStorage")
	}

	if _, err := os.Stat(doneFile); !os.IsNotExist(err) {
		t.Errorf("old completed job file was not deleted by CleanStorage")
	}

	s.mu.Lock()
	if _, ok := s.jobs["job-done-old"]; ok {
		t.Errorf("cleaned job should be removed from s.jobs, but still present")
	}
	s.mu.Unlock()
}
