package downloader

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
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
		{ClipRequest{SubsLang: "ru", SubsNoLLM: true}, "-ru-yt"},
	}
	for _, c := range cases {
		require.Equal(t, c.want, variantSuffix(c.req), "variantSuffix(%+v)", c.req)
	}
}

func TestFileNameWithTimecodeVariant(t *testing.T) {
	got := fileNameWithTimecodeVariant("dQw4w9WgXcQ", 365900, 376100, "-ru")
	require.True(t, strings.HasPrefix(got, "dQw4w9WgXcQ_000605-900-000616-100-ru"), "unexpected name %q", got)
}

func TestCacheMarkerRoundTrip(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "clip.mp4")
	require.NoError(t, os.WriteFile(out, []byte("video"), 0644))
	require.Nil(t, readCacheHit(out), "cache hit must not exist without marker")

	writeCacheMarker(out, cacheMarker{Title: "Test Title", Tags: []string{"#tag"}})
	m := readCacheHit(out)
	require.NotNil(t, m, "marker = %+v", m)
	require.Equal(t, "Test Title", m.Title)
	require.Equal(t, []string{"#tag"}, m.Tags)
}

func TestCacheHitRequiresVideoFile(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "clip.mp4")
	writeCacheMarker(out, cacheMarker{Title: "orphan"})
	require.Nil(t, readCacheHit(out), "marker without video file must be ignored")
}

func newTestService(t *testing.T) *Service {
	t.Helper()
	return NewService(nil, nil, t.TempDir(), 1, zap.NewNop())
}

func TestCleanupOnceRemovesOldArtifactsOnly(t *testing.T) {
	s := newTestService(t)

	oldDir := filepath.Join(s.dataDir, "vidold")
	newDir := filepath.Join(s.dataDir, "vidnew")
	require.NoError(t, os.MkdirAll(oldDir, 0755))
	require.NoError(t, os.MkdirAll(newDir, 0755))

	newFile := filepath.Join(newDir, "vidnew_000605-900.mp4")
	past := time.Now().Add(-48 * time.Hour)
	// Every kind of downloader output must expire, not just video.
	oldFiles := []string{"vidold_000605-900.mp4", "vidold_full.mp3", "ig_x_1.jpg", "vidold.f137.mp4.part"}
	for _, name := range oldFiles {
		p := filepath.Join(oldDir, name)
		require.NoError(t, os.WriteFile(p, []byte("x"), 0644))
		require.NoError(t, os.Chtimes(p, past, past))
	}
	require.NoError(t, os.WriteFile(newFile, []byte("x"), 0644))

	removed, _ := s.CleanupOnce(context.Background())

	require.Equal(t, len(oldFiles), removed, "want every stale file removed")
	require.NoError(t, isMissing(newFile), "fresh artifact must survive cleanup")
	for _, name := range oldFiles {
		require.True(t, os.IsNotExist(isMissing(filepath.Join(oldDir, name))), "stale %s must be removed", name)
	}
	// Empty directory pruned.
	require.True(t, os.IsNotExist(isMissing(oldDir)), "empty dir after cleanup should be pruned")
}

func TestSubmitAcceptsOverlongIntervals(t *testing.T) {
	t.Setenv("CLIP_MAX_DURATION_SECONDS", "300")
	t.Setenv("CLIP_MAX_DURATION_HQ_SECONDS", "30")
	s := newTestService(t)

	// Overlong intervals are clamped/fail later, after metadata reveals the real duration.
	// Invalid URL on purpose: its error proves the video-id lookup is now the first rejection point.
	_, err := s.SubmitWithCallbacks(context.Background(), ClipRequest{URL: "https://example.com/video", Start: "0:00", End: "6:01"}, nil)
	require.ErrorContains(t, err, "cannot extract video id")
	require.NotContains(t, err.Error(), "is too long", "duration must not be validated at submit time")

	_, err = s.SubmitWithCallbacks(context.Background(), ClipRequest{URL: "https://example.com/video", Start: "0:31", End: "0:30"}, nil)
	require.ErrorContains(t, err, "must be after start", "reversed interval still fails at submit")
}

func TestCleanStorageProtectsActiveJobs(t *testing.T) {
	s := newTestService(t)

	activeDir := filepath.Join(s.dataDir, "activeVid")
	doneDir := filepath.Join(s.dataDir, "doneVid")
	require.NoError(t, os.MkdirAll(activeDir, 0755))
	require.NoError(t, os.MkdirAll(doneDir, 0755))

	activeFile := filepath.Join(activeDir, "clip.mp4")
	doneFile := filepath.Join(doneDir, "clip.mp4")
	require.NoError(t, os.WriteFile(activeFile, []byte("active"), 0644))
	require.NoError(t, os.WriteFile(doneFile, []byte("done"), 0644))

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
	require.NoError(t, err, "CleanStorage failed")

	require.Equal(t, 1, removed, "want 1 removed file (freed: %d)", freed)

	require.NoError(t, isMissing(activeFile), "active job file was deleted by CleanStorage")
	require.True(t, os.IsNotExist(isMissing(doneFile)), "old completed job file was not deleted by CleanStorage")

	s.mu.Lock()
	_, stillThere := s.jobs["job-done-old"]
	s.mu.Unlock()
	require.False(t, stillThere, "cleaned job should be removed from s.jobs, but still present")
}

// isMissing returns the os.Stat error for path (nil when the file exists).
func isMissing(path string) error {
	_, err := os.Stat(path)
	return err
}
