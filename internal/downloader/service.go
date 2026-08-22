package downloader

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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
	SubsLang  string `json:"subs_lang,omitempty"` // empty = no subs
	Quality   int    `json:"quality,omitempty"`   // max height, default 1080
	HQ        bool   `json:"hq,omitempty"`        // up to 2K
	GIF       bool   `json:"gif,omitempty"`       // drop audio track
	Shorts    bool   `json:"shorts,omitempty"`    // download whole short, best quality
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
	lastProgAt      time.Time
}

// Service orchestrates yt-dlp/ffmpeg clip downloads.
type Service struct {
	cookies model.CookieRepository
	dataDir string
	logger  *zap.Logger

	mu    sync.Mutex
	jobs  map[string]*Job
	order []string
	sem   chan struct{}
}

// NewService creates a downloader service writing into dataDir/downloads.
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

// Submit enqueues a clip request and returns its job.
func (s *Service) Submit(ctx context.Context, req ClipRequest) (*Job, error) {
	return s.SubmitWithCallbacks(ctx, req, nil)
}

// SubmitWithCallbacks enqueues a clip request with progress callbacks.
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
				"интервал %s-%s слишком длинный (%.0f c), максимум %.0f секунд",
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
			// Still running: re-point callbacks so the new status message
			// keeps receiving stage/progress updates.
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

// GetJob returns a job by id.
func (s *Service) GetJob(id string) (*Job, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[id]
	return job, ok
}

// ListJobs returns all jobs in submission order.
func (s *Service) ListJobs() []*Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*Job, 0, len(s.order))
	for _, id := range s.order {
		out = append(out, s.jobs[id])
	}
	return out
}

// ClipCallbacks receives progress events for one job. All fields optional.
type ClipCallbacks struct {
	// OnStage is called on stage transitions with an optional human detail.
	OnStage func(stage, detail string)
	// OnProgress reports 0..100 completion of the current heavy stage
	// (download/re-encode), throttled to avoid Telegram rate limits.
	OnProgress func(stage string, percent int)
	// OnDone is called once with the final file path and caption.
	OnDone func(path, caption string)
	// OnError is called with a user-facing error message.
	OnError func(message string)
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

	// Fast path: this exact request (id + interval + variant) was produced
	// before and its marker still exists — serve it from cache instantly.
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
		// Audio extraction mode (full audio or clipped interval).
		s.reportStage(job, StageDownload, "")
		hasSections := job.Request.Start != "" && job.Request.End != ""
		rawMedia := filepath.Join(s.workbenchDir(job.VideoID), job.ID+"_raw.mp4")
		if _, err := os.Stat(clipPath); os.IsNotExist(err) {
			if _, err := os.Stat(rawMedia); os.IsNotExist(err) {
				if hasSections {
					if err := s.downloadSegment(ctx, job.Request, rawMedia, cookieFile, func(pct int) {
						s.reportProgress(job, StageDownload, pct)
					}); err != nil {
						s.fail(job, "download failed: "+err.Error())
						return
					}
				} else {
					if err := s.downloadFullVideo(ctx, job.Request, rawMedia, cookieFile, func(pct int) {
						s.reportProgress(job, StageDownload, pct)
					}); err != nil {
						s.fail(job, "download failed: "+err.Error())
						return
					}
				}
			}
			s.setStatus(job, StatusProcessing)
			s.reportStage(job, StageReencode, "mp3")
			if err := s.ExtractAudioMP3(ctx, rawMedia, clipPath); err != nil {
				s.fail(job, "mp3 conversion failed: "+err.Error())
				return
			}
		}
	} else if job.Request.Shorts || (job.Request.Start == "" && job.Request.End == "") {
		// Shorts / TikTok / Full videos are downloaded whole at best quality without sections.
		s.reportStage(job, StageDownload, "")
		if _, err := os.Stat(clipPath); os.IsNotExist(err) {
			if err := s.downloadFullVideo(ctx, job.Request, clipPath, cookieFile, func(pct int) {
				s.reportProgress(job, StageDownload, pct)
			}); err != nil {
				s.fail(job, "download failed: "+err.Error())
				return
			}
		}
		// Fast remux to ensure +faststart and device compatibility.
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
				// The user explicitly requested subtitles; silently delivering
				// a clip without them is confusing, so fail the job instead.
				s.fail(job, "subtitles unavailable ("+lang+"): "+err.Error())
				return
			}
		}

		s.setStatus(job, StatusDownloading)
		s.reportStage(job, StageDownload, "")
		if _, err := os.Stat(clipPath); os.IsNotExist(err) {
			if err := s.downloadSegment(ctx, job.Request, clipPath, cookieFile, func(pct int) {
				s.reportProgress(job, StageDownload, pct)
			}); err != nil {
				s.fail(job, "download failed: "+err.Error())
				return
			}
		}

		s.setStatus(job, StatusProcessing)
		s.reportStage(job, StageReencode, "")
		finalPath, rerr := s.ReencodeWithSubs(ctx, clipPath, trimmedVTT, job.Request.GIF, endMs-startMs, func(pct int) {
			s.reportProgress(job, StageReencode, pct)
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

// outputPathFor computes the deterministic final file path for a job:
// <videoID>/<videoID>_<start>-<end><variant>.mp4 or <videoID>/<videoID>_full<variant>.mp4 (or .mp3).
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

// serveFromCache completes a job immediately using a previously made file.
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

// reportStage notifies the job callbacks about a stage transition.
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

// reportProgress forwards download/encode progress to callbacks, throttled
// so Telegram messages are edited at most once per 2 seconds or 5% step.
func (s *Service) reportProgress(job *Job, stage string, percent int) {
	s.mu.Lock()
	cbs := job.callbacks
	if cbs == nil || cbs.OnProgress == nil {
		s.mu.Unlock()
		return
	}
	now := time.Now()
	shouldEmit := job.lastProgStage != stage ||
		now.Sub(job.lastProgAt) >= 2*time.Second &&
			abs(percent-job.lastProgPercent) >= 5 ||
		percent >= 100 && job.lastProgPercent < 100
	if shouldEmit {
		job.lastProgStage = stage
		job.lastProgPercent = percent
		job.lastProgAt = now
	}
	s.mu.Unlock()

	if shouldEmit {
		cbs.OnProgress(stage, percent)
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// snapshotStage reports the job's current position into freshly attached
// callbacks so a repeated command shows real progress instead of a hang.
func (s *Service) snapshotStage(job *Job, cbs *ClipCallbacks) {
	s.mu.Lock()
	status := job.Status
	lastPct := job.lastProgPercent
	s.mu.Unlock()

	switch status {
	case StatusDownloading:
		cbs.OnStage(StageDownload, "")
		if lastPct > 0 {
			cbs.OnProgress(StageDownload, lastPct)
		}
	case StatusProcessing:
		cbs.OnStage(StageReencode, "")
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

// prepareCookieFile writes domain cookies to a temp Netscape file for yt-dlp.
// Returns the file path and a cleanup func; both are empty when no cookies exist.
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

// GetStorageUsage calculates total bytes and file count in the downloads directory.
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

// CleanStorage removes all cached files inside the downloads directory.
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

