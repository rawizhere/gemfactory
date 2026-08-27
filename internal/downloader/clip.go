package downloader

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	ytdlp "github.com/lrstanley/go-ytdlp"
	"go.uber.org/zap"
)

var ErrIncompleteStream = errors.New("incomplete stream: video duration is shorter than expected")

var ytDlpProgressRe = regexp.MustCompile(`\[download\]\s+(\d+(?:\.\d+)?)%(?:\s+of\s+~?\s*([^\s]+))?(?:\s+at\s+([^\s]+))?(?:\s+ETA\s+([^\s]+))?`)
var ffmpegSectionTimeRe = regexp.MustCompile(`time=(\d{2}:\d{2}:\d{2}(?:\.\d+)?)`)
var ffmpegSectionSizeRe = regexp.MustCompile(`size=\s*([^\s]+)`)
var ffmpegSectionSpeedRe = regexp.MustCompile(`speed=\s*([^\s]+)`)
var ffmpegSectionBitrateRe = regexp.MustCompile(`bitrate=\s*([^\s]+)`)

func FormatHumanSize(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	rawLower := strings.ToLower(raw)
	if strings.HasSuffix(rawLower, "kib") || strings.HasSuffix(rawLower, "kb") {
		valStr := strings.TrimSuffix(strings.TrimSuffix(rawLower, "kib"), "kb")
		if v, err := strconv.ParseFloat(strings.TrimSpace(valStr), 64); err == nil {
			if v >= 1024 {
				return fmt.Sprintf("%.1f MB", v/1024)
			}
			return fmt.Sprintf("%.0f KB", v)
		}
	}
	if strings.HasSuffix(rawLower, "mib") || strings.HasSuffix(rawLower, "mb") {
		valStr := strings.TrimSuffix(strings.TrimSuffix(rawLower, "mib"), "mb")
		if v, err := strconv.ParseFloat(strings.TrimSpace(valStr), 64); err == nil {
			if v >= 1024 {
				return fmt.Sprintf("%.1f GB", v/1024)
			}
			return fmt.Sprintf("%.1f MB", v)
		}
	}
	if strings.HasSuffix(rawLower, "gib") || strings.HasSuffix(rawLower, "gb") {
		valStr := strings.TrimSuffix(strings.TrimSuffix(rawLower, "gib"), "gb")
		if v, err := strconv.ParseFloat(strings.TrimSpace(valStr), 64); err == nil {
			return fmt.Sprintf("%.1f GB", v)
		}
	}
	return raw
}

// formatSelector picks the yt-dlp -f expression for the given quality tier.
// Vertical videos are capped by width so their full height fits the tier.
func formatSelector(quality string, hq, shorts, audioOnly, vertical bool) string {
	if audioOnly {
		return "bestaudio/best"
	}
	dim := "height"
	if vertical {
		dim = "width"
	}
	switch {
	case shorts:
		return "bestvideo[vcodec^=avc1]+bestaudio[acodec^=mp4a]/bestvideo[ext=mp4]+bestaudio[ext=m4a]/bestvideo+bestaudio/best"
	case hq:
		return fmt.Sprintf("bestvideo[%s<=1440]+bestaudio/best[%s<=1440]", dim, dim)
	case quality == "720p" || quality == "720":
		return fmt.Sprintf("bestvideo[%s<=720]+bestaudio/best[%s<=720]", dim, dim)
	default:
		maxHeight := 1080
		if q, err := strconv.Atoi(strings.TrimSuffix(quality, "p")); err == nil && q > 0 {
			maxHeight = q
		}
		return fmt.Sprintf("bestvideo[%s<=%d]+bestaudio/best[%s<=%d]", dim, maxHeight, dim, maxHeight)
	}
}

// baseYTDLPArgs returns the common flags for every yt-dlp invocation.
func (s *Service) baseYTDLPArgs(ctx context.Context, cookieFile string) []string {
	args := []string{
		"--ignore-config",
		"--no-playlist",
		"--no-playlist-reverse",
		"--js-runtimes", "node",
		"--remote-components", "ejs:github",
		"-N", "4",
		"--add-header", "Accept-Language:en-US,en;q=0.9",
		"--extractor-args", "youtube:lang=en",
		// mweb keeps DASH alive now that web is SABR'd; web_safari adds an HLS fallback.
		"--extractor-args", "youtube:player_client=default,mweb,web_safari",
		"--progress-template", "download:[download] %(progress._percent_str)s of %(progress._total_bytes_estimate_str|progress._total_bytes_str)s at %(progress._speed_str)s ETA %(progress._eta_str)s",
	}
	args = append(args, s.authArgs(ctx, cookieFile)...)
	if potURL := os.Getenv("BGUTIL_POT_URL"); potURL != "" {
		args = append(args, "--extractor-args", "youtubepot-bgutilhttp:base_url="+potURL)
	}
	return args
}

// downloadFullVideo downloads an entire video (shorts / tiktok) at best quality.
func (s *Service) downloadFullVideo(ctx context.Context, req ClipRequest, outPath, cookieFile string, vertical bool, onProgress func(ProgressUpdate)) error {
	return s.download(ctx, req, outPath, cookieFile, false, 0, 0, vertical, onProgress)
}

// downloadSegment downloads a video section via yt-dlp --download-sections and merges it to mp4.
func (s *Service) downloadSegment(ctx context.Context, req ClipRequest, outPath, cookieFile string, startMs, endMs float64, vertical bool, onProgress func(ProgressUpdate)) error {
	return s.download(ctx, req, outPath, cookieFile, true, startMs, endMs, vertical, onProgress)
}

// download runs the yt-dlp download; withSections cuts [startMs, endMs) via --download-sections.
func (s *Service) download(ctx context.Context, req ClipRequest, outPath, cookieFile string, withSections bool, startMs, endMs float64, vertical bool, onProgress func(ProgressUpdate)) error {
	bin, err := YTDLPBinary()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	var expectedDurationMS float64
	if withSections && endMs > startMs {
		expectedDurationMS = endMs - startMs
	}

	format := formatSelector(req.Quality, req.HQ, req.Shorts, req.AudioOnly, vertical)

	args := append(s.baseYTDLPArgs(ctx, cookieFile),
		"-f", format,
		"--newline",
	)
	if !req.AudioOnly {
		args = append(args, "--merge-output-format", "mp4")
	}
	if withSections && endMs > startMs {
		section := fmt.Sprintf("*%s-%s", FormatTimecode(startMs), FormatTimecode(endMs))
		args = append(args, "--download-sections", section, "--force-keyframes-at-cuts")
	}

	ext := filepath.Ext(outPath)
	if ext == "" {
		ext = ".mp4"
	}
	baseOut := strings.TrimSuffix(outPath, ext)
	args = append(args, "-o", baseOut+".%(ext)s", req.URL)

	cmd := exec.CommandContext(ctx, bin, args...)
	s.logger.Info("yt-dlp download starting",
		zap.String("url", req.URL),
		zap.Bool("sections", withSections),
		zap.String("format", format))

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start yt-dlp: %w", err)
	}

	var errBuf strings.Builder
	var errMu sync.Mutex

	var wg sync.WaitGroup
	scanPipe := func(r io.Reader, isErr bool) {
		defer wg.Done()
		scanner := bufio.NewScanner(r)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)
		scanner.Split(scanLinesOrCR)
		for scanner.Scan() {
			text := strings.TrimSpace(scanner.Text())
			if text == "" {
				continue
			}
			if isErr {
				errMu.Lock()
				if errBuf.Len() < 4000 {
					errBuf.WriteString(text)
					errBuf.WriteString("\n")
				}
				errMu.Unlock()
			}
			if onProgress != nil {
				if m := ytDlpProgressRe.FindStringSubmatch(text); len(m) > 1 {
					if pct, perr := strconv.ParseFloat(m[1], 64); perr == nil {
						upd := ProgressUpdate{
							Stage:   StageDownload,
							Percent: int(pct),
						}
						if len(m) > 2 && m[2] != "" {
							upd.Size = FormatHumanSize(m[2])
						}
						if len(m) > 3 && m[3] != "" && !strings.EqualFold(m[3], "unknown") {
							upd.Speed = m[3]
						}
						if len(m) > 4 && m[4] != "" && !strings.EqualFold(m[4], "unknown") {
							upd.ETA = m[4]
						}
						onProgress(upd)
					}
				} else if tm := ffmpegSectionTimeRe.FindStringSubmatch(text); len(tm) > 1 && expectedDurationMS > 0 {
					if curMS, perr := parseFFmpegTimeString(tm[1]); perr == nil {
						pct := int(curMS / expectedDurationMS * 100.0)
						if pct > 100 {
							pct = 100
						}
						upd := ProgressUpdate{
							Stage:   StageDownload,
							Percent: pct,
						}
						if sm := ffmpegSectionSizeRe.FindStringSubmatch(text); len(sm) > 1 {
							upd.Size = FormatHumanSize(sm[1])
						}
						if spm := ffmpegSectionSpeedRe.FindStringSubmatch(text); len(spm) > 1 {
							speedMultStr := strings.TrimSuffix(spm[1], "x")
							if brm := ffmpegSectionBitrateRe.FindStringSubmatch(text); len(brm) > 1 {
								brStr := strings.TrimSuffix(strings.ToLower(brm[1]), "kbits/s")
								brStr = strings.TrimSuffix(brStr, "kbps")
								if brVal, err := strconv.ParseFloat(strings.TrimSpace(brStr), 64); err == nil {
									if speedVal, err2 := strconv.ParseFloat(strings.TrimSpace(speedMultStr), 64); err2 == nil && speedVal > 0 {
										throughputKBps := (brVal * speedVal) / 8.0
										if throughputKBps >= 1024 {
											upd.Speed = fmt.Sprintf("%.1f MB/s", throughputKBps/1024.0)
										} else {
											upd.Speed = fmt.Sprintf("%.0f KB/s", throughputKBps)
										}
									}
								}
							}
							if upd.Speed == "" {
								upd.Speed = spm[1]
							}
						}
						onProgress(upd)
					}
				}
			}
		}
	}

	wg.Add(2)
	go scanPipe(stdout, false)
	go scanPipe(stderrPipe, true)
	wg.Wait()

	runErr := cmd.Wait()
	errMu.Lock()
	errText := errBuf.String()
	errMu.Unlock()

	if runErr != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("yt-dlp failed: %w: %s", runErr, truncate(errText, 2000))
	}

	// Probe common container extensions if exact outPath was not written directly.
	if _, err := os.Stat(outPath); err != nil {
		for _, probeExt := range []string{".mp4", ".mkv", ".webm", ".m4a", ".opus", ".mp3"} {
			alt := baseOut + probeExt
			if _, err := os.Stat(alt); err == nil {
				if renErr := os.Rename(alt, outPath); renErr != nil {
					return fmt.Errorf("rename %s to %s: %w", alt, outPath, renErr)
				}
				break
			}
		}
		if _, err := os.Stat(outPath); err != nil {
			return fmt.Errorf("segment output not found at %s: %s", outPath, truncate(errText, 500))
		}
	}

	if withSections && expectedDurationMS > 0 && !req.AudioOnly {
		videoDur, audioDur, err := probeStreamDurations(outPath)
		if err == nil && videoDur > 0 {
			expectedDurSec := expectedDurationMS / 1000.0
			if expectedDurSec-videoDur > 2.0 || (expectedDurSec > 5.0 && audioDur > 0 && math.Abs(videoDur-audioDur) > 2.0) {
				_ = os.Remove(outPath)
				return fmt.Errorf("%w (video: %.2fs, audio: %.2fs, expected: %.2fs)", ErrIncompleteStream, videoDur, audioDur, expectedDurSec)
			}
		}
	}
	return nil
}

// variantSuffix builds "-mp3", "-hq", "-720p", "-gif", "-ru" etc. so variants of one interval never collide.
func variantSuffix(req ClipRequest) string {
	var parts []string
	if req.AudioOnly {
		parts = append(parts, "mp3")
	}
	if req.HQ {
		parts = append(parts, "hq")
	} else if req.Quality != "" && req.Quality != "1080p" && req.Quality != "1080" {
		parts = append(parts, sanitizeName(req.Quality))
	}
	if req.GIF {
		parts = append(parts, "gif")
	}
	if req.SubsLang != "" {
		parts = append(parts, sanitizeName(req.SubsLang))
	}
	if req.SubsNoLLM {
		parts = append(parts, "gt")
	}
	if len(parts) == 0 {
		return ""
	}
	return "-" + strings.Join(parts, "-")
}

// fileNameWithTimecodeVariant builds "<id>_<start>_<end>" clip file names with an optional variant suffix.
func fileNameWithTimecodeVariant(videoID string, startMs, endMs float64, variant string) string {
	start := FormatTimecode(startMs)
	end := FormatTimecode(endMs)
	return fmt.Sprintf("%s_%s-%s%s", videoID, sanitizeName(start), sanitizeName(end), variant)
}

func sanitizeName(s string) string {
	return strings.NewReplacer(":", "", ".", "-", " ", "", "/", "-", "\\", "-").Replace(s)
}

func scanLinesOrCR(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	for i, b := range data {
		if b == '\n' || b == '\r' {
			return i + 1, data[:i], nil
		}
	}
	if atEOF {
		return len(data), data, nil
	}
	return 0, nil, nil
}

func parseFFmpegTimeString(ts string) (float64, error) {
	parts := strings.Split(ts, ":")
	switch len(parts) {
	case 1:
		s, err := strconv.ParseFloat(parts[0], 64)
		if err != nil {
			return 0, err
		}
		return s * 1000, nil
	case 2:
		m, err := strconv.ParseFloat(parts[0], 64)
		if err != nil {
			return 0, err
		}
		s, err := strconv.ParseFloat(parts[1], 64)
		if err != nil {
			return 0, err
		}
		return m*60000 + s*1000, nil
	case 3:
		h, err := strconv.ParseFloat(parts[0], 64)
		if err != nil {
			return 0, err
		}
		m, err := strconv.ParseFloat(parts[1], 64)
		if err != nil {
			return 0, err
		}
		s, err := strconv.ParseFloat(parts[2], 64)
		if err != nil {
			return 0, err
		}
		return h*3600000 + m*60000 + s*1000, nil
	default:
		return 0, fmt.Errorf("invalid time %q", ts)
	}
}

var (
	resolveMu   sync.Mutex
	binaryPath  string
	binaryReady bool
)

func EnsureYTDLP(ctx context.Context) error {
	resolveMu.Lock()
	defer resolveMu.Unlock()

	if binaryReady {
		return nil
	}

	if path, err := exec.LookPath("yt-dlp"); err == nil {
		binaryPath = path
		binaryReady = true
		return nil
	}

	resolved, err := ytdlp.Install(ctx, &ytdlp.InstallOptions{AllowVersionMismatch: true})
	if err != nil {
		return fmt.Errorf("yt-dlp auto-install failed: %w", err)
	}
	binaryPath = resolved.Executable
	binaryReady = true
	return nil
}

func YTDLPBinary() (string, error) {
	resolveMu.Lock()
	defer resolveMu.Unlock()
	if !binaryReady || binaryPath == "" {
		return "", fmt.Errorf("yt-dlp is not available; install yt-dlp or restart the server")
	}
	return binaryPath, nil
}

func StartYTDLPUpdateLoop(ctx context.Context) {
	go func() {
		updateToNightly(ctx)
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				updateToNightly(ctx)
			}
		}
	}()
}

func updateToNightly(parent context.Context) {
	path, err := YTDLPBinary()
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Minute)
	defer cancel()
	_, _ = exec.CommandContext(ctx, path, "--update-to", "nightly").CombinedOutput()
}

// subtitleStyle burns subs in Lato with a translucent outline box; needs libass + Lato on the host.
const subtitleStyle = "Fontname=Lato\\,OutlineColour=&H40000000\\,BorderStyle=3"

func calculateBitrateCapKbps(durationSec float64, audioBitrateStr string, maxFileMB int, gif bool) float64 {
	if durationSec <= 0 {
		return 0
	}
	if maxFileMB <= 0 {
		maxFileMB = 49
	}
	// Target slightly below limit to leave room for container/metadata overhead.
	targetMB := float64(maxFileMB) - 1.0
	if targetMB <= 0 {
		targetMB = float64(maxFileMB) * 0.95
	}
	totalBits := targetMB * 1024 * 1024 * 8
	audioKbps := 192.0
	if gif {
		audioKbps = 0.0
	} else if strings.HasSuffix(audioBitrateStr, "k") {
		if val, err := strconv.ParseFloat(strings.TrimSuffix(audioBitrateStr, "k"), 64); err == nil && val > 0 {
			audioKbps = val
		}
	}
	maxVideoKbps := (totalBits / durationSec / 1000.0) - audioKbps
	if maxVideoKbps < 500 {
		maxVideoKbps = 500
	}
	return maxVideoKbps
}

var ffmpegOutTimeRe = regexp.MustCompile(`^out_time_(?:us|ms)=(\d+)`)
var ffmpegSpeedRe = regexp.MustCompile(`^speed=\s*([^\s]+)`)

func (s *Service) ReencodeWithSubs(ctx context.Context, clipPath, trimmedVTT string, gif bool, expectedDurationMS float64, onProgress func(ProgressUpdate)) (string, error) {
	bin := ffmpegBinary()
	if bin == "" {
		return "", fmt.Errorf("ffmpeg not found in PATH")
	}

	opts := s.GetEncodeOptions(ctx, trimmedVTT != "")
	maxSizeBytes := int64(opts.MaxFileMB) * 1024 * 1024
	if maxSizeBytes <= 0 {
		maxSizeBytes = 49 * 1024 * 1024
	}

	tmpPath := strings.TrimSuffix(clipPath, filepath.Ext(clipPath)) + "_reencoded.mp4"

	// Fast path: no subtitles, no gif -> try ultra-fast remux if already Apple/H.264/AAC compatible and under size limit.
	if trimmedVTT == "" && !gif && isAppleCompatible(clipPath) {
		fastCmd := exec.CommandContext(ctx, bin, "-y", "-nostdin", "-i", clipPath, "-c", "copy", "-movflags", "+faststart", tmpPath)
		if out, err := fastCmd.CombinedOutput(); err == nil {
			if fi, serr := os.Stat(tmpPath); serr == nil && fi.Size() <= maxSizeBytes {
				if renErr := os.Rename(tmpPath, clipPath); renErr == nil {
					return clipPath, nil
				}
			}
			_ = os.Remove(tmpPath)
		} else {
			s.logger.Debug("fast remux fallback to full reencode", zap.String("output", string(out)))
			_ = os.Remove(tmpPath)
		}
	}

	durationSec := expectedDurationMS / 1000.0
	if durationSec <= 0 {
		if vDur, _, err := probeStreamDurations(clipPath); err == nil && vDur > 0 {
			durationSec = vDur
		}
	}
	maxVideoKbps := calculateBitrateCapKbps(durationSec, opts.AudioBitrate, opts.MaxFileMB, gif)

	args := []string{"-y", "-nostdin", "-i", clipPath}
	var vfilters []string
	if trimmedVTT != "" {
		subArg := escapeFilterPath(trimmedVTT)
		vfilters = append(vfilters, fmt.Sprintf("subtitles=%s:force_style=%s", subArg, subtitleStyle))
	}
	if maxVideoKbps > 0 && maxVideoKbps < 2800 {
		vfilters = append(vfilters, "scale=trunc(min(iw\\,1280)/2)*2:trunc(min(ih\\,720)/2)*2")
	} else {
		vfilters = append(vfilters, "scale=trunc(iw/2)*2:trunc(ih/2)*2")
	}
	args = append(args, "-vf", strings.Join(vfilters, ","))

	if gif {
		args = append(args, "-an")
	} else {
		args = append(args, "-c:a", "aac", "-b:a", opts.AudioBitrate)
	}
	if onProgress != nil && expectedDurationMS > 0 {
		args = append(args, "-progress", "pipe:1", "-nostats")
	}
	args = append(args,
		"-c:v", "libx264",
		"-crf", opts.CRF,
		"-preset", opts.Preset,
		"-pix_fmt", "yuv420p",
	)
	if maxVideoKbps > 0 && maxVideoKbps < 8000 {
		args = append(args,
			"-maxrate", fmt.Sprintf("%dk", int(maxVideoKbps)),
			"-bufsize", fmt.Sprintf("%dk", int(maxVideoKbps*2)),
		)
	}
	args = append(args,
		"-movflags", "+faststart",
		tmpPath)

	cmd := exec.CommandContext(ctx, bin, args...)
	s.logger.Info("ffmpeg re-encode starting",
		zap.String("input", clipPath),
		zap.Bool("burn_subs", trimmedVTT != ""),
		zap.String("crf", opts.CRF),
		zap.String("preset", opts.Preset),
		zap.String("audio_bitrate", opts.AudioBitrate),
		zap.Float64("max_video_kbps", maxVideoKbps))

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("stdout pipe: %w", err)
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("start ffmpeg: %w", err)
	}

	var lastSpeed string
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		if onProgress == nil || expectedDurationMS <= 0 {
			continue
		}
		line := scanner.Text()
		if sm := ffmpegSpeedRe.FindStringSubmatch(line); len(sm) > 1 {
			lastSpeed = strings.TrimSpace(sm[1])
		}
		if m := ffmpegOutTimeRe.FindStringSubmatch(line); len(m) > 1 {
			if us, perr := strconv.ParseInt(m[1], 10, 64); perr == nil {
				expectedUS := expectedDurationMS * 1000.0
				pct := int(float64(us) / expectedUS * 100.0)
				if pct > 100 {
					pct = 100
				}
				onProgress(ProgressUpdate{
					Stage:   StageReencode,
					Percent: pct,
					Speed:   lastSpeed,
				})
			}
		}
	}

	runErr := cmd.Wait()
	if runErr != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("ffmpeg failed: %w: %s", runErr, truncate(stderr.String(), 2000))
	}

	if err := os.Rename(tmpPath, clipPath); err != nil {
		return "", fmt.Errorf("replace clip with re-encoded file: %w", err)
	}
	return clipPath, nil
}

func isAppleCompatible(filePath string) bool {
	vCodec, pixFmt, aCodec := probeMediaCodecs(filePath)
	if vCodec != "h264" || (pixFmt != "" && pixFmt != "yuv420p") {
		return false
	}
	if aCodec != "" && aCodec != "aac" {
		return false
	}
	return true
}

func probeMediaCodecs(filePath string) (vCodec, pixFmt, aCodec string) {
	out, err := exec.Command("ffprobe", "-v", "error", "-show_entries", "stream=codec_type,codec_name,pix_fmt", "-of", "csv=p=0", filePath).Output()
	if err != nil {
		return "", "", ""
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines {
		parts := strings.Split(strings.TrimSpace(line), ",")
		if len(parts) >= 2 {
			switch parts[0] {
			case "video":
				if vCodec == "" {
					vCodec = strings.ToLower(parts[1])
					if len(parts) >= 3 {
						pixFmt = strings.ToLower(parts[2])
					}
				}
			case "audio":
				if aCodec == "" {
					aCodec = strings.ToLower(parts[1])
				}
			}
		}
	}
	return vCodec, pixFmt, aCodec
}

func probeStreamDurations(filePath string) (videoDur, audioDur float64, err error) {
	out, err := exec.Command("ffprobe", "-v", "error", "-show_entries", "stream=codec_type,duration:format=duration", "-of", "csv=p=0", filePath).Output()
	if err != nil {
		return 0, 0, err
	}
	var fmtDur float64
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines {
		parts := strings.Split(strings.TrimSpace(line), ",")
		if len(parts) == 1 && parts[0] != "" {
			if d, perr := strconv.ParseFloat(parts[0], 64); perr == nil && d > 0 {
				fmtDur = d
			}
		} else if len(parts) >= 2 {
			d, perr := strconv.ParseFloat(parts[1], 64)
			if perr != nil || d <= 0 {
				continue
			}
			switch parts[0] {
			case "video":
				if videoDur == 0 {
					videoDur = d
				}
			case "audio":
				if audioDur == 0 {
					audioDur = d
				}
			}
		}
	}
	if videoDur == 0 && fmtDur > 0 {
		videoDur = fmtDur
	}
	if audioDur == 0 && fmtDur > 0 {
		audioDur = fmtDur
	}
	return videoDur, audioDur, nil
}

func (s *Service) ExtractAudioMP3(ctx context.Context, inputPath, outMP3Path string, expectedDurationMS float64, onProgress func(ProgressUpdate)) error {
	bin := ffmpegBinary()
	if bin == "" {
		return fmt.Errorf("ffmpeg not found in PATH")
	}
	tmpMP3 := strings.TrimSuffix(outMP3Path, ".mp3") + "_tmp.mp3"
	args := []string{
		"-y", "-nostdin",
		"-i", inputPath,
		"-vn",
		"-c:a", "libmp3lame",
		"-q:a", "2",
	}
	if onProgress != nil && expectedDurationMS > 0 {
		args = append(args, "-progress", "pipe:1", "-nostats")
	}
	args = append(args, tmpMP3)

	cmd := exec.CommandContext(ctx, bin, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start ffmpeg: %w", err)
	}

	var lastSpeed string
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		if onProgress == nil || expectedDurationMS <= 0 {
			continue
		}
		line := scanner.Text()
		if sm := ffmpegSpeedRe.FindStringSubmatch(line); len(sm) > 1 {
			lastSpeed = strings.TrimSpace(sm[1])
		}
		if m := ffmpegOutTimeRe.FindStringSubmatch(line); len(m) > 1 {
			if us, perr := strconv.ParseInt(m[1], 10, 64); perr == nil {
				expectedUS := expectedDurationMS * 1000.0
				pct := int(float64(us) / expectedUS * 100.0)
				if pct > 100 {
					pct = 100
				}
				onProgress(ProgressUpdate{
					Stage:   StageReencode,
					Percent: pct,
					Speed:   lastSpeed,
					Detail:  "mp3",
				})
			}
		}
	}

	if err := cmd.Wait(); err != nil {
		_ = os.Remove(tmpMP3)
		return fmt.Errorf("ffmpeg mp3 extract failed: %w: %s", err, truncate(stderr.String(), 1000))
	}
	return os.Rename(tmpMP3, outMP3Path)
}

func ffmpegBinary() string {
	if bin := os.Getenv("FFMPEG_BINARY"); bin != "" {
		return bin
	}
	if path, err := exec.LookPath("ffmpeg"); err == nil {
		return path
	}
	return ""
}

// escapeFilterPath normalizes a subtitle path for ffmpeg filter syntax: backslashes become slashes and Windows drive colons are escaped.
func escapeFilterPath(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	p = strings.ReplaceAll(p, ":", "\\:")
	return p
}
