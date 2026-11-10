package downloader

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"log/slog"

	"gemfactory/internal/model"
	"gemfactory/internal/settings"
)

// ClipRequest describes a single clip job.
type ClipRequest struct {
	URL       string
	Start     string
	End       string
	SubsLang  string // empty = no subs
	SubsNoLLM bool   // translate via YouTube auto-translate only
	Quality   string // e.g. "720p", "720", "1080p"
	HQ        bool   // up to 2K
	GIF       bool   // drop audio track
	Shorts    bool   // download whole short, best quality
	AudioOnly bool   // extract mp3
	NoDesc    bool   // send the result without a caption
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

// Active reports whether a job in this state is still being worked on.
func (s JobStatus) Active() bool {
	return s == StatusPending || s == StatusDownloading || s == StatusProcessing
}

// Job tracks progress of one clip request.
type Job struct {
	ID          string
	VideoID     string
	Request     ClipRequest
	Status      JobStatus
	Error       string
	OutputDir   string
	Caption     string
	Translation string

	callbacks *ClipCallbacks

	// Progress throttle state, guarded by Service.mu.
	lastProgStage   string
	lastProgPercent int
	lastProgSpeed   string
	lastProgAt      time.Time

	finishedAt time.Time // when the job became StatusDone

	// Last reported pipeline stage (with its detail), for callback re-attachment.
	lastStage       string
	lastStageDetail string
}

type Service struct {
	cookies model.CookieRepository
	cfg     *settings.Provider
	dataDir string
	logger  *slog.Logger

	mu          sync.Mutex
	cond        *sync.Cond
	concurrency int
	activeJobs  int
	jobs        map[string]*Job
	order       []string
}

func NewService(cookies model.CookieRepository, cfg *settings.Provider, dataDir string, concurrency int, logger *slog.Logger) *Service {
	if concurrency < 1 {
		concurrency = 1
	}
	s := &Service{
		cookies:     cookies,
		cfg:         cfg,
		dataDir:     filepath.Join(dataDir, "downloads", "youtube"),
		logger:      logger,
		jobs:        make(map[string]*Job),
		concurrency: concurrency,
	}
	s.cond = sync.NewCond(&s.mu)
	return s
}

func (s *Service) SetConcurrency(n int) {
	if n < 1 {
		n = 1
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.concurrency = n
	s.cond.Broadcast()
	s.logger.Info("downloader concurrency updated", slog.Int("concurrency", n))
}

func (s *Service) acquireSlot() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for s.activeJobs >= s.concurrency {
		s.cond.Wait()
	}
	s.activeJobs++
}

func (s *Service) releaseSlot() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activeJobs--
	s.cond.Broadcast()
}

func (s *Service) SubmitWithCallbacks(ctx context.Context, req ClipRequest, cbs *ClipCallbacks) (*Job, error) {
	var startMs, endMs float64
	if req.Start != "" || req.End != "" {
		var err error
		if startMs, err = ParseTimecode(req.Start); err != nil {
			return nil, fmt.Errorf("invalid start: %w", err)
		}
		if endMs, err = ParseTimecode(req.End); err != nil {
			return nil, fmt.Errorf("invalid end: %w", err)
		}
		if endMs <= startMs {
			return nil, fmt.Errorf("end %s must be after start %s", req.End, req.Start)
		}
	}

	if req.URL == "" {
		return nil, fmt.Errorf("url is required")
	}
	videoID, err := videoIDFromURL(req.URL)
	if err != nil {
		return nil, err
	}
	if req.Quality == "" && !req.HQ && !req.Shorts {
		req.Quality = "1080p"
	}

	job := &Job{
		ID:      clipName(videoID, req, startMs, endMs),
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
			// Posts carry several files; rerunning hits the post manifest and delivers all of them.
			if existing.OutputDir != "" && !isPostJob(existing.Request) {
				if _, err := os.Stat(existing.OutputDir); err == nil {
					// Already produced & file exists on disk: serve instantly through callbacks.
					existing.callbacks = cbs
					output := existing.OutputDir
					caption := existing.Caption
					snap := snapshot(existing)
					s.mu.Unlock()
					if cbs != nil && cbs.OnDone != nil {
						go cbs.OnDone(output, caption)
					}
					return snap, nil
				}
			}
			// File was deleted from disk or OutputDir is empty: drop stale job so fresh attempt starts.
			delete(s.jobs, job.ID)
		default:
			// Still running: re-point callbacks so the new status message keeps receiving stage/progress updates.
			existing.callbacks = cbs
			snap := snapshot(existing)
			s.mu.Unlock()
			if cbs != nil && cbs.OnStage != nil {
				s.snapshotStage(existing, cbs)
			}
			return snap, nil
		}
	}
	s.jobs[job.ID] = job
	s.order = append(s.order, job.ID)
	if cbs != nil {
		job.callbacks = cbs
	}
	snap := snapshot(job)
	s.mu.Unlock()

	go s.run(context.WithoutCancel(ctx), job)
	return snap, nil
}

// snapshot copies a job for callers outside the service; the caller must hold s.mu.
func snapshot(job *Job) *Job {
	c := *job
	c.callbacks = nil
	return &c
}

// callbacksOf returns the job's current callbacks; a resubmit may re-point them at any time.
func (s *Service) callbacksOf(job *Job) *ClipCallbacks {
	s.mu.Lock()
	defer s.mu.Unlock()
	return job.callbacks
}

func (s *Service) setTranslation(job *Job, providerModel string) {
	s.mu.Lock()
	job.Translation = providerModel
	s.mu.Unlock()
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
	OnStage            func(stage, detail string)
	OnHashtags         func(tags []string)
	OnTranslation      func(providerModel string)
	OnTranslateAttempt func(providerModel string) // fired before the Gemini request
	OnProgress         func(p ProgressUpdate)     // throttled
	OnDone             func(path, caption string)
	OnPostDone         func(media []PostMedia, caption string) // Instagram/X posts; falls back to OnDone with the first item
	OnError            func(message string)                    // user-facing text
}

// Stage names reported via ClipCallbacks.OnStage.
const (
	StageQueued    = "queued"
	StageMetadata  = "metadata"
	StageSubtitles = "subtitles"
	StageTranslate = "translate"
	StageDownload  = "download"
	StageReencode  = "reencode"
	StageUpload    = "upload"
)

func (s *Service) run(ctx context.Context, job *Job) {
	s.reportStage(job, StageQueued, "")
	s.acquireSlot()
	defer s.releaseSlot()

	if isPostJob(job.Request) {
		s.runPost(ctx, job)
		return
	}

	// Serve instantly when this exact request was produced before.
	clipPath := s.outputPathFor(job)
	if marker := readCacheHit(clipPath); marker != nil {
		s.serveFromCache(job, clipPath, marker)
		return
	}

	s.setStatus(job, StatusDownloading)
	s.reportStage(job, StageDownload, "")

	cookieFile, cleanup, err := s.prepareCookieFile(ctx, cookieDomainFor(job.Request.URL))
	if err != nil {
		s.logger.Warn("cookies unavailable, continuing without", slog.Any("error", err))
	} else {
		defer cleanup()
	}

	s.reportStage(job, StageMetadata, "")
	meta, err := s.ExtractMetadata(ctx, job.Request.URL, job.VideoID, cookieFile)
	if err != nil {
		s.fail(job, "metadata extraction failed: "+err.Error())
		return
	}
	s.reportStage(job, StageMetadata, DisplayTitle(meta))
	if cbs := s.callbacksOf(job); cbs != nil && cbs.OnHashtags != nil {
		cbs.OnHashtags(AllHashtags(meta))
	}

	// Clamp the requested interval to the real video length before any work.
	full := job.Request.Start == "" && job.Request.End == ""
	var startMs, endMs float64
	if !full {
		startMs, endMs, err = s.resolveSegmentBounds(ctx, meta, job.Request)
		if err != nil {
			s.fail(job, err.Error())
			return
		}
		if clamped := s.outputPath(job, startMs, endMs); clamped != clipPath {
			clipPath = clamped
			if marker := readCacheHit(clipPath); marker != nil {
				s.serveFromCache(job, clipPath, marker)
				return
			}
		}
	}
	progress := func(p ProgressUpdate) { s.reportProgress(job, p) }
	// fetch downloads the source into out unless it is already there; false means the job failed.
	fetch := func(out string) bool {
		if _, err := os.Stat(out); !os.IsNotExist(err) {
			return true
		}
		if err := s.fetchMedia(ctx, job, dlOpts{req: job.Request, out: out, cookie: cookieFile,
			startMs: startMs, endMs: endMs, vertical: meta.IsVertical(), onProgress: progress}); err != nil {
			s.fail(job, "download failed: "+err.Error())
			return false
		}
		return true
	}

	switch {
	case job.Request.AudioOnly:
		s.reportStage(job, StageDownload, "")
		rawMedia := filepath.Join(s.workbenchDir(job.VideoID), job.ID+"_raw.mp4")
		if _, err := os.Stat(clipPath); os.IsNotExist(err) {
			if !fetch(rawMedia) {
				return
			}
			s.setStatus(job, StatusProcessing)
			s.reportStage(job, StageReencode, "mp3")
			durMS := endMs - startMs
			if full {
				durMS = max(meta.Duration*1000, 0)
			}
			if err := s.ExtractAudioMP3(ctx, rawMedia, clipPath, durMS, progress); err != nil {
				s.fail(job, "mp3 conversion failed: "+err.Error())
				return
			}
		}
	case full:
		s.reportStage(job, StageDownload, "")
		if !fetch(clipPath) {
			return
		}
		s.setStatus(job, StatusProcessing)
		s.reportStage(job, StageReencode, "")
		if finalPath, rerr := s.ReencodeWithSubs(ctx, clipPath, "", false, 0, nil); rerr == nil {
			clipPath = finalPath
		}
	default:
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
		if !fetch(clipPath) {
			return
		}

		s.setStatus(job, StatusProcessing)
		s.reportStage(job, StageReencode, "")
		finalPath, rerr := s.ReencodeWithSubs(ctx, clipPath, trimmedVTT, job.Request.GIF, endMs-startMs, progress)
		if rerr != nil {
			s.fail(job, "re-encode failed: "+rerr.Error())
			return
		}
		clipPath = finalPath
	}

	if err := checkFileSize(clipPath, s.telegramFileLimitBytes(ctx)); err != nil {
		s.fail(job, err.Error())
		return
	}

	var caption string
	if job.Request.Shorts && full {
		caption = FormatCaption(DisplayTitle(meta), CaptionHashtags(meta))
	}

	s.mu.Lock()
	translation := job.Translation
	s.mu.Unlock()
	baseTitle, altTitle := displayTitleParts(meta)
	writeCacheMarker(clipPath, cacheMarker{
		Title:       baseTitle,
		AltTitle:    altTitle,
		Tags:        AllHashtags(meta),
		Translation: translation,
	})
	s.markDone(job, clipPath, caption)
	s.reportStage(job, StageUpload, "")
	s.notifyDone(job, clipPath, caption)
	s.logger.Info("clip ready", slog.String("job", job.ID), slog.String("output", clipPath))
}

// fetchMedia downloads the job's section (or the whole video when the bounds are empty) and retries
// with the fallback fetch path when the result turns out to be an incomplete stream.
func (s *Service) fetchMedia(ctx context.Context, job *Job, o dlOpts) error {
	err := s.download(ctx, o)
	if errors.Is(err, ErrIncompleteStream) {
		s.logger.Warn("incomplete stream detected, retrying with fallback fetch strategy", slog.String("job", job.ID), slog.Any("error", err))
		_ = os.Remove(o.out)
		o.fallback = true
		err = s.download(ctx, o)
	}
	return err
}

// clipName is the base name shared by job IDs and output files: "<id>_full<variant>" or "<id>_<start>-<end><variant>".
func clipName(videoID string, req ClipRequest, startMs, endMs float64) string {
	if req.Start == "" && req.End == "" {
		return videoID + "_full" + variantSuffix(req)
	}
	return fileNameWithTimecodeVariant(videoID, startMs, endMs, variantSuffix(req))
}

// outputPath is the final file for the job's request with the given (clamped) bounds.
func (s *Service) outputPath(job *Job, startMs, endMs float64) string {
	ext := ".mp4"
	if job.Request.AudioOnly {
		ext = ".mp3"
	}
	return filepath.Join(s.workbenchDir(job.VideoID), clipName(job.VideoID, job.Request, startMs, endMs)+ext)
}

// outputPathFor is the output file for the request's bounds as given (before clamping).
func (s *Service) outputPathFor(job *Job) string {
	startMs, endMs := mustParsePair(job.Request.Start, job.Request.End)
	return s.outputPath(job, startMs, endMs)
}

func (s *Service) serveFromCache(job *Job, clipPath string, marker *cacheMarker) {
	s.logger.Info("cache hit", slog.String("job", job.ID), slog.String("output", clipPath))
	title := marker.Title
	if marker.AltTitle != "" {
		title = marker.AltTitle
	}
	s.reportStage(job, StageMetadata, title)
	if marker.Translation != "" {
		s.setTranslation(job, marker.Translation)
	}
	if cbs := s.callbacksOf(job); cbs != nil {
		if cbs.OnHashtags != nil && len(marker.Tags) > 0 {
			cbs.OnHashtags(marker.Tags)
		}
		if cbs.OnTranslation != nil && marker.Translation != "" {
			cbs.OnTranslation(marker.Translation)
		}
	}
	s.reportStage(job, StageUpload, "")

	caption := ""
	if marker.Title != "" {
		caption = FormatCaption(title, marker.Tags[:min(len(marker.Tags), 5)])
	}
	s.markDone(job, clipPath, caption)
	s.notifyDone(job, clipPath, caption)
}

// markDone records a finished job's output.
func (s *Service) markDone(job *Job, output, caption string) {
	s.mu.Lock()
	job.Status = StatusDone
	job.OutputDir = output
	job.Caption = caption
	job.finishedAt = time.Now()
	s.mu.Unlock()
}

func (s *Service) reportStage(job *Job, stage, detail string) {
	s.mu.Lock()
	job.lastStage = stage
	job.lastStageDetail = detail
	cbs := job.callbacks
	s.mu.Unlock()
	if cbs != nil && cbs.OnStage != nil {
		cbs.OnStage(stage, detail)
	}
}

func (s *Service) notifyDone(job *Job, path, caption string) {
	cbs := s.callbacksOf(job)
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
	stage, detail := job.lastStage, job.lastStageDetail
	lastPct := job.lastProgPercent
	lastSpd := job.lastProgSpeed
	s.mu.Unlock()

	if stage == "" {
		stage = StageQueued
	}
	cbs.OnStage(stage, detail)
	if lastPct > 0 && (stage == StageDownload || stage == StageReencode) {
		cbs.OnProgress(ProgressUpdate{Stage: stage, Percent: lastPct, Speed: lastSpd})
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
	s.logger.Error("clip job failed", slog.String("job", job.ID), slog.String("error", msg))

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
	protectedVideoIDs := make(map[string]bool)
	now := time.Now()
	for _, j := range s.jobs {
		// Recently finished jobs may still be uploading their output.
		if j.VideoID != "" && (j.Status.Active() || j.Status == StatusDone && now.Sub(j.finishedAt) < 5*time.Minute) {
			protectedVideoIDs[j.VideoID] = true
		}
	}
	s.mu.Unlock()

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return 0, 0, nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, 0, err
	}

	for _, entry := range entries {
		name := entry.Name()
		if protectedVideoIDs[name] {
			s.logger.Debug("skipping protected active download directory", slog.String("video_id", name))
			continue
		}
		targetPath := filepath.Join(dir, name)
		var targetBytes int64
		var targetFiles int
		_ = filepath.Walk(targetPath, func(path string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() {
				targetBytes += info.Size()
				targetFiles++
			}
			return nil
		})
		if rmErr := os.RemoveAll(targetPath); rmErr == nil {
			freedBytes += targetBytes
			removedFiles += targetFiles
		}
	}

	s.mu.Lock()
	for id, j := range s.jobs {
		if (j.Status == StatusDone || j.Status == StatusError) && !protectedVideoIDs[j.VideoID] {
			delete(s.jobs, id)
		}
	}
	s.mu.Unlock()

	s.logger.Info("Downloads storage cleaned",
		slog.Int64("freed_bytes", freedBytes),
		slog.Int("removed_files", removedFiles),
	)
	return freedBytes, removedFiles, nil
}

// EncodeOptions contains video and audio encoding configuration for ffmpeg.
type EncodeOptions struct {
	CRF          string
	Preset       string
	AudioBitrate string
	MaxFileMB    int
}

// GetEncodeOptions resolves encoding options from database config with fallback to environment or defaults.
func (s *Service) GetEncodeOptions(ctx context.Context, hasSubs bool) EncodeOptions {
	crf := settings.ClipCRF
	if hasSubs {
		crf = settings.SubsCRF
	}
	return EncodeOptions{
		CRF:          s.cfg.Value(ctx, crf),
		Preset:       s.cfg.Value(ctx, settings.ClipPreset),
		AudioBitrate: s.cfg.Value(ctx, settings.ClipAudioBitrate),
		MaxFileMB:    s.cfg.Int(ctx, settings.TGFileLimitMB),
	}
}

// resolveSegmentBounds clamps the requested interval to the actual video length and enforces the duration limit.
func (s *Service) resolveSegmentBounds(ctx context.Context, meta *SourceMeta, req ClipRequest) (float64, float64, error) {
	startMs, endMs := mustParsePair(req.Start, req.End)
	if meta != nil && meta.Duration > 0 {
		videoEndMs := meta.Duration * 1000.0
		if startMs >= videoEndMs {
			return 0, 0, fmt.Errorf("start %s is beyond the end of the video (%s)", req.Start, FormatTimecode(videoEndMs))
		}
		if endMs > videoEndMs {
			endMs = videoEndMs
		}
	}
	if maxDur := s.maxSegmentDuration(ctx, req.HQ); endMs-startMs > maxDur {
		return 0, 0, fmt.Errorf(
			"interval %s-%s is too long (%.0f s), maximum is %.0f seconds",
			req.Start, req.End, (endMs-startMs)/1000, maxDur/1000)
	}
	return startMs, endMs, nil
}

// maxSegmentDuration returns the max clip length in ms: 30s for HQ, 300s otherwise.
func (s *Service) maxSegmentDuration(ctx context.Context, hq bool) float64 {
	key := settings.ClipMaxDurationSeconds
	if hq {
		key = settings.ClipMaxDurationHQSeconds
	}
	return float64(s.cfg.Int(ctx, key)) * 1000
}

func (s *Service) MaxSegmentDurationSeconds(ctx context.Context, hq bool) float64 {
	return s.maxSegmentDuration(ctx, hq) / 1000
}

func (s *Service) telegramFileLimitBytes(ctx context.Context) int64 {
	return int64(s.cfg.Int(ctx, settings.TGFileLimitMB)) << 20
}

func checkFileSize(path string, limitBytes int64) error {
	st, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("output file missing: %w", err)
	}
	if st.Size() > limitBytes {
		return fmt.Errorf(
			"file is too large: %.1f MB (Telegram bot limit is %d MB). Try a shorter interval",
			float64(st.Size())/(1024*1024), limitBytes/(1024*1024))
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

	case strings.Contains(raw, "invalid vtt content"),
		strings.Contains(raw, "Did not get any data blocks"):
		// Tracks were resolved from metadata, so this is a download failure, not a missing-subtitles case.
		return "Subtitles exist for this video, but YouTube failed to serve them. Please try again later."

	case strings.Contains(raw, "this video has no subtitles"),
		strings.Contains(raw, "no subtitles available"),
		strings.Contains(raw, "no subtitles found"),
		strings.Contains(raw, "no usable subtitle track"):
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

	case strings.Contains(raw, "Too Many Requests"),
		strings.Contains(raw, "HTTP 429"):
		return "YouTube rate limit reached. Please try again later."

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
