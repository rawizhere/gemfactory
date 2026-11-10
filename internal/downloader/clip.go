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

	"log/slog"

	"gemfactory/internal/format"
	"gemfactory/internal/media"
)

var ErrIncompleteStream = errors.New("incomplete stream: video duration is shorter than expected")

var ytDlpProgressRe = regexp.MustCompile(`\[download\]\s+(\d+(?:\.\d+)?)%(?:\s+of\s+~?\s*([^\s]+))?(?:\s+at\s+([^\s]+))?(?:\s+ETA\s+([^\s]+))?`)
var ffmpegSectionTimeRe = regexp.MustCompile(`time=(\d{2}:\d{2}:\d{2}(?:\.\d+)?)`)
var ffmpegSectionSizeRe = regexp.MustCompile(`size=\s*([^\s]+)`)
var ffmpegSectionSpeedRe = regexp.MustCompile(`speed=\s*([^\s]+)`)
var ffmpegSectionBitrateRe = regexp.MustCompile(`bitrate=\s*([^\s]+)`)

// FormatHumanSize renders a yt-dlp/ffmpeg size (binary units, e.g. "25.40MiB") in human form.
func FormatHumanSize(raw string) string {
	raw = strings.TrimSpace(raw)
	lower := strings.ToLower(raw)
	num := strings.TrimRight(lower, "kmgib")
	unit := lower[len(num):]
	v, err := strconv.ParseFloat(strings.TrimSpace(num), 64)
	if err != nil || unit == "" {
		return raw
	}
	bytes := v * 1024
	switch unit[0] {
	case 'm':
		bytes *= 1024
	case 'g':
		bytes *= 1024 * 1024
	}
	return format.Bytes(int64(bytes))
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
	case shorts && hq:
		// No caps: best stream of any codec; the re-encode step fits it into the Telegram size limit.
		return "bestvideo+bestaudio/best"
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
		"--extractor-retries", "3",
		"--retries", "5",
		"--fragment-retries", "5",
	}
	args = append(args, s.authArgs(ctx, cookieFile)...)
	if potURL := os.Getenv("BGUTIL_POT_URL"); potURL != "" {
		args = append(args, "--extractor-args", "youtubepot-bgutilhttp:base_url="+potURL)
	}
	return args
}

// fallbackFetchArgs overrides player_client (web_safari+android instead of mweb) and disables concurrent fragments; appended after base args so they win.
func fallbackFetchArgs() []string {
	return []string{
		"--extractor-args", "youtube:player_client=web_safari,android",
		"-N", "1",
	}
}

// dlOpts describes one yt-dlp download; endMs > startMs cuts that section via --download-sections.
type dlOpts struct {
	req            ClipRequest
	out, cookie    string
	startMs, endMs float64
	vertical       bool
	fallback       bool // switch to the incomplete-stream retry fetch path
	onProgress     func(ProgressUpdate)
}

func (o dlOpts) sectionMs() float64 { return max(o.endMs-o.startMs, 0) }

// download runs the yt-dlp download with one retry on failure.
func (s *Service) download(ctx context.Context, o dlOpts) error {
	err := s.downloadOnce(ctx, o)
	if err == nil {
		return nil
	}
	s.logger.Warn("download failed, retrying once", slog.String("url", o.req.URL), slog.Any("error", err))
	time.Sleep(10 * time.Second)
	return s.downloadOnce(ctx, o)
}

func (s *Service) downloadOnce(ctx context.Context, o dlOpts) error {
	bin, err := YTDLPBinary()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(o.out), 0755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	expectedMs := o.sectionMs()
	format := formatSelector(o.req.Quality, o.req.HQ, o.req.Shorts, o.req.AudioOnly, o.vertical)
	args := append(s.baseYTDLPArgs(ctx, o.cookie), "-f", format, "--newline")
	if o.fallback {
		args = append(args, fallbackFetchArgs()...)
	}
	if !o.req.AudioOnly {
		args = append(args, "--merge-output-format", "mp4")
	}
	if expectedMs > 0 {
		section := fmt.Sprintf("*%s-%s", FormatTimecode(o.startMs), FormatTimecode(o.endMs))
		args = append(args, "--download-sections", section, "--force-keyframes-at-cuts")
	}
	ext := filepath.Ext(o.out)
	if ext == "" {
		ext = ".mp4"
	}
	baseOut := strings.TrimSuffix(o.out, ext)
	args = append(args, "-o", baseOut+".%(ext)s", o.req.URL)

	cmd := exec.CommandContext(ctx, bin, args...)
	s.logger.Info("yt-dlp download starting",
		slog.String("url", o.req.URL),
		slog.Bool("sections", expectedMs > 0),
		slog.String("format", format))

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
					errBuf.WriteString(text + "\n")
				}
				errMu.Unlock()
			}
			if o.onProgress == nil {
				continue
			}
			if upd, ok := parseYTDLPProgress(text); ok {
				o.onProgress(upd)
			} else if upd, ok := parseSectionProgress(text, expectedMs); ok {
				o.onProgress(upd)
			}
		}
	}
	wg.Add(2)
	go scanPipe(stdout, false)
	go scanPipe(stderrPipe, true)
	wg.Wait()

	runErr := cmd.Wait()
	errText := errBuf.String()
	if runErr != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("yt-dlp failed: %w: %s", runErr, truncate(errText, 2000))
	}

	// Probe common container extensions if exact out path was not written directly.
	if _, err := os.Stat(o.out); err != nil {
		for _, probeExt := range []string{".mp4", ".mkv", ".webm", ".m4a", ".opus", ".mp3"} {
			alt := baseOut + probeExt
			if _, err := os.Stat(alt); err == nil {
				if renErr := os.Rename(alt, o.out); renErr != nil {
					return fmt.Errorf("rename %s to %s: %w", alt, o.out, renErr)
				}
				break
			}
		}
		if _, err := os.Stat(o.out); err != nil {
			return fmt.Errorf("segment output not found at %s: %s", o.out, truncate(errText, 500))
		}
	}

	if expectedMs > 0 && !o.req.AudioOnly {
		if info, err := media.Probe(o.out); err == nil && info.VideoDuration > 0 {
			videoDur, audioDur, expectedSec := info.VideoDuration, info.AudioDuration, expectedMs/1000
			if expectedSec-videoDur > 2.0 || (expectedSec > 5.0 && audioDur > 0 && math.Abs(videoDur-audioDur) > 2.0) {
				_ = os.Remove(o.out)
				return fmt.Errorf("%w (video: %.2fs, audio: %.2fs, expected: %.2fs)", ErrIncompleteStream, videoDur, audioDur, expectedSec)
			}
		}
	}
	return nil
}

// parseYTDLPProgress parses a "[download] 42.0% of 25.40MiB at 4.12MiB/s ETA 00:03" line.
func parseYTDLPProgress(line string) (ProgressUpdate, bool) {
	m := ytDlpProgressRe.FindStringSubmatch(line)
	if m == nil {
		return ProgressUpdate{}, false
	}
	pct, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return ProgressUpdate{}, false
	}
	upd := ProgressUpdate{Stage: StageDownload, Percent: int(pct)}
	if m[2] != "" {
		upd.Size = FormatHumanSize(m[2])
	}
	if !strings.EqualFold(m[3], "unknown") {
		upd.Speed = m[3]
	}
	if !strings.EqualFold(m[4], "unknown") {
		upd.ETA = m[4]
	}
	return upd, true
}

// parseSectionProgress parses ffmpeg's "size=... time=... bitrate=... speed=..." status line of a
// --download-sections fetch; speed is shown as throughput (bitrate x speed) when both are known.
func parseSectionProgress(line string, expectedMs float64) (ProgressUpdate, bool) {
	tm := ffmpegSectionTimeRe.FindStringSubmatch(line)
	if tm == nil || expectedMs <= 0 {
		return ProgressUpdate{}, false
	}
	curMs, err := ParseTimecode(tm[1])
	if err != nil {
		return ProgressUpdate{}, false
	}
	upd := ProgressUpdate{Stage: StageDownload, Percent: min(int(curMs/expectedMs*100), 100)}
	if m := ffmpegSectionSizeRe.FindStringSubmatch(line); m != nil {
		upd.Size = FormatHumanSize(m[1])
	}
	if m := ffmpegSectionSpeedRe.FindStringSubmatch(line); m != nil {
		upd.Speed = m[1]
		mult, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(m[1], "x")), 64)
		if br := ffmpegSectionBitrateRe.FindStringSubmatch(line); br != nil && err == nil && mult > 0 {
			raw := strings.TrimSuffix(strings.TrimSuffix(strings.ToLower(br[1]), "kbits/s"), "kbps")
			if kbps, err := strconv.ParseFloat(strings.TrimSpace(raw), 64); err == nil {
				upd.Speed = format.Bytes(int64(kbps*mult/8*1024)) + "/s"
			}
		}
	}
	return upd, true
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
		parts = append(parts, "yt")
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

var (
	resolveMu   sync.Mutex
	binaryPath  string
	binaryReady bool
)

// EnsureYTDLP resolves the yt-dlp binary from PATH (installed by the image).
func EnsureYTDLP(_ context.Context) error {
	resolveMu.Lock()
	defer resolveMu.Unlock()

	if binaryReady {
		return nil
	}
	path, err := exec.LookPath("yt-dlp")
	if err != nil {
		return fmt.Errorf("yt-dlp not found in PATH: %w", err)
	}
	binaryPath = path
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
	if durationSec <= 0 || maxFileMB <= 0 {
		return 0
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
	return max((totalBits/durationSec/1000.0)-audioKbps, 500)
}

var ffmpegOutTimeRe = regexp.MustCompile(`^out_time_(?:us|ms)=(\d+)`)
var ffmpegSpeedRe = regexp.MustCompile(`^speed=\s*([^\s]+)`)

// runFFmpeg runs ffmpeg; with onProgress and expectedMs > 0 it reads -progress output and reports
// stage/detail updates against expectedMs. Errors carry the (truncated) stderr.
func runFFmpeg(ctx context.Context, args []string, expectedMs float64, stage, detail string, onProgress func(ProgressUpdate)) error {
	bin := ffmpegBinary()
	if bin == "" {
		return fmt.Errorf("ffmpeg not found in PATH")
	}
	track := onProgress != nil && expectedMs > 0
	if track {
		args = append([]string{"-progress", "pipe:1", "-nostats"}, args...)
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start ffmpeg: %w", err)
	}
	var speed string
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		if !track {
			continue
		}
		line := scanner.Text()
		if m := ffmpegSpeedRe.FindStringSubmatch(line); m != nil {
			speed = strings.TrimSpace(m[1])
		}
		if m := ffmpegOutTimeRe.FindStringSubmatch(line); m != nil {
			if us, err := strconv.ParseInt(m[1], 10, 64); err == nil {
				pct := min(int(float64(us)/(expectedMs*1000)*100), 100)
				onProgress(ProgressUpdate{Stage: stage, Percent: pct, Speed: speed, Detail: detail})
			}
		}
	}
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("ffmpeg failed: %w: %s", err, truncate(stderr.String(), 2000))
	}
	return nil
}

func (s *Service) ReencodeWithSubs(ctx context.Context, clipPath, trimmedVTT string, gif bool, expectedDurationMS float64, onProgress func(ProgressUpdate)) (string, error) {
	opts := s.GetEncodeOptions(ctx, trimmedVTT != "")
	maxSizeBytes := int64(opts.MaxFileMB) << 20
	tmpPath := strings.TrimSuffix(clipPath, filepath.Ext(clipPath)) + "_reencoded.mp4"

	// One probe serves both the fast-path codec check and the duration fallback.
	fastPath := trimmedVTT == "" && !gif
	var info *media.Info
	if fastPath || expectedDurationMS <= 0 {
		info, _ = media.Probe(clipPath)
	}

	// Fast path: no subtitles, no gif -> try ultra-fast remux if already Apple/H.264/AAC compatible and under size limit.
	if fastPath && isAppleCompatible(info) {
		err := runFFmpeg(ctx, []string{"-y", "-nostdin", "-i", clipPath, "-c", "copy", "-movflags", "+faststart", tmpPath}, 0, "", "", nil)
		if err == nil {
			if fi, serr := os.Stat(tmpPath); serr == nil && fi.Size() <= maxSizeBytes {
				if renErr := os.Rename(tmpPath, clipPath); renErr == nil {
					return clipPath, nil
				}
			}
		} else {
			s.logger.Debug("fast remux fallback to full reencode", slog.Any("error", err))
		}
		_ = os.Remove(tmpPath)
	}

	durationSec := expectedDurationMS / 1000.0
	if durationSec <= 0 && info != nil {
		durationSec = info.VideoDuration
	}
	maxVideoKbps := calculateBitrateCapKbps(durationSec, opts.AudioBitrate, opts.MaxFileMB, gif)

	baseArgs := []string{"-y", "-nostdin", "-i", clipPath}
	var vfilters []string
	if trimmedVTT != "" {
		vfilters = append(vfilters, fmt.Sprintf("subtitles=%s:force_style=%s", escapeFilterPath(trimmedVTT), subtitleStyle))
	}
	if maxVideoKbps > 0 && maxVideoKbps < 2800 {
		vfilters = append(vfilters, "scale=w=1280:h=720:force_original_aspect_ratio=decrease:force_divisible_by=2")
	} else {
		vfilters = append(vfilters, "scale=trunc(iw/2)*2:trunc(ih/2)*2")
	}
	baseArgs = append(baseArgs, "-vf", strings.Join(vfilters, ","))
	if gif {
		baseArgs = append(baseArgs, "-an")
	} else {
		baseArgs = append(baseArgs, "-c:a", "aac", "-b:a", opts.AudioBitrate)
	}
	baseArgs = append(baseArgs,
		"-c:v", "libx264",
		"-crf", opts.CRF,
		"-preset", opts.Preset,
		"-pix_fmt", "yuv420p",
	)

	// CRF alone ignores the size budget, so always cap it with the computed maxrate; one corrective retry absorbs VBV overshoot.
	capKbps := int(maxVideoKbps)
	for attempt := 0; ; attempt++ {
		args := append([]string{}, baseArgs...)
		if maxVideoKbps > 0 {
			args = append(args,
				"-maxrate", fmt.Sprintf("%dk", capKbps),
				"-bufsize", fmt.Sprintf("%dk", capKbps*2),
			)
		}
		args = append(args, "-movflags", "+faststart", tmpPath)

		s.logger.Info("ffmpeg re-encode starting",
			slog.String("input", clipPath),
			slog.Bool("burn_subs", trimmedVTT != ""),
			slog.String("crf", opts.CRF),
			slog.String("preset", opts.Preset),
			slog.String("audio_bitrate", opts.AudioBitrate),
			slog.Float64("max_video_kbps", float64(capKbps)),
			slog.Int("attempt", attempt))
		if err := runFFmpeg(ctx, args, expectedDurationMS, StageReencode, "", onProgress); err != nil {
			_ = os.Remove(tmpPath)
			return "", err
		}

		fi, serr := os.Stat(tmpPath)
		if serr != nil || fi.Size() <= maxSizeBytes || attempt >= 1 || maxVideoKbps <= 0 {
			// still over budget after the corrective retry: keep the file, run()'s size check reports the failure
			break
		}
		// rescale the cap by the measured overshoot and encode once more
		capKbps = max(int(float64(capKbps)*float64(maxSizeBytes)/float64(fi.Size())*0.97), 500)
		s.logger.Info("re-encode overshot the size budget, retrying with lower cap", slog.Int("cap_kbps", capKbps), slog.Int64("bytes", fi.Size()))
	}

	if err := os.Rename(tmpPath, clipPath); err != nil {
		return "", fmt.Errorf("replace clip with re-encoded file: %w", err)
	}
	return clipPath, nil
}

// isAppleCompatible reports whether a probed file is H.264 (yuv420p) with AAC or no audio.
func isAppleCompatible(info *media.Info) bool {
	return info != nil && info.VideoCodec == "h264" &&
		(info.PixFmt == "" || info.PixFmt == "yuv420p") &&
		(info.AudioCodec == "" || info.AudioCodec == "aac")
}

func (s *Service) ExtractAudioMP3(ctx context.Context, inputPath, outMP3Path string, expectedDurationMS float64, onProgress func(ProgressUpdate)) error {
	tmpMP3 := strings.TrimSuffix(outMP3Path, ".mp3") + "_tmp.mp3"
	args := []string{"-y", "-nostdin", "-i", inputPath, "-vn", "-c:a", "libmp3lame", "-q:a", "2", tmpMP3}
	if err := runFFmpeg(ctx, args, expectedDurationMS, StageReencode, "mp3", onProgress); err != nil {
		_ = os.Remove(tmpMP3)
		return fmt.Errorf("mp3 extract: %w", err)
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
