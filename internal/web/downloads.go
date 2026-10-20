package web

import (
	"fmt"
	"net/http"

	"github.com/dustin/go-humanize"
)

func (s *Server) getStorageUsage(w http.ResponseWriter, r *http.Request) {
	if s.Downloads == nil {
		writeJSON(w, map[string]any{"bytes": 0, "formatted": "0 B", "files": 0})
		return
	}
	b, files, err := s.Downloads.GetStorageUsage()
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, map[string]any{
		"bytes":     b,
		"formatted": formatBytes(b),
		"files":     files,
	})
}

func (s *Server) cleanStorage(w http.ResponseWriter, r *http.Request) {
	if s.Downloads == nil {
		http.Error(w, "downloader unavailable", http.StatusServiceUnavailable)
		return
	}
	freed, files, err := s.Downloads.CleanStorage()
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, map[string]any{
		"freed_bytes": freed,
		"formatted":   formatBytes(freed),
		"files":       files,
	})
}

// formatBytes renders byte counts in binary (1024-based) units.
func formatBytes(b int64) string {
	if b < 1024 {
		return fmt.Sprintf("%d B", b)
	}
	return humanize.IBytes(uint64(b))
}
