package downloader

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"gemfactory/internal/model"
)

// ClipRequest describes a single clip job.
type ClipRequest struct {
	URL       string `json:"url"`
	Start     string `json:"start,omitempty"`
	End       string `json:"end,omitempty"`
	SubsLang  string `json:"subs_lang,omitempty"`  // empty = no subs
	Quality   int    `json:"quality,omitempty"`    // max height, default 1080
	HQ        bool   `json:"hq,omitempty"`         // up to 2K
	GIF       bool   `json:"gif,omitempty"`        // drop audio track
	Shorts    bool   `json:"shorts,omitempty"`     // download whole short, best quality
	AudioOnly bool   `json:"audio_only,omitempty"` // extract mp3
}

// JobStatus is the lifecycle state of a download job.
type JobStatus string

const (
	StatusPending     JobStatus = "pending"
	StatusDownloading JobStatus = "downloading"
	StatusProcessing  JobStatus = "processing"
	StatusDone        JobStatus = "done"
	StatusError       JobStatus = "error"
)

// Job tracks progress of one clip request.
type Job struct {
	ID        string      `json:"id"`
	VideoID   string      `json:"video_id"`
	Request   ClipRequest `json:"request"`
	Status    JobStatus   `json:"status"`
	Error     string      `json:"error,omitempty"`
	OutputDir string      `json:"output_dir,omitempty"`
	Caption   string      `json:"caption,omitempty"`

	callbacks *ClipCallbacks `json:"-"`

	// Progress throttle state, guarded by Service.mu.
	lastProgStage   string
	lastProgPercent int
	lastProgSpeed   string
	lastProgAt      time.Time
}

type Service struct {
	cookies model.CookieRepository
	configs model.ConfigRepository
	dataDir string
	logger  *zap.Logger

	mu    sync.Mutex
	jobs  map[string]*Job
	order []string
	sem   chan struct{}
}

func NewService(cookies model.CookieRepository, dataDir string, concurrency int, logger *zap.Logger) *Service {
	if concurrency < 1 {
		concurrency = 1
	}
	return &Service{
		cookies: cookies,
		dataDir: filepath.Join(dataDir, "downloads", "youtube"),
		logger:  logger,
		jobs:    make(map[string]*Job),
		sem:     make(chan struct{}, concurrency),
	}
}

func (s *Service) SetConfigRepo(configs model.ConfigRepository) {
	s.configs = configs
}

func (s *Service) Submit(ctx context.Context, req ClipRequest) (*Job, error) {
	return s.SubmitWithCallbacks(ctx, req, nil)
}

func (s *Service) SubmitWithCallbacks(ctx context.Context, req ClipRequest, cbs *ClipCallbacks) (*Job, error) {
	videoID := ""
	if !req.Shorts && (req.Start != "" || req.End != "") {
		startMs, err := ParseTimecode(req.Start)
		if err != nil {
			return nil, fmt.Errorf("invalid start: %w", err)
		}
		endMs, err := ParseTimecode(req.End)
		if err != nil {
			return nil, fmt.Errorf("invalid end: %w", err)
		}
		if endMs <= startMs {
			return nil, fmt.Errorf("end %s must be after start %s", req.End, req.Start)
		}
		if maxDur := s.maxSegmentDuration(req.HQ); endMs-startMs > maxDur {
			return nil, fmt.Errorf(
				"interval %s-%s is too long (%.0f s), maximum is %.0f seconds",
				req.Start, req.End, (endMs-startMs)/1000, maxDur/1000)
		}
	}

	if req.URL == "" {
		return nil, fmt.Errorf("url is required")
	}
	videoID, err := videoIDFromURL(req.URL)
	if err != nil {
		return nil, err
	}
	if req.Quality == 0 && !req.HQ && !req.Shorts {
		req.Quality = 1080
	}

	variant := variantSuffix(req)
	var jobID string
	if req.Shorts || (req.Start == "" && req.End == "") {
		jobID = videoID + "_full" + variant
	} else {
		jobID = fmt.Sprintf("%s_%s-%s%s", videoID, req.Start, req.End, variant)
	}

	job := &Job{
		ID:      jobID,
		VideoID: videoID,
		Request: req,
		Status:  StatusPending,
	}

	s.mu.Lock()
	if existing, ok := s.jobs[job.ID]; ok {
		switch existing.Status {
		case StatusError:
			// Failed run: drop it so this submit starts a fresh attempt.
			delete(s.jobs, job.ID)
		case StatusDone:
			// Already produced: serve instantly through the new callbacks.
			existing.callbacks = cbs
			output := existing.OutputDir
			caption := existing.Caption
			s.mu.Unlock()
			if cbs != nil && cbs.OnDone != nil {
				go cbs.OnDone(output, caption)
			}
			return existing, nil
		default:
			// Still running: re-point callbacks so the new status message keeps receiving stage/progress updates.
			existing.callbacks = cbs
			s.mu.Unlock()
			if cbs != nil {
				if cbs.OnStage != nil {
					s.snapshotStage(existing, cbs)
				}
			}
			return existing, nil
		}
	}
	s.jobs[job.ID] = job
	s.order = append(s.order, job.ID)
	if cbs != nil {
		job.callbacks = cbs
	}
	s.mu.Unlock()

	go s.run(context.WithoutCancel(ctx), job)
	return job, nil
}

func (s *Service) GetJob(id string) (*Job, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[id]
	return job, ok
}

func (s *Service) ListJobs() []*Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*Job, 0, len(s.order))
	for _, id := range s.order {
		out = append(out, s.jobs[id])
	}
	return out
}

// ProgressUpdate holds real-time progress details for heavy stages (download / re-encode).
type ProgressUpdate struct {
	Stage   string
	Percent int
	Speed   string // e.g. "4.12MiB/s" or "2.45x"
	ETA     string // e.g. "00:03"
	Size    string // e.g. "25.40MiB"
	Detail  string // e.g. "Burning subtitles" or "Extracting MP3"
}

// ClipCallbacks receives progress events for one job. All fields optional.
type ClipCallbacks struct {
	OnStage    func(stage, detail string)
	OnProgress func(p ProgressUpdate) // throttled
	OnDone     func(path, caption string)
	OnError    func(message string) // user-facing text
}

// Stage names reported via ClipCallbacks.OnStage.
const (
	StageMetadata  = "metadata"
	StageSubtitles = "subtitles"
	StageDownload  = "download"
	StageReencode  = "reencode"
	StageUpload    = "upload"
)

func (s *Service) run(ctx context.Context, job *Job) {
	s.sem <- struct{}{}
	defer func() { <-s.sem }()

	// Serve instantly when this exact request was produced before.
	clipPath := s.outputPathFor(job)
	if marker := readCacheHit(clipPath); marker != nil {
		s.serveFromCache(job, clipPath, marker)
		return
	}

	s.setStatus(job, StatusDownloading)
	s.reportStage(job, StageDownload, "")

	cookieFile, cleanup, err := s.prepareCookieFile(ctx, "youtube.com")
	if err != nil {
		s.logger.Warn("cookies unavailable, continuing without", zap.Error(err))
	} else {
		defer cleanup()
	}

	s.reportStage(job, StageMetadata, "")
	meta, err := s.ExtractMetadata(ctx, job.Request.URL, job.VideoID, cookieFile)
	if err != nil {
		s.fail(job, "metadata extraction failed: "+err.Error())
		return
	}
	s.reportStage(job, StageMetadata, meta.Title)

	if job.Request.AudioOnly {
		s.reportStage(job, StageDownload, "")
		hasSections := job.Request.Start != "" && job.Request.End != ""
		rawMedia := filepath.Join(s.workbenchDir(job.VideoID), job.ID+"_raw.mp4")
		if _, err := os.Stat(clipPath); os.IsNotExist(err) {
			if _, err := os.Stat(rawMedia); os.IsNotExist(err) {
				if hasSections {
					if err := s.downloadSegment(ctx, job.Request, rawMedia, cookieFile, func(p ProgressUpdate) {
						s.reportProgress(job, p)
					}); err != nil {
						s.fail(job, "download failed: "+err.Error())
						return
					}
				} else {
					if err := s.downloadFullVideo(ctx, job.Request, rawMedia, cookieFile, func(p ProgressUpdate) {
						s.reportProgress(job, p)
					}); err != nil {
						s.fail(job, "download failed: "+err.Error())
						return
					}
				}
			}
			s.setStatus(job, StatusProcessing)
			s.reportStage(job, StageReencode, "mp3")
			var durMS float64
			if hasSections {
				sMs, eMs := mustParsePair(job.Request.Start, job.Request.End)
				durMS = eMs - sMs
			} else if meta.Duration > 0 {
				durMS = meta.Duration * 1000.0
			}
			if err := s.ExtractAudioMP3(ctx, rawMedia, clipPath, durMS, func(p ProgressUpdate) {
				s.reportProgress(job, p)
			}); err != nil {
				s.fail(job, "mp3 conversion failed: "+err.Error())
				return
			}
		}
	} else if job.Request.Shorts || (job.Request.Start == "" && job.Request.End == "") {
		s.reportStage(job, StageDownload, "")
		if _, err := os.Stat(clipPath); os.IsNotExist(err) {
			if err := s.downloadFullVideo(ctx, job.Request, clipPath, cookieFile, func(p ProgressUpdate) {
				s.reportProgress(job, p)
			}); err != nil {
				s.fail(job, "download failed: "+err.Error())
				return
			}
		}
		s.setStatus(job, StatusProcessing)
		s.reportStage(job, StageReencode, "")
		if finalPath, rerr := s.ReencodeWithSubs(ctx, clipPath, "", false, 0, nil); rerr == nil {
			clipPath = finalPath
		}
	} else {
		startMs, endMs := mustParsePair(job.Request.Start, job.Request.End)

		var trimmedVTT string
		if lang := job.Request.SubsLang; lang != "" {
			s.setStatus(job, StatusProcessing)
			s.reportStage(job, StageSubtitles, lang)
			trimmedVTT, err = s.subtitlesForClip(ctx, meta, job, lang, cookieFile, startMs, endMs)
			if err != nil {
				// User explicitly asked for subs; don't deliver a clip without them.
				s.fail(job, "subtitles unavailable ("+lang+"): "+err.Error())
				return
			}
		}

		s.setStatus(job, StatusDownloading)
		s.reportStage(job, StageDownload, "")
		if _, err := os.Stat(clipPath); os.IsNotExist(err) {
			if err := s.downloadSegment(ctx, job.Request, clipPath, cookieFile, func(p ProgressUpdate) {
				s.reportProgress(job, p)
			}); err != nil {
				s.fail(job, "download failed: "+err.Error())
				return
			}
		}

		s.setStatus(job, StatusProcessing)
		s.reportStage(job, StageReencode, "")
		finalPath, rerr := s.ReencodeWithSubs(ctx, clipPath, trimmedVTT, job.Request.GIF, endMs-startMs, func(p ProgressUpdate) {
			s.reportProgress(job, p)
		})
		if rerr != nil {
			s.fail(job, "re-encode failed: "+rerr.Error())
			return
		}
		clipPath = finalPath
	}

	if err := checkFileSize(clipPath); err != nil {
		s.fail(job, err.Error())
		return
	}

	var caption string
	if job.Request.Shorts {
		caption = FormatCaption(meta)
	}

	writeCacheMarker(clipPath, meta.Title, caption)
	s.mu.Lock()
	job.Status = StatusDone
	job.OutputDir = clipPath
	job.Caption = caption
	s.mu.Unlock()
	s.reportStage(job, StageUpload, "")
	s.notifyDone(job, clipPath, caption)
	s.logger.Info("clip ready", zap.String("job", job.ID), zap.String("output", clipPath))
}

func (s *Service) outputPathFor(job *Job) string {
	variant := variantSuffix(job.Request)
	ext := ".mp4"
	if job.Request.AudioOnly {
		ext = ".mp3"
	}
	if job.Request.Shorts || (job.Request.Start == "" && job.Request.End == "") {
		return filepath.Join(s.workbenchDir(job.VideoID), job.VideoID+"_full"+variant+ext)
	}
	startMs, endMs := mustParsePair(job.Request.Start, job.Request.End)
	return filepath.Join(s.workbenchDir(job.VideoID),
		fileNameWithTimecodeVariant(job.VideoID, startMs, endMs, variant)+ext)
}

func (s *Service) serveFromCache(job *Job, clipPath string, marker *cacheMarker) {
	s.logger.Info("cache hit", zap.String("job", job.ID), zap.String("output", clipPath))
	s.reportStage(job, StageMetadata, marker.Title)
	s.reportStage(job, StageUpload, "")

	s.mu.Lock()
	job.Status = StatusDone
	job.OutputDir = clipPath
	job.Caption = marker.Caption
	s.mu.Unlock()
	s.notifyDone(job, clipPath, marker.Caption)
}

func (s *Service) reportStage(job *Job, stage, detail string) {
	s.mu.Lock()
	cbs := job.callbacks
	s.mu.Unlock()
	if cbs != nil && cbs.OnStage != nil {
		cbs.OnStage(stage, detail)
	}
}

func (s *Service) notifyDone(job *Job, path, caption string) {
	s.mu.Lock()
	cbs := job.callbacks
	s.mu.Unlock()
	if cbs != nil && cbs.OnDone != nil {
		cbs.OnDone(path, caption)
	}
}

func (s *Service) reportProgress(job *Job, p ProgressUpdate) {
	s.mu.Lock()
	cbs := job.callbacks
	if cbs == nil || cbs.OnProgress == nil {
		s.mu.Unlock()
		return
	}
	now := time.Now()
	shouldEmit := job.lastProgStage != p.Stage ||
		now.Sub(job.lastProgAt) >= 1500*time.Millisecond &&
			(abs(p.Percent-job.lastProgPercent) >= 5 || p.Speed != job.lastProgSpeed) ||
		p.Percent >= 100 && job.lastProgPercent < 100
	if shouldEmit {
		job.lastProgStage = p.Stage
		job.lastProgPercent = p.Percent
		job.lastProgSpeed = p.Speed
		job.lastProgAt = now
	}
	s.mu.Unlock()

	if shouldEmit {
		cbs.OnProgress(p)
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func (s *Service) snapshotStage(job *Job, cbs *ClipCallbacks) {
	s.mu.Lock()
	status := job.Status
	lastPct := job.lastProgPercent
	lastSpd := job.lastProgSpeed
	s.mu.Unlock()

	switch status {
	case StatusDownloading:
		cbs.OnStage(StageDownload, "")
		if lastPct > 0 {
			cbs.OnProgress(ProgressUpdate{Stage: StageDownload, Percent: lastPct, Speed: lastSpd})
		}
	case StatusProcessing:
		cbs.OnStage(StageReencode, "")
		if lastPct > 0 {
			cbs.OnProgress(ProgressUpdate{Stage: StageReencode, Percent: lastPct, Speed: lastSpd})
		}
	case StatusPending:
		cbs.OnStage(StageMetadata, "")
	}
}

func (s *Service) setStatus(job *Job, status JobStatus) {
	s.mu.Lock()
	job.Status = status
	s.mu.Unlock()
}

func (s *Service) fail(job *Job, msg string) {
	s.mu.Lock()
	job.Status = StatusError
	job.Error = msg
	cbs := job.callbacks
	s.mu.Unlock()
	s.logger.Error("clip job failed", zap.String("job", job.ID), zap.String("error", msg))

	if cbs != nil && cbs.OnError != nil {
		cbs.OnError(FriendlyError(msg))
	}
}

func (s *Service) prepareCookieFile(ctx context.Context, domain string) (string, func(), error) {
	noop := func() {}
	if s.cookies == nil {
		return "", noop, fmt.Errorf("cookie repository is not configured")
	}
	cookie, err := s.cookies.GetByDomain(ctx, domain)
	if err != nil {
		return "", noop, err
	}
	if cookie == nil || cookie.Content == "" {
		return "", noop, fmt.Errorf("no cookies stored for %s", domain)
	}
	f, err := os.CreateTemp("", "ytdlp-cookies-*.txt")
	if err != nil {
		return "", noop, err
	}
	if _, err := f.WriteString(cookie.Content); err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		return "", noop, err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(f.Name())
		return "", noop, err
	}
	cleanup := func() { _ = os.Remove(f.Name()) }
	return f.Name(), cleanup, nil
}

func (s *Service) workbenchDir(videoID string) string {
	dir := filepath.Join(s.dataDir, videoID)
	_ = os.MkdirAll(dir, 0755)
	return dir
}

func (s *Service) GetStorageUsage() (totalBytes int64, fileCount int, err error) {
	s.mu.Lock()
	dir := s.dataDir
	s.mu.Unlock()

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return 0, 0, nil
	}

	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			totalBytes += info.Size()
			fileCount++
		}
		return nil
	})
	return totalBytes, fileCount, err
}

func (s *Service) CleanStorage() (freedBytes int64, removedFiles int, err error) {
	s.mu.Lock()
	dir := s.dataDir
	s.mu.Unlock()

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return 0, 0, nil
	}

	freedBytes, removedFiles, _ = s.GetStorageUsage()

	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, 0, err
	}
	for _, entry := range entries {
		_ = os.RemoveAll(filepath.Join(dir, entry.Name()))
	}

	s.logger.Info("Downloads storage cleaned",
		zap.Int64("freed_bytes", freedBytes),
		zap.Int("removed_files", removedFiles),
	)
	return freedBytes, removedFiles, nil
}

// maxSegmentDuration returns the max clip length in ms: 30s for HQ, 300s otherwise.
func (s *Service) maxSegmentDuration(hq bool) float64 {
	if hq {
		if v := durationEnvMs("CLIP_MAX_DURATION_HQ_SECONDS"); v > 0 {
			return v
		}
		return 30 * 1000
	}
	if v := durationEnvMs("CLIP_MAX_DURATION_SECONDS"); v > 0 {
		return v
	}
	return 300 * 1000
}

func durationEnvMs(key string) float64 {
	raw := os.Getenv(key)
	if raw == "" {
		return 0
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || v <= 0 {
		return 0
	}
	return v * 1000
}

func (s *Service) MaxSegmentDurationSeconds(hq bool) float64 {
	return s.maxSegmentDuration(hq) / 1000
}

func telegramFileLimitBytes() int64 {
	const defMB = 49
	raw := os.Getenv("TG_FILE_LIMIT_MB")
	mb := defMB
	if raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			mb = v
		}
	}
	return int64(mb) * 1024 * 1024
}

func checkFileSize(path string) error {
	st, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("output file missing: %w", err)
	}
	limit := telegramFileLimitBytes()
	if st.Size() > limit {
		return fmt.Errorf(
			"file is too large: %.1f MB (Telegram bot limit is %d MB). Try a shorter interval",
			float64(st.Size())/(1024*1024), limit/(1024*1024))
	}
	return nil
}

func mustParsePair(start, end string) (float64, float64) {
	s, _ := ParseTimecode(start)
	e, _ := ParseTimecode(end)
	return s, e
}

// FriendlyError converts raw internal error text into a short actionable Telegram message.
func FriendlyError(raw string) string {
	switch {
	case strings.Contains(raw, "Sign in to confirm you're not a bot"),
		strings.Contains(raw, "Sign in to confirm you are not a bot"):
		return "YouTube requires authentication from this server. Add youtube.com cookies in the web panel and try again."

	case strings.Contains(raw, "No supported JavaScript runtime"):
		return "No JavaScript runtime (deno/node) available for yt-dlp. Rebuild the image: docker compose build."

	case strings.Contains(raw, "this video has no subtitles"),
		strings.Contains(raw, "no subtitles available"),
		strings.Contains(raw, "no subtitles found"):
		return "This video has no subtitles."

	case strings.Contains(raw, "is too long"):
		return raw

	case strings.Contains(raw, "The page needs to be reloaded"):
		return "Cookies expired or revoked by YouTube. Export fresh cookies and update them in the web panel."

	case strings.Contains(raw, "Private video"),
		strings.Contains(raw, "members-only"),
		strings.Contains(raw, "Join this channel"):
		return "Video is private or members-only. Cookies from an authorized account are required."

	case strings.Contains(raw, "Video unavailable"),
		strings.Contains(raw, "removed by the uploader"):
		return "Video is unavailable or removed."

	default:
		msg := strings.TrimSpace(raw)
		if len(msg) > 300 {
			msg = msg[:300] + "..."
		}
		return "Error: " + msg
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "...(truncated)"
}
