package downloader

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"go.uber.org/zap"
)

// ytDlpProgressRe matches "[download]   42.3% of ..." lines (--newline).
var ytDlpProgressRe = regexp.MustCompile(`^\[download\]\s+(\d+(?:\.\d+)?)%`)

// formatSelector picks the yt-dlp -f expression:
//   - audioOnly: best audio stream
//   - shorts: best compatible MP4 stream, no height cap
//   - hq: up to 2K (2560p)
//   - default: up to maxHeight (1080)
func formatSelector(maxHeight int, hq, shorts, audioOnly bool) string {
	if audioOnly {
		return "bestaudio/best"
	}
	switch {
	case shorts:
		return "bestvideo[vcodec^=avc1]+bestaudio[acodec^=mp4a]/bestvideo[ext=mp4]+bestaudio[ext=m4a]/bestvideo+bestaudio/best"
	case hq:
		return "bestvideo[height<=2560]+bestaudio/best[height<=2560]"
	default:
		if maxHeight <= 0 {
			maxHeight = 1080
		}
		return fmt.Sprintf("bestvideo[height<=%d]+bestaudio/best[height<=%d]", maxHeight, maxHeight)
	}
}

// baseYTDLPArgs returns the common flags for every yt-dlp invocation.
func (s *Service) baseYTDLPArgs(cookieFile string) []string {
	args := []string{
		"--ignore-config",
		"--no-playlist",
		"--no-playlist-reverse",
		"--js-runtimes", "node",
		"--remote-components", "ejs:github",
	}
	args = append(args, s.authArgs(cookieFile)...)
	return args
}

// downloadFullVideo downloads an entire video (shorts / tiktok) at best quality.
func (s *Service) downloadFullVideo(ctx context.Context, req ClipRequest, outPath, cookieFile string, onProgress func(int)) error {
	return s.download(ctx, req, outPath, cookieFile, false, onProgress)
}

// downloadSegment downloads a video section via yt-dlp and merges it to mp4:
//
//	yt-dlp --ignore-config [auth] -f "bestvideo[height<=1080]+bestaudio/best[height<=1080]" \
//	  --download-sections "*START-END" --force-keyframes-at-cuts \
//	  --merge-output-format mp4 -o <out> <url>
func (s *Service) downloadSegment(ctx context.Context, req ClipRequest, outPath, cookieFile string, onProgress func(int)) error {
	return s.download(ctx, req, outPath, cookieFile, true, onProgress)
}

// download runs the yt-dlp download; withSections adds --download-sections
// and --force-keyframes-at-cuts using req.Start/req.End. onProgress receives
// the 0..100 download percentage when non-nil.
func (s *Service) download(ctx context.Context, req ClipRequest, outPath, cookieFile string, withSections bool, onProgress func(int)) error {
	bin, err := YTDLPBinary()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	format := formatSelector(req.Quality, req.HQ, req.Shorts, req.AudioOnly)

	args := append(s.baseYTDLPArgs(cookieFile),
		"-f", format,
		"--newline",
	)
	if !req.AudioOnly {
		args = append(args, "--merge-output-format", "mp4")
	}
	if withSections && req.Start != "" && req.End != "" {
		section := fmt.Sprintf("*%s-%s", req.Start, req.End)
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
	var stderr strings.Builder
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start yt-dlp: %w", err)
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		if onProgress == nil {
			continue
		}
		if m := ytDlpProgressRe.FindStringSubmatch(scanner.Text()); len(m) > 1 {
			if pct, perr := strconv.ParseFloat(m[1], 64); perr == nil {
				onProgress(int(pct))
			}
		}
	}

	runErr := cmd.Wait()
	if runErr != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("yt-dlp failed: %w: %s", runErr, truncate(stderr.String(), 2000))
	}

	// Probe common container extensions if exact outPath was not written directly.
	if _, err := os.Stat(outPath); err != nil {
		for _, probeExt := range []string{".mp4", ".mkv", ".webm", ".m4a", ".opus", ".mp3"} {
			alt := baseOut + probeExt
			if _, err := os.Stat(alt); err == nil {
				return os.Rename(alt, outPath)
			}
		}
		return fmt.Errorf("segment output not found at %s: %s", outPath, truncate(stderr.String(), 500))
	}
	return nil
}

// variantSuffix builds a distinguishing suffix for a request so that
// different variants of the same interval never collide:
// "-mp3", "-hq", "-gif", "-ru", or combinations like "-hq-gif-ru".
func variantSuffix(req ClipRequest) string {
	var parts []string
	if req.AudioOnly {
		parts = append(parts, "mp3")
	}
	if req.HQ {
		parts = append(parts, "hq")
	}
	if req.GIF {
		parts = append(parts, "gif")
	}
	if req.SubsLang != "" {
		parts = append(parts, sanitizeName(req.SubsLang))
	}
	if len(parts) == 0 {
		return ""
	}
	return "-" + strings.Join(parts, "-")
}

// fileNameWithTimecode builds "<id>_<start>_<end>" clip file names.
func fileNameWithTimecode(videoID string, startMs, endMs float64) string {
	return fileNameWithTimecodeVariant(videoID, startMs, endMs, "")
}

// fileNameWithTimecodeVariant appends an optional variant suffix.
func fileNameWithTimecodeVariant(videoID string, startMs, endMs float64, variant string) string {
	start := FormatTimecode(startMs)
	end := FormatTimecode(endMs)
	return fmt.Sprintf("%s_%s-%s%s", videoID, sanitizeName(start), sanitizeName(end), variant)
}

func sanitizeName(s string) string {
	return strings.NewReplacer(":", "", ".", "-", " ", "", "/", "-", "\\", "-").Replace(s)
}
