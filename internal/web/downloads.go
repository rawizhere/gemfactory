package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"gemfactory/internal/downloader"
)

func (s *Server) submitDownload(w http.ResponseWriter, r *http.Request) {
	if s.downloads == nil {
		http.Error(w, "downloader unavailable", http.StatusServiceUnavailable)
		return
	}
	var req downloader.ClipRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	job, err := s.downloads.Submit(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, job)
}

func (s *Server) listDownloads(w http.ResponseWriter, r *http.Request) {
	if s.downloads == nil {
		http.Error(w, "downloader unavailable", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, s.downloads.ListJobs())
}

func (s *Server) getDownload(w http.ResponseWriter, r *http.Request) {
	if s.downloads == nil {
		http.Error(w, "downloader unavailable", http.StatusServiceUnavailable)
		return
	}
	job, ok := s.downloads.GetJob(r.PathValue("id"))
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	writeJSON(w, job)
}

func (s *Server) downloadFile(w http.ResponseWriter, r *http.Request) {
	if s.downloads == nil {
		http.Error(w, "downloader unavailable", http.StatusServiceUnavailable)
		return
	}
	job, ok := s.downloads.GetJob(r.PathValue("id"))
	if !ok || job.Status != downloader.StatusDone || job.OutputDir == "" {
		http.Error(w, "not ready", http.StatusNotFound)
		return
	}
	f, err := os.Open(job.OutputDir)
	if err != nil {
		s.fail(w, err)
		return
	}
	defer func() { _ = f.Close() }()

	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Content-Disposition",
		"attachment; filename=\""+filepath.Base(job.OutputDir)+"\"")

	var mod time.Time
	if st, err := os.Stat(job.OutputDir); err == nil {
		mod = st.ModTime()
	}
	http.ServeContent(w, r, filepath.Base(job.OutputDir), mod, f)
}

func (s *Server) getStorageUsage(w http.ResponseWriter, r *http.Request) {
	if s.downloads == nil {
		writeJSON(w, map[string]any{"bytes": 0, "formatted": "0 B", "files": 0})
		return
	}
	b, files, err := s.downloads.GetStorageUsage()
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
	if s.downloads == nil {
		http.Error(w, "downloader unavailable", http.StatusServiceUnavailable)
		return
	}
	freed, files, err := s.downloads.CleanStorage()
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

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
