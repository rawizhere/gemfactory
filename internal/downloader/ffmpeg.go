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

// subtitleStyle mirrors the reference force_style: Lato, translucent
// outline box. Requires libass and the Lato font on the host.
const subtitleStyle = "Fontname=Lato\\,OutlineColour=&H40000000\\,BorderStyle=3"

// ffmpegOutTimeRe parses "-progress pipe:1" lines. Note: ffmpeg reports
// microseconds under the "out_time_ms" key as well as "out_time_us".
var ffmpegOutTimeRe = regexp.MustCompile(`^out_time_(?:us|ms)=(\d+)`)

// ReencodeWithSubs re-encodes clipPath with libx264/yuv420p and burns
// trimmedVTT into the image when provided. GIF mode drops the audio track.
// When no subtitles and no GIF are requested, it attempts a fast stream-copy
// with +faststart first, saving significant CPU and time.
// expectedDurationMS (0 = unknown) drives progress reporting via onProgress.
// The output atomically replaces clipPath only after ffmpeg succeeds.
func (s *Service) ReencodeWithSubs(ctx context.Context, clipPath, trimmedVTT string, gif bool, expectedDurationMS float64, onProgress func(int)) (string, error) {
	bin := ffmpegBinary()
	if bin == "" {
		return "", fmt.Errorf("ffmpeg not found in PATH")
	}

	tmpPath := strings.TrimSuffix(clipPath, filepath.Ext(clipPath)) + "_reencoded.mp4"

	// Fast path: no subtitles, no gif -> try ultra-fast remux if already Apple/H.264/AAC compatible.
	if trimmedVTT == "" && !gif && isAppleCompatible(clipPath) {
		fastCmd := exec.CommandContext(ctx, bin, "-y", "-nostdin", "-i", clipPath, "-c", "copy", "-movflags", "+faststart", tmpPath)
		if out, err := fastCmd.CombinedOutput(); err == nil {
			if renErr := os.Rename(tmpPath, clipPath); renErr == nil {
				return clipPath, nil
			}
		} else {
			s.logger.Debug("fast remux fallback to full reencode", zap.String("output", string(out)))
			_ = os.Remove(tmpPath)
		}
	}

	args := []string{"-y", "-nostdin", "-i", clipPath}
	if trimmedVTT != "" {
		subArg := escapeFilterPath(trimmedVTT)
		args = append(args,
			"-vf",
			fmt.Sprintf("subtitles=%s:force_style=%s,scale=trunc(iw/2)*2:trunc(ih/2)*2", subArg, subtitleStyle),
		)
	} else {
		// Ensure width/height are divisible by 2 for H.264 yuv420p hardware decoders.
		args = append(args, "-vf", "scale=trunc(iw/2)*2:trunc(ih/2)*2")
	}
	if gif {
		args = append(args, "-an")
	} else {
		args = append(args, "-c:a", "aac", "-b:a", "192k")
	}
	if onProgress != nil && expectedDurationMS > 0 {
		args = append(args, "-progress", "pipe:1", "-nostats")
	}
	args = append(args,
		"-c:v", "libx264",
		"-crf", "18",
		"-preset", "fast",
		"-pix_fmt", "yuv420p",
		// moov atom first: required for Telegram streaming uploads and
		// avoids the server-side transcode path that can distort playback.
		"-movflags", "+faststart",
		tmpPath)

	cmd := exec.CommandContext(ctx, bin, args...)
	s.logger.Info("ffmpeg re-encode starting",
		zap.String("input", clipPath),
		zap.Bool("burn_subs", trimmedVTT != ""))

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("stdout pipe: %w", err)
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("start ffmpeg: %w", err)
	}

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		if onProgress == nil || expectedDurationMS <= 0 {
			continue
		}
		if m := ffmpegOutTimeRe.FindStringSubmatch(scanner.Text()); len(m) > 1 {
			if us, perr := strconv.ParseInt(m[1], 10, 64); perr == nil {
				pct := int(float64(us) / float64(expectedDurationMS) * 100)
				if pct > 100 {
					pct = 100
				}
				onProgress(pct)
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

// isAppleCompatible checks if video is H.264/yuv420p and audio is AAC (or silent).
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

// ExtractAudioMP3 converts input media into a high-quality MP3 audio file.
func (s *Service) ExtractAudioMP3(ctx context.Context, inputPath, outMP3Path string) error {
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
		tmpMP3,
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		_ = os.Remove(tmpMP3)
		return fmt.Errorf("ffmpeg mp3 extract failed: %w: %s", err, truncate(string(out), 1000))
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

// escapeFilterPath normalizes a subtitle path for ffmpeg filter syntax:
// backslashes become slashes and Windows drive colons are escaped.
func escapeFilterPath(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	p = strings.ReplaceAll(p, ":", "\\:")
	return p
}
