package downloader

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// vttTagReplacer strips YouTube auto-caption styling tags.
var vttTagReplacer = strings.NewReplacer("<c>", "", "</c>", "")

// inlineWordTimestampRe matches inline word-timing tags such as
// <00:00:00.546> found in YouTube auto-generated subtitle payloads.
var inlineWordTimestampRe = regexp.MustCompile(`<\d{2}:\d{2}:\d{2}\.\d{3}>`)

// subtitlesForClip resolves the language, downloads the full VTT (cached),
// trims it to [startMs, endMs) and returns the trimmed file path. An empty
// string means "no usable subtitles" (encode without them).
func (s *Service) subtitlesForClip(
	ctx context.Context,
	meta *SourceMeta,
	job *Job,
	requestedLang, cookieFile string,
	startMs, endMs float64,
) (string, error) {
	videoID := job.VideoID
	workbench := s.workbenchDir(videoID)

	lang, ok := ResolveSubsLanguage(meta, requestedLang)
	if !ok {
		return "", fmt.Errorf("no subtitle track found for %q", requestedLang)
	}

	fullVTT := filepath.Join(workbench, "fullsubs_"+videoID+"."+lang+".vtt")
	if _, err := os.Stat(fullVTT); os.IsNotExist(err) {
		if err := s.downloadSubtitlesWithRetry(ctx, job, lang, cookieFile, fullVTT); err != nil {
			return "", err
		}
	}
	if _, err := os.Stat(fullVTT); err != nil {
		return "", fmt.Errorf("no subtitles found for language %q", lang)
	}

	trimmedPath := filepath.Join(workbench, fileNameWithTimecode(videoID, startMs, endMs)+"_trimmed.vtt")
	cues, err := TrimVTTFile(fullVTT, startMs, endMs, trimmedPath)
	if err != nil {
		return "", err
	}
	if cues == 0 {
		return "", nil
	}
	return trimmedPath, nil
}

// subtitleDownloadAttempts is how many times a failing yt-dlp subtitle
// download (e.g. YouTube 429 rate limiting) is retried before giving up.
const subtitleDownloadAttempts = 3

func (s *Service) downloadSubtitlesWithRetry(ctx context.Context, job *Job, lang, cookieFile, fullVTT string) error {
	bin, err := YTDLPBinary()
	if err != nil {
		return err
	}
	args := append(s.baseYTDLPArgs(cookieFile),
		"--write-sub",
		"--write-auto-subs",
		"--sub-lang", lang,
		"--sub-format", "vtt",
		"--skip-download",
		"-o", filepath.Join(filepath.Dir(fullVTT), "fullsubs_"+job.VideoID),
		job.Request.URL,
	)

	var lastErr error
	for attempt := 1; attempt <= subtitleDownloadAttempts; attempt++ {
		cmd := exec.CommandContext(ctx, bin, args...)
		out, runErr := cmd.CombinedOutput()
		if runErr == nil {
			return nil
		}
		lastErr = fmt.Errorf("yt-dlp subtitle download failed: %w: %s", runErr, truncate(string(out), 2000))
		slog.Warn("subtitle download attempt failed",
			"video_id", job.VideoID, "lang", lang,
			"attempt", attempt, "of", subtitleDownloadAttempts,
			"error", truncate(string(out), 500))

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(attempt) * 15 * time.Second):
		}
	}
	return lastErr
}

// vttHeaderLines is the canonical WEBVTT header written to trimmed files.
var vttHeaderLines = []string{"WEBVTT", "Kind: captions", "Language: en", ""}

// TrimVTTFile keeps cues overlapping [startMs, endMs), shifts their
// timestamps relative to startMs and writes the result to outPath.
// Returns the number of cues kept.
func TrimVTTFile(inPath string, startMs, endMs float64, outPath string) (int, error) {
	data, err := os.ReadFile(inPath)
	if err != nil {
		return 0, fmt.Errorf("read subtitles: %w", err)
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")

	type cue struct{ lines []string }
	var cues []cue
	var current cue
	inCue := false

trimLoop:
	for _, line := range lines {
		switch {
		case strings.Contains(line, "-->"):
			if inCue && len(current.lines) > 1 {
				cues = append(cues, current)
				current = cue{}
			}
			parts := strings.Split(line, " --> ")
			if len(parts) != 2 {
				continue
			}
			startStr := strings.Fields(parts[0])[0]
			endStr := strings.Fields(parts[1])[0]
			cs, err := parseVTTTimestamp(startStr)
			if err != nil {
				inCue = false
				current = cue{}
				continue
			}
			ce, err := parseVTTTimestamp(endStr)
			if err != nil {
				inCue = false
				current = cue{}
				continue
			}
			if cs >= endMs {
				// Everything past the clip end can be skipped entirely.
				break trimLoop
			}
			if ce <= startMs {
				// Cue fully before the clip start: drop it and its text lines.
				inCue = false
				current = cue{}
				continue
			}
			newStart := cs - startMs
			if newStart < 0 {
				newStart = 0
			}
			newEnd := ce - startMs
			ts := FormatTimecode(newStart) + " --> " + FormatTimecode(newEnd)
			current = cue{lines: []string{ts}}
			inCue = true
		case inCue:
			if line == "" {
				if len(current.lines) > 1 {
					cues = append(cues, current)
				}
				current = cue{}
				inCue = false
				continue
			}
			line = inlineWordTimestampRe.ReplaceAllString(vttTagReplacer.Replace(line), "")
			if strings.TrimSpace(line) == "" {
				// Drop whitespace-only filler lines (YouTube auto-subs).
				continue
			}
			// Drop VTT block identifiers (numeric cue ids) that leaked through.
			if len(current.lines) == 1 && isNumericLine(line) {
				continue
			}
			current.lines = append(current.lines, line)
		}
	}
	if inCue && len(current.lines) > 1 {
		cues = append(cues, current)
	}

	var b strings.Builder
	for _, l := range vttHeaderLines {
		b.WriteString(l)
		b.WriteString("\n")
	}
	for _, c := range cues {
		for _, l := range c.lines {
			b.WriteString(l)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	if err := os.WriteFile(outPath, []byte(b.String()), 0644); err != nil {
		return 0, fmt.Errorf("write trimmed subtitles: %w", err)
	}
	return len(cues), nil
}

func isNumericLine(s string) bool {
	_, err := strconv.Atoi(strings.TrimSpace(s))
	return err == nil
}

// parseVTTTimestamp parses "HH:MM:SS.mmm" or "MM:SS.mmm" into milliseconds.
func parseVTTTimestamp(ts string) (float64, error) {
	parts := strings.Split(ts, ":")
	if len(parts) == 2 {
		parts = append([]string{"00"}, parts...)
	}
	if len(parts) != 3 {
		return 0, fmt.Errorf("invalid VTT timestamp %q", ts)
	}
	h, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return 0, err
	}
	m, err := strconv.ParseFloat(parts[1], 64)
	if err != nil {
		return 0, err
	}
	sec, err := strconv.ParseFloat(parts[2], 64)
	if err != nil {
		return 0, err
	}
	return h*3600000 + m*60000 + sec*1000, nil
}
