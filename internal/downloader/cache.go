package downloader

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
)

// cacheMarker marks a finished clip so repeats survive restarts.
type cacheMarker struct {
	Title       string    `json:"title,omitempty"`
	AltTitle    string    `json:"alt_title,omitempty"`
	Caption     string    `json:"caption,omitempty"`
	Tags        []string  `json:"tags,omitempty"`
	Translation string    `json:"translation,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// markerPath is the sidecar path for a finished output file.
func markerPath(outputPath string) string {
	return outputPath + ".ok.json"
}

// readCacheHit returns the marker when a previously completed clip exists.
func readCacheHit(outputPath string) *cacheMarker {
	data, err := os.ReadFile(markerPath(outputPath))
	if err != nil {
		return nil
	}
	var m cacheMarker
	if json.Unmarshal(data, &m) != nil {
		return nil
	}
	// Marker without the actual file is stale.
	if st, err := os.Stat(outputPath); err != nil || st.Size() == 0 {
		return nil
	}
	return &m
}

// writeCacheMarker records a successfully produced clip.
func writeCacheMarker(outputPath string, m cacheMarker) {
	m.CreatedAt = time.Now()
	data, err := json.Marshal(m)
	if err != nil {
		return
	}
	if err := os.WriteFile(markerPath(outputPath), data, 0644); err != nil {
		slog.Warn("failed to write cache marker", "path", markerPath(outputPath), "error", err)
	}
}

// StartCleanupLoop removes downloaded files once per day until ctx is done.
func (s *Service) StartCleanupLoop(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				n, bytes := s.CleanupOnce()
				s.logger.Info("downloads cleanup finished",
					zap.Int("files_removed", n), zap.Int64("bytes_freed", bytes))
			}
		}
	}()
}

// CleanupOnce deletes download artifacts older than DOWNLOAD_RETENTION_HOURS (default 24h).
func (s *Service) CleanupOnce() (int, int64) {
	retention := retentionPeriod()
	cutoff := time.Now().Add(-retention)

	var removed int
	var freed int64

	err := filepath.WalkDir(s.dataDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if d.IsDir() {
			return nil
		}
		info, statErr := d.Info()
		if statErr != nil {
			return nil
		}
		if info.ModTime().After(cutoff) {
			return nil
		}
		if !isRemovableArtifact(d.Name()) {
			return nil
		}
		size := info.Size()
		if rmErr := os.Remove(path); rmErr == nil {
			removed++
			freed += size
		}
		return nil
	})
	if err != nil {
		s.logger.Warn("cleanup walk failed", zap.String("dir", s.dataDir), zap.Error(err))
	}

	pruneEmptyDirs(s.dataDir)

	// Prune completed in-memory jobs to bound memory usage.
	s.mu.Lock()
	if len(s.jobs) > 100 {
		newJobs := make(map[string]*Job)
		var newOrder []string
		keepStart := len(s.order) - 50
		if keepStart < 0 {
			keepStart = 0
		}
		for i, id := range s.order {
			if j, ok := s.jobs[id]; ok {
				if j.Status == StatusPending || j.Status == StatusDownloading || j.Status == StatusProcessing || i >= keepStart {
					newJobs[id] = j
					newOrder = append(newOrder, id)
				}
			}
		}
		s.jobs = newJobs
		s.order = newOrder
	}
	s.mu.Unlock()

	return removed, freed
}

// isRemovableArtifact reports whether the filename is generated download output.
func isRemovableArtifact(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".mp4", ".mkv", ".webm", ".mov", ".vtt", ".json":
		return true
	default:
		return false
	}
}

// pruneEmptyDirs removes empty leaf directories bottom-up.
func pruneEmptyDirs(root string) {
	var dirs []string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err == nil && d.IsDir() && path != root {
			dirs = append(dirs, path)
		}
		return nil
	})
	for i := len(dirs) - 1; i >= 0; i-- {
		_ = os.Remove(dirs[i]) // only removes empty dirs
	}
}

func retentionPeriod() time.Duration {
	raw := os.Getenv("DOWNLOAD_RETENTION_HOURS")
	if raw != "" {
		if h, err := strconv.Atoi(raw); err == nil && h > 0 {
			return time.Duration(h) * time.Hour
		}
	}
	return 24 * time.Hour
}
