package downloader

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/avast/retry-go/v4"
	"github.com/MercuryEngineering/CookieMonster"
	"github.com/sashabaranov/go-openai"
	"go.uber.org/zap"
	"google.golang.org/genai"
)

// vttTagReplacer strips YouTube auto-caption styling tags.
var vttTagReplacer = strings.NewReplacer("<c>", "", "</c>", "")

// inlineWordTimestampRe matches inline word-timing tags such as <00:00:00.546> found in YouTube auto-generated subtitle payloads.
var inlineWordTimestampRe = regexp.MustCompile(`<\d{2}:\d{2}:\d{2}\.\d{3}>`)

// thinkTagRe strips reasoning/thinking tags emitted by models like Qwen.
var thinkTagRe = regexp.MustCompile(`(?s)<think>.*?</think>`)

func (s *Service) subtitlesForClip(
	ctx context.Context,
	meta *SourceMeta,
	job *Job,
	requestedLang, cookieFile string,
	startMs, endMs float64,
) (string, error) {
	videoID := job.VideoID
	s.reportStage(job, StageSubtitles, requestedLang)
	workbench := s.workbenchDir(videoID)

	cfg := s.ResolveTranslationConfig(ctx)
	res, err := ResolveSubtitleTrack(meta, requestedLang, cfg.SourcePrefRU)
	if err != nil {
		return "", err
	}

	fullVTT := filepath.Join(workbench, "fullsubs_"+videoID+"."+res.SourceLang+".vtt")
	if _, err := os.Stat(fullVTT); os.IsNotExist(err) {
		dlErr := downloadSubtitlesDirect(ctx, res.TrackURL, cookieFile, fullVTT)
		if dlErr != nil {
			slog.Warn("direct subtitle download failed, falling back to yt-dlp",
				"video_id", videoID, "error", dlErr)
			if fbErr := s.downloadSubtitlesWithRetry(ctx, job, res.SourceLang, cookieFile, fullVTT); fbErr != nil {
				return "", fmt.Errorf("failed to download subtitles: %w", dlErr)
			}
		}
	}
	if _, err := os.Stat(fullVTT); err != nil {
		return "", fmt.Errorf("no subtitles found for language %q", res.FinalLang)
	}

	trimmedPath := filepath.Join(workbench,
		fileNameWithTimecodeVariant(videoID, startMs, endMs, variantSuffix(job.Request))+"_trimmed.vtt")
	cues, err := TrimVTTFile(fullVTT, startMs, endMs, trimmedPath)
	if err != nil {
		return "", fmt.Errorf("failed to process subtitles: %w", err)
	}
	if cues == 0 {
		return "", nil
	}

	if res.TargetLang != "" {
		var videoTitle string
		if meta != nil {
			videoTitle = meta.Title
		}
		s.reportStage(job, StageTranslate, res.TargetLang)
		prov, terr := s.translateVTTFile(ctx, trimmedPath, res.TargetLang, res.SourceLang, videoTitle, job.Request.SubsNoLLM)
		if terr == nil && prov != "" {
			job.Translation = prov
			if job.callbacks != nil && job.callbacks.OnTranslation != nil {
				job.callbacks.OnTranslation(prov)
			}
		}
		if terr != nil {
			slog.Warn("vtt translation failed", "target_lang", res.TargetLang, "error", terr)
			return "", fmt.Errorf("translation into %s failed (%w)", res.TargetLang, terr)
		}
	}

	return trimmedPath, nil
}

func downloadSubtitlesDirect(ctx context.Context, rawURL, cookieFile, outPath string) error {
	if strings.Contains(rawURL, "fmt=") {
		re := regexp.MustCompile(`fmt=[^&]+`)
		rawURL = re.ReplaceAllString(rawURL, "fmt=vtt")
	} else {
		if strings.Contains(rawURL, "?") {
			rawURL += "&fmt=vtt"
		} else {
			rawURL += "?fmt=vtt"
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	if cookieFile != "" {
		for _, c := range parseNetscapeCookies(cookieFile) {
			req.AddCookie(c)
		}
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("http error: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("http %d: %s", resp.StatusCode, resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}

	if !strings.HasPrefix(strings.TrimSpace(string(body)), "WEBVTT") && !strings.Contains(string(body), "-->") {
		return fmt.Errorf("invalid vtt content: %s", truncate(string(body), 200))
	}

	if err := os.WriteFile(outPath, body, 0644); err != nil {
		return fmt.Errorf("write subtitle file: %w", err)
	}
	return nil
}

func (s *Service) translateVTTFile(ctx context.Context, vttPath, targetLang, sourceLang, videoTitle string, noLLM bool) (string, error) {
	data, err := os.ReadFile(vttPath)
	if err != nil {
		return "", err
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")

	type cue struct {
		ts   string
		text string
	}
	var cues []cue
	var currentTs string
	var currentText []string

	for _, line := range lines {
		if strings.Contains(line, "-->") {
			if currentTs != "" && len(currentText) > 0 {
				cues = append(cues, cue{ts: currentTs, text: strings.Join(currentText, "\n")})
				currentText = nil
			}
			currentTs = line
		} else if currentTs != "" {
			if line == "" {
				if len(currentText) > 0 {
					cues = append(cues, cue{ts: currentTs, text: strings.Join(currentText, "\n")})
					currentTs = ""
					currentText = nil
				}
			} else {
				currentText = append(currentText, line)
			}
		}
	}
	if currentTs != "" && len(currentText) > 0 {
		cues = append(cues, cue{ts: currentTs, text: strings.Join(currentText, "\n")})
	}

	if len(cues) == 0 {
		return "", nil
	}

	// Translate physical lines separately so multi-line cues keep their line breaks.
	var texts []string
	cueLineCounts := make([]int, len(cues))
	for i, c := range cues {
		ls := strings.Split(c.text, "\n")
		texts = append(texts, ls...)
		cueLineCounts[i] = len(ls)
	}

	cfg := s.ResolveTranslationConfig(ctx)
	if noLLM {
		cfg.GoogleOnly = true
	}
	translated, provider, err := TranslateWithChain(ctx, texts, targetLang, sourceLang, cfg, videoTitle)
	if err != nil {
		return "", err
	}
	s.logger.Info("subtitles translated", zap.String("provider", provider), zap.Int("lines", len(texts)), zap.String("target", targetLang))

	var b strings.Builder
	b.WriteString("WEBVTT\nKind: captions\nLanguage: " + targetLang + "\n\n")
	idx := 0
	for i, c := range cues {
		var outLines []string
		for j := 0; j < cueLineCounts[i] && idx < len(translated); j++ {
			t := sanitizeSubtitleTypography(strings.TrimSpace(translated[idx]))
			idx++
			if t == "" {
				t = strings.Split(c.text, "\n")[j]
			}
			outLines = append(outLines, t)
		}
		if outLines == nil {
			outLines = []string{c.text}
		}
		b.WriteString(c.ts + "\n" + splitDialogueLines(strings.Join(outLines, "\n")) + "\n\n")
	}

	return provider, os.WriteFile(vttPath, []byte(b.String()), 0644)
}

// splitDialogueLines puts each speaker of an inline "- A. - B." exchange on its own line.
func splitDialogueLines(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if strings.HasPrefix(l, "- ") {
			lines[i] = strings.ReplaceAll(l, " - ", "\n- ")
		}
	}
	return strings.Join(lines, "\n")
}

func parseNetscapeCookies(path string) []*http.Cookie {
	cookies, err := cookiemonster.ParseFile(path)
	if err != nil {
		return nil
	}
	return cookies
}

// subtitleDownloadAttempts is how many times a failing yt-dlp subtitle download is retried before giving up.
const subtitleDownloadAttempts = 3

func (s *Service) downloadSubtitlesWithRetry(ctx context.Context, job *Job, lang, cookieFile, fullVTT string) error {
	bin, err := YTDLPBinary()
	if err != nil {
		return err
	}
	args := append(s.baseYTDLPArgs(cookieFile),
		"--write-sub",
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

var vttHeaderLines = []string{"WEBVTT", "Kind: captions", "Language: en", ""}

// TrimVTTFile keeps cues overlapping [startMs, endMs), shifts their timestamps relative to startMs and writes the result to outPath. Returns the number of cues kept.
func TrimVTTFile(inPath string, startMs, endMs float64, outPath string) (int, error) {
	data, err := os.ReadFile(inPath)
	if err != nil {
		return 0, fmt.Errorf("read subtitles: %w", err)
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")

	var cues []vttCue
	var current vttCue
	inCue := false

trimLoop:
	for _, line := range lines {
		switch {
		case strings.Contains(line, "-->"):
			if inCue && len(current.lines) > 1 {
				cues = append(cues, current)
				current = vttCue{}
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
				current = vttCue{}
				continue
			}
			ce, err := parseVTTTimestamp(endStr)
			if err != nil {
				inCue = false
				current = vttCue{}
				continue
			}
			if cs >= endMs {
				// Everything past the clip end can be skipped entirely.
				break trimLoop
			}
			if ce <= startMs {
				// Cue fully before the clip start: drop it and its text lines.
				inCue = false
				current = vttCue{}
				continue
			}
			newStart := cs - startMs
			if newStart < 0 {
				newStart = 0
			}
			newEnd := ce - startMs
			ts := FormatTimecode(newStart) + " --> " + FormatTimecode(newEnd)
			current = vttCue{lines: []string{ts}}
			inCue = true
		case inCue:
			if line == "" {
				if len(current.lines) > 1 {
					cues = append(cues, current)
				}
				current = vttCue{}
				inCue = false
				continue
			}
			line = sanitizeSubtitleTypography(inlineWordTimestampRe.ReplaceAllString(vttTagReplacer.Replace(line), ""))
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

	cues = dedupeRollingCues(cues)

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

// vttCue holds a timing line plus its text lines.
type vttCue struct{ lines []string }

// dedupeRollingCues collapses the echo repeats YouTube auto-captions carry from cue to cue.
func dedupeRollingCues(cues []vttCue) []vttCue {
	var prevLower []string
	out := make([]vttCue, 0, len(cues))
	for _, c := range cues {
		raw := strings.Join(c.lines[1:], " ")
		words := strings.Fields(raw)
		lower := make([]string, len(words))
		for i, w := range words {
			lower[i] = strings.ToLower(w)
		}
		skip := longestWordOverlap(prevLower, lower)
		if skip >= len(words) {
			continue
		}
		prevLower = lower
		out = append(out, vttCue{lines: []string{c.lines[0], strings.Join(words[skip:], " ")}})
	}
	return out
}

func longestWordOverlap(a, b []string) int {
	max := len(a)
	if len(b) < max {
		max = len(b)
	}
	for n := max; n > 0; n-- {
		match := true
		for i := 0; i < n; i++ {
			if a[len(a)-n+i] != b[i] {
				match = false
				break
			}
		}
		if match {
			return n
		}
	}
	return 0
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

// SubtitleTrack describes a single subtitle URL entry from yt-dlp metadata.
type SubtitleTrack struct {
	Ext  string `json:"ext"`
	URL  string `json:"url"`
	Name string `json:"name"`
}

// SourceMeta is the subset of yt-dlp info.json used by the downloader.
type SourceMeta struct {
	ID                string                     `json:"id"`
	Title             string                     `json:"title"`
	AltTitle          string                     `json:"alt_title"`
	Description       string                     `json:"description"`
	Tags              []string                   `json:"tags"`
	Language          string                     `json:"language"`
	Duration          float64                    `json:"duration"`
	Subtitles         map[string][]SubtitleTrack `json:"subtitles"`
	AutomaticCaptions map[string][]SubtitleTrack `json:"automatic_captions"`
}

// displayTitleParts returns the cleaned base title and the alt (English) title.
func displayTitleParts(meta *SourceMeta) (string, string) {
	if meta == nil {
		return "", ""
	}
	title := strings.TrimSpace(meta.Title)
	desc := strings.TrimSpace(meta.Description)

	// If Title is truncated with ellipsis (TikTok), take the matching first line of Description.
	cleanTitle := strings.TrimRight(title, ".… ")
	if cleanTitle != "" && desc != "" {
		firstDescLine := strings.TrimSpace(strings.SplitN(desc, "\n", 2)[0])
		if strings.HasPrefix(firstDescLine, cleanTitle) {
			title = firstDescLine
		}
	} else if title == "" && desc != "" {
		title = strings.TrimSpace(strings.SplitN(desc, "\n", 2)[0])
	}

	return title, strings.TrimSpace(meta.AltTitle)
}

// DisplayTitle returns the clean full title, preferring alt_title (English) if present.
func DisplayTitle(meta *SourceMeta) string {
	base, alt := displayTitleParts(meta)
	if alt != "" {
		return alt
	}
	return base
}

// FormatCaption builds a clean, minimalist HTML caption (Title + up to 5 hashtags).
func FormatCaption(meta *SourceMeta) string {
	if meta == nil {
		return ""
	}

	title := DisplayTitle(meta)
	if title == "" {
		return ""
	}

	caption := "<b>" + htmlEscape(title) + "</b>"

	if tags := CaptionHashtags(meta); len(tags) > 0 {
		caption += "\n\n" + strings.Join(tags, " ")
	}

	if len(caption) > 1000 {
		caption = caption[:997] + "…"
	}
	return caption
}

// CaptionHashtags returns up to five "#Tag" entries not already present in the title.
func CaptionHashtags(meta *SourceMeta) []string {
	return hashtags(meta, 5)
}

// AllHashtags returns every metadata tag as "#Tag" entries.
func AllHashtags(meta *SourceMeta) []string {
	return hashtags(meta, 0)
}

func hashtags(meta *SourceMeta, limit int) []string {
	if meta == nil || len(meta.Tags) == 0 {
		return nil
	}
	var tagList []string
	titleLower := strings.ToLower(strings.TrimSpace(meta.Title))
	for _, tag := range meta.Tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		tagClean := strings.ReplaceAll(tag, " ", "")
		if !strings.Contains(titleLower, "#"+strings.ToLower(tagClean)) {
			tagList = append(tagList, "#"+tagClean)
			if limit > 0 && len(tagList) >= limit {
				break
			}
		}
	}
	return tagList
}

func htmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

func (s *Service) ExtractMetadata(ctx context.Context, url, videoID, cookieFile string) (*SourceMeta, error) {
	metaPath := filepath.Join(s.workbenchDir(videoID), videoID+".info.json")
	if _, err := os.Stat(metaPath); err == nil {
		return readSourceMeta(metaPath)
	}

	bin, err := YTDLPBinary()
	if err != nil {
		return nil, err
	}

	dir := filepath.Dir(metaPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create workbench dir: %w", err)
	}
	base := strings.TrimSuffix(metaPath, ".info.json")

	args := append(s.baseYTDLPArgs(cookieFile),
		"--write-info-json",
		"--skip-download",
		"-o", base,
		url,
	)

	cmd := exec.CommandContext(ctx, bin, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("yt-dlp metadata extraction failed: %w: %s", err, truncate(string(out), 2000))
	}
	return readSourceMeta(metaPath)
}

func readSourceMeta(path string) (*SourceMeta, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read info.json: %w", err)
	}
	var meta SourceMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("parse info.json: %w", err)
	}
	return &meta, nil
}

// SubtitleTrackResolution contains the selected source track URL and whether it needs translation.
type SubtitleTrackResolution struct {
	TrackURL   string
	SourceLang string
	TargetLang string // translation target, "" if direct
	FinalLang  string
}

// ResolveSubtitleTrack uses creator-uploaded tracks only; direct when the target exists, otherwise LLM translation from the closest source.
func ResolveSubtitleTrack(meta *SourceMeta, requested string, sourcePref []string) (*SubtitleTrackResolution, error) {
	if meta == nil || len(meta.Subtitles) == 0 {
		return nil, fmt.Errorf("no subtitles found for this video")
	}
	target := normalizeLang(requested)

	// Manual ru tracks are YouTube machine translations, so ru output always comes from the LLM.
	if target != "ru" {
		if key := findTrack(meta.Subtitles, target); key != "" {
			return &SubtitleTrackResolution{
				TrackURL:   meta.Subtitles[key][0].URL,
				SourceLang: target,
				FinalLang:  target,
			}, nil
		}
	}

	var pref []string
	if target == "ru" {
		pref = sourcePref
	}
	src := pickSourceLang(meta, target, pref)
	key := findTrack(meta.Subtitles, src)
	if key == "" {
		return nil, fmt.Errorf("no usable subtitle track found")
	}

	return &SubtitleTrackResolution{
		TrackURL:   meta.Subtitles[key][0].URL,
		SourceLang: src,
		TargetLang: target,
		FinalLang:  target,
	}, nil
}

func normalizeLang(lang string) string {
	l := strings.TrimSpace(lang)
	if l == "" {
		return "en"
	}
	return strings.ToLower(strings.SplitN(l, "-", 2)[0])
}

// findTrack matches an exact language key or a regional variant ("en-US" satisfies "en").
func findTrack(subs map[string][]SubtitleTrack, lang string) string {
	if v, ok := subs[lang]; ok && len(v) > 0 && v[0].URL != "" {
		return lang
	}
	keys := make([]string, 0, len(subs))
	for k := range subs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	prefix := lang + "-"
	for _, k := range keys {
		v, ok := subs[k]
		if ok && strings.HasPrefix(strings.ToLower(k), prefix) && len(v) > 0 && v[0].URL != "" {
			return k
		}
	}
	return ""
}

// pickSourceLang walks pref list, then the video's own language, then domain defaults.
func pickSourceLang(meta *SourceMeta, target string, pref []string) string {
	var cands []string
	for _, l := range pref {
		cands = append(cands, normalizeLang(l))
	}
	if meta != nil && meta.Language != "" {
		cands = append(cands, strings.ToLower(strings.SplitN(meta.Language, "-", 2)[0]))
	}
	cands = append(cands, "ko", "ja", "en")

	for _, l := range cands {
		if l != target && findTrack(meta.Subtitles, l) != "" {
			return l
		}
	}
	keys := make([]string, 0, len(meta.Subtitles))
	for k := range meta.Subtitles {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if strings.Contains(k, " ") {
			continue
		}
		return normalizeLang(k)
	}
	return ""
}

// DefaultSourcePrefRU is the fallback source-language order for ru subtitles.
const DefaultSourcePrefRU = "en,ko"

var langNames = map[string]string{
	"en": "English", "ru": "Russian", "ko": "Korean", "ja": "Japanese",
	"zh": "Chinese", "es": "Spanish", "pt": "Portuguese", "id": "Indonesian",
}

func langDisplayName(code string) string {
	if n, ok := langNames[code]; ok {
		return n
	}
	return code
}

const (
	ProviderGoogle   = "google"
	ProviderGemini   = "gemini"
	ProviderGroq     = "groq"
	ProviderOpencode = "opencode"
	ProviderNvidia   = "nvidia"
)

// DefaultOpencodeModels is the free-model fallback chain on OpenCode Zen.
var DefaultOpencodeModels = []string{
	"x-preview-f-free",
	"nemotron-3-ultra-free",
	"big-pickle",
}

// DefaultNvidiaModels is the free-model chain on NVIDIA NIM.
var DefaultNvidiaModels = []string{
	"minimaxai/minimax-m3",
}

const translatorRole = "You are a professional video subtitle translator."

const DefaultTranslationPrompt = `Translate subtitles for entertainment videos: variety shows, vlogs, livestreams, song lyrics. The source language varies — detect it from the lines themselves.
1. Meaning over words: natural spoken register matching the original's energy; never wooden literalism or calques.
2. Names: personal, stage and group names, fandoms and products are names — transliterate them by sound into the target script; if translating a token yields a common word, it is a name. Acronyms (MBTI, TMI, PPL) and stylized brand/model names stay in Latin script.
3. Formatting: keep speaker tags ([ALL], Host:) and multi-speaker dash structure intact; translate bracketed notes naturally inside the brackets; preserve ♪, emojis, tildes, asterisks and Asian brackets.
4. Song lyrics: translate the thought and rhythm, not word-by-word.
5. Silently fix obvious ASR/speech-recognition errors from context.`

const ruAddendum = `For Russian: informal "ты", lively youth phrasing; transliterate Korean honorifics (онни, оппа, макнэ, хён); names transliterate by sound (Liv -> Лив, May -> Мэй), a common noun next to a name stays a common word (리브 미모 = красота Лив, not "Liv Mimo"); bracketed notes translate naturally ([Laughter] -> [Смех]).`

// BuildSystemInstruction combines the configured prompt with target-language rules.
func BuildSystemInstruction(prompt, targetLang string) string {
	s := prompt
	if targetLang == "ru" {
		if s != "" {
			s += "\n"
		}
		s += ruAddendum
	}
	return s
}

var speakerTagRe = regexp.MustCompile(`^(\[[^\]]+\]|\([^\)]+\)|[A-Za-z0-9_가-힣]+:)\s*`)

// TranslationConfig holds the provider fallback order, API keys, models, and custom prompt.
type TranslationConfig struct {
	GoogleOnly     bool // force Google Translate, skip all LLM providers
	GeminiKey      string
	GroqKey        string
	OpencodeKey    string
	NvidiaKey      string
	GeminiModels   []string
	GroqModels     []string
	OpencodeModels []string
	NvidiaModels   []string
	FallbackOrder  []string
	Prompt         string
	SourcePrefRU   []string // preferred source languages when translating into ru
}

var DefaultGeminiModels = []string{
	"gemini-3.7-flash",
	"gemini-3.5-flash-lite",
	"gemini-3.1-flash-lite",
	"gemini-2.5-flash-lite",
	"gemini-2.5-flash",
	"gemini-flash-latest",
}

var DefaultGroqModels = []string{
	"groq/compound",
	"openai/gpt-oss-120b",
}

func ParseCSV(s string) []string {
	parts := strings.Split(s, ",")
	var res []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			res = append(res, p)
		}
	}
	return res
}

func isTruthy(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// DefaultTranslationConfig resolves the translation config from env with built-in defaults.
func DefaultTranslationConfig() TranslationConfig {
	cfg := TranslationConfig{
		GeminiKey:      strings.TrimSpace(os.Getenv("GEMINI_API_KEY")),
		GroqKey:        strings.TrimSpace(os.Getenv("GROQ_API_KEY")),
		OpencodeKey:    strings.TrimSpace(os.Getenv("OPENCODE_API_KEY")),
		NvidiaKey:      strings.TrimSpace(os.Getenv("NVIDIA_API_KEY")),
		GeminiModels:   ParseCSV(os.Getenv("GEMINI_MODELS")),
		GroqModels:     ParseCSV(os.Getenv("GROQ_MODELS")),
		OpencodeModels: ParseCSV(os.Getenv("OPENCODE_MODELS")),
		NvidiaModels:   ParseCSV(os.Getenv("NVIDIA_MODELS")),
		FallbackOrder:  ParseCSV(os.Getenv("TRANSLATION_FALLBACK_ORDER")),
		Prompt:         strings.TrimSpace(os.Getenv("TRANSLATION_PROMPT")),
		SourcePrefRU:   ParseCSV(os.Getenv("SUBS_SOURCE_PREF_RU")),
	}
	cfg.GoogleOnly = isTruthy(os.Getenv("SUBS_GOOGLE_ONLY"))
	return applyTranslationDefaults(cfg)
}

func (s *Service) ResolveTranslationConfig(ctx context.Context) TranslationConfig {
	cfg := DefaultTranslationConfig()
	if s.configs == nil {
		return cfg
	}
	get := func(key string) (string, bool) {
		val, err := s.configs.Get(ctx, key)
		if err != nil || val == nil || strings.TrimSpace(val.Value) == "" {
			return "", false
		}
		return strings.TrimSpace(val.Value), true
	}
	if v, ok := get("GEMINI_API_KEY"); ok {
		cfg.GeminiKey = v
	}
	if v, ok := get("GROQ_API_KEY"); ok {
		cfg.GroqKey = v
	}
	if v, ok := get("OPENCODE_API_KEY"); ok {
		cfg.OpencodeKey = v
	}
	if v, ok := get("NVIDIA_API_KEY"); ok {
		cfg.NvidiaKey = v
	}
	if v, ok := get("GEMINI_MODELS"); ok {
		cfg.GeminiModels = ParseCSV(v)
	}
	if v, ok := get("GROQ_MODELS"); ok {
		cfg.GroqModels = ParseCSV(v)
	}
	if v, ok := get("OPENCODE_MODELS"); ok {
		cfg.OpencodeModels = ParseCSV(v)
	}
	if v, ok := get("NVIDIA_MODELS"); ok {
		cfg.NvidiaModels = ParseCSV(v)
	}
	if v, ok := get("TRANSLATION_FALLBACK_ORDER"); ok {
		cfg.FallbackOrder = ParseCSV(v)
	}
	if v, ok := get("TRANSLATION_PROMPT"); ok {
		cfg.Prompt = v
	}
	if v, ok := get("SUBS_SOURCE_PREF_RU"); ok {
		cfg.SourcePrefRU = ParseCSV(v)
	}
	if val, err := s.configs.Get(ctx, "SUBS_GOOGLE_ONLY"); err == nil && val != nil {
		cfg.GoogleOnly = isTruthy(val.Value)
	}
	return applyTranslationDefaults(cfg)
}

func applyTranslationDefaults(cfg TranslationConfig) TranslationConfig {
	if len(cfg.GeminiModels) == 0 {
		cfg.GeminiModels = DefaultGeminiModels
	}
	if len(cfg.GroqModels) == 0 {
		cfg.GroqModels = DefaultGroqModels
	}
	if len(cfg.OpencodeModels) == 0 {
		cfg.OpencodeModels = DefaultOpencodeModels
	}
	if len(cfg.NvidiaModels) == 0 {
		cfg.NvidiaModels = DefaultNvidiaModels
	}
	if len(cfg.SourcePrefRU) == 0 {
		cfg.SourcePrefRU = ParseCSV(DefaultSourcePrefRU)
	}
	if cfg.Prompt == "" {
		cfg.Prompt = DefaultTranslationPrompt
	}
	return cfg
}

func BuildFallbackChain(cfg TranslationConfig) []string {
	if cfg.GoogleOnly {
		return []string{ProviderGoogle}
	}
	if len(cfg.FallbackOrder) > 0 {
		var chain []string
		for _, p := range cfg.FallbackOrder {
			p = strings.ToLower(strings.TrimSpace(p))
			if p == ProviderGoogle || p == ProviderGemini || p == ProviderGroq || p == ProviderOpencode || p == ProviderNvidia {
				chain = append(chain, p)
			}
		}
		if len(chain) > 0 {
			return chain
		}
	}
	return []string{ProviderGemini, ProviderNvidia, ProviderGroq, ProviderOpencode}
}

func (s *Service) TranslateTextsWithFallback(ctx context.Context, texts []string, targetLang string, videoTitle ...string) ([]string, string, error) {
	if len(texts) == 0 {
		return nil, "", nil
	}
	vTitle := ""
	if len(videoTitle) > 0 {
		vTitle = videoTitle[0]
	}
	return TranslateWithChain(ctx, texts, targetLang, "", s.ResolveTranslationConfig(ctx), vTitle)
}

// TranslateWithChain walks providers in configured order; first validated output wins.
func TranslateWithChain(ctx context.Context, texts []string, targetLang, sourceLang string, cfg TranslationConfig, videoTitle string) ([]string, string, error) {
	if len(texts) == 0 {
		return nil, "", nil
	}
	chain := BuildFallbackChain(cfg)
	instr := BuildSystemInstruction(cfg.Prompt, targetLang)

	var lastErr error
	for _, provider := range chain {
		switch provider {
		case ProviderGoogle:
			translated, err := TranslateWithGoogle(ctx, texts, targetLang)
			if err == nil && TranslationLooksTarget(texts, translated, targetLang) {
				return preserveSpeakerTags(texts, translated), ProviderGoogle + "/web", nil
			}
			lastErr = fmt.Errorf("google: %w", err)

		case ProviderOpencode:
			if cfg.OpencodeKey == "" {
				continue
			}
			translated, model, err := TranslateWithOpencode(ctx, texts, targetLang, sourceLang, cfg.OpencodeKey, instr, videoTitle, cfg.OpencodeModels)
			if err == nil && TranslationLooksTarget(texts, translated, targetLang) {
				return preserveSpeakerTags(texts, translated), ProviderOpencode + "/" + model, nil
			}
			lastErr = fmt.Errorf("opencode (%s): %w", modelOrEmpty(model), errIf(err))

		case ProviderNvidia:
			if cfg.NvidiaKey == "" {
				continue
			}
			translated, model, err := TranslateWithNvidia(ctx, texts, targetLang, sourceLang, cfg.NvidiaKey, instr, videoTitle, cfg.NvidiaModels)
			if err == nil && TranslationLooksTarget(texts, translated, targetLang) {
				return preserveSpeakerTags(texts, translated), ProviderNvidia + "/" + model, nil
			}
			lastErr = fmt.Errorf("nvidia (%s): %w", modelOrEmpty(model), errIf(err))

		case ProviderGemini:
			if cfg.GeminiKey == "" {
				continue
			}
			translated, model, err := TranslateWithGemini(ctx, texts, targetLang, sourceLang, cfg.GeminiKey, instr, videoTitle, cfg.GeminiModels)
			if err == nil && TranslationLooksTarget(texts, translated, targetLang) {
				return preserveSpeakerTags(texts, translated), ProviderGemini + "/" + model, nil
			}
			lastErr = fmt.Errorf("gemini (%s): %w", modelOrEmpty(model), errIf(err))

		case ProviderGroq:
			if cfg.GroqKey == "" {
				continue
			}
			translated, model, err := TranslateWithGroq(ctx, texts, targetLang, sourceLang, cfg.GroqKey, instr, videoTitle, cfg.GroqModels)
			if err == nil && TranslationLooksTarget(texts, translated, targetLang) {
				return preserveSpeakerTags(texts, translated), ProviderGroq + "/" + model, nil
			}
			lastErr = fmt.Errorf("groq (%s): %w", modelOrEmpty(model), errIf(err))
		}
	}

	if lastErr != nil {
		return nil, "", lastErr
	}
	return nil, "", fmt.Errorf("all translation providers failed or unconfigured")
}

func modelOrEmpty(m string) string {
	if m == "" {
		return "unknown"
	}
	return m
}

func errIf(err error) error {
	if err != nil {
		return err
	}
	return fmt.Errorf("output failed language validation")
}

// TranslationLooksTarget rejects responses that mostly stayed in the source language (refusals, leaked reasoning).
func TranslationLooksTarget(src, res []string, targetLang string) bool {
	if len(res) == 0 || len(src) == 0 {
		return false
	}
	wantCyrillic := targetLang == "ru"
	bad := 0
	for i, line := range res {
		srcLine := ""
		if i < len(src) {
			srcLine = speakerTagRe.ReplaceAllString(strings.TrimSpace(src[i]), "")
		}
		body := speakerTagRe.ReplaceAllString(strings.TrimSpace(line), "")

		if wantCyrillic {
			cyr := 0
			for _, r := range body {
				if unicode.Is(unicode.Cyrillic, r) {
					cyr++
				}
			}
			if cyr == 0 || looksLikePassthrough(srcLine, body) {
				bad++
			}
			continue
		}
		lat := false
		for _, r := range body {
			if r < 128 && unicode.IsLetter(r) {
				lat = true
				break
			}
		}
		if !lat || looksLikePassthrough(srcLine, body) {
			bad++
		}
	}
	return bad*4 <= len(res)
}

func looksLikePassthrough(srcLine, out string) bool {
	if srcLine == "" || out == "" {
		return false
	}
	srcWords := wordSet(srcLine)
	if len(srcWords) == 0 {
		return false
	}
	outLower := strings.ToLower(out)
	hit := 0
	for w := range srcWords {
		if strings.Contains(outLower, w) {
			hit++
		}
	}
	return hit*2 >= len(srcWords)
}

func wordSet(s string) map[string]struct{} {
	set := make(map[string]struct{})
	for _, w := range strings.Fields(strings.ToLower(s)) {
		w = strings.Trim(w, ".,!?\"'()[]")
		set[w] = struct{}{}
	}
	return set
}

// sanitizeSubtitleTypography normalizes dashes, quotes and spaces, dropping zero-width chars that render as tofu boxes.
func sanitizeSubtitleTypography(s string) string {
	var sb strings.Builder
	sb.Grow(len(s))
	for _, r := range s {
		switch r {
		case '\u2010', '\u2011', '\u2012', '\u2013', '\u2212', '\u00AD':
			sb.WriteByte('-')
		case '\u00A0', '\u2000', '\u2001', '\u2002', '\u2003', '\u2004', '\u2005', '\u2006', '\u2007', '\u2008', '\u2009', '\u200A', '\u202F', '\u205F', '\u3000':
			sb.WriteByte(' ')
		case '\u200B', '\u200C', '\uFEFF':
		case '\u2018', '\u2019', '\u201A', '\u201B':
			sb.WriteByte('\'')
		case '\u201C', '\u201D', '\u201E', '\u201F':
			sb.WriteByte('"')
		default:
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

func preserveSpeakerTags(original, translated []string) []string {
	out := make([]string, len(translated))
	for i, tr := range translated {
		tr = sanitizeSubtitleTypography(tr)
		if i >= len(original) {
			out[i] = tr
			continue
		}
		orig := original[i]
		if match := speakerTagRe.FindString(orig); match != "" {
			tag := strings.TrimSpace(match)
			trTrim := strings.TrimSpace(tr)
			if !strings.HasPrefix(trTrim, tag) && !strings.HasPrefix(trTrim, "[") && !strings.HasPrefix(trTrim, "(") {
				out[i] = tag + " " + trTrim
			} else {
				out[i] = tr
			}
		} else {
			out[i] = tr
		}
	}
	return out
}

// isReasoningModel reports models that spend output tokens on hidden reasoning.
func isReasoningModel(model string) bool {
	return strings.Contains(model, "gpt-oss") ||
		strings.Contains(model, "big-pickle") ||
		strings.Contains(model, "minimax")
}

// maxOutputTokens scales the budget with batch size; reasoning models get extra headroom for hidden reasoning.
func maxOutputTokens(lines int, model string) int {
	tokens := lines*60 + 256
	if isReasoningModel(model) {
		tokens += 2048
	}
	if tokens < 1024 {
		return 1024
	}
	return tokens
}

// llmTimeout bounds a single LLM request; a whole subtitle file goes out in one call.
func llmTimeout() time.Duration {
	if v, err := time.ParseDuration(os.Getenv("TRANSLATION_TIMEOUT")); err == nil && v > 0 {
		return v
	}
	return 180 * time.Second
}

var mdBoldRe = regexp.MustCompile(`\*\*([^*]+)\*\*`)

// stripMarkdownBold removes markdown bold markers without touching lone asterisks.
func stripMarkdownBold(s string) string {
	return mdBoldRe.ReplaceAllString(s, "$1")
}

func TranslateWithGoogle(ctx context.Context, texts []string, targetLang string) ([]string, error) {
	var out []string
	for _, batch := range batchByRuneLimit(texts, googleMaxQueryRunes) {
		part, err := translateWithGoogleBatch(ctx, batch, targetLang)
		if err != nil {
			return nil, err
		}
		out = append(out, part...)
	}
	return out, nil
}

// googleMaxQueryRunes keeps each GET query safely under the URL length limit.
const googleMaxQueryRunes = 1400

func batchByRuneLimit(texts []string, limit int) [][]string {
	var batches [][]string
	var cur []string
	size := 0
	for _, t := range texts {
		if size > 0 && size+len(t)+1 > limit {
			batches = append(batches, cur)
			cur = nil
			size = 0
		}
		cur = append(cur, t)
		size += len(t) + 1
	}
	if len(cur) > 0 {
		batches = append(batches, cur)
	}
	return batches
}

func translateWithGoogleBatch(ctx context.Context, texts []string, targetLang string) ([]string, error) {
	joined := strings.Join(texts, "\n")
	apiURL := fmt.Sprintf(
		"https://translate.googleapis.com/translate_a/single?client=gtx&sl=auto&tl=%s&dt=t&q=%s",
		url.QueryEscape(targetLang),
		url.QueryEscape(joined),
	)

	client := &http.Client{Timeout: 30 * time.Second}

	var body []byte
	err := retry.Do(
		func() error {
			req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
			if reqErr != nil {
				return retry.Unrecoverable(reqErr)
			}
			req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")

			resp, doErr := client.Do(req)
			if doErr != nil {
				return fmt.Errorf("google translate request failed: %w", doErr)
			}
			defer func() { _ = resp.Body.Close() }()

			b, readErr := io.ReadAll(resp.Body)
			if readErr != nil {
				return readErr
			}
			if resp.StatusCode == http.StatusOK {
				body = b
				return nil
			}
			statusErr := fmt.Errorf("google translate returned status %d: %s", resp.StatusCode, truncate(string(b), 300))
			if resp.StatusCode != http.StatusTooManyRequests && resp.StatusCode < 500 {
				return retry.Unrecoverable(statusErr)
			}
			return statusErr
		},
		retry.Context(ctx),
		retry.Attempts(4),
		retry.DelayType(retry.BackOffDelay),
		retry.Delay(3*time.Second),
		retry.LastErrorOnly(true),
	)
	if err != nil {
		return nil, err
	}

	var raw []interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse google translate response: %w", err)
	}

	if len(raw) == 0 {
		return nil, fmt.Errorf("empty google translate response")
	}

	segments, ok := raw[0].([]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected google translate response format")
	}

	var sb strings.Builder
	for _, seg := range segments {
		if pair, ok := seg.([]interface{}); ok && len(pair) > 0 {
			if str, ok := pair[0].(string); ok {
				sb.WriteString(str)
			}
		}
	}

	result := sb.String()
	lines := strings.Split(strings.ReplaceAll(result, "\r\n", "\n"), "\n")

	if len(lines) != len(texts) {
		lines = alignTranslatedLines(lines, len(texts))
	}

	return lines, nil
}
func alignTranslatedLines(lines []string, expectedLen int) []string {
	if len(lines) == expectedLen {
		return lines
	}
	out := make([]string, expectedLen)
	for i := range out {
		if i < len(lines) {
			out[i] = lines[i]
		}
	}
	return out
}

func TranslateWithGemini(ctx context.Context, texts []string, targetLang, sourceLang, apiKey, systemInstruction string, videoTitle string, models ...[]string) ([]string, string, error) {
	geminiModels := firstModels(models, DefaultGeminiModels)
	userMsg := buildTranslateUserMsg(texts, targetLang, sourceLang, videoTitle)

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:     apiKey,
		Backend:    genai.BackendGeminiAPI,
		HTTPClient: &http.Client{Timeout: llmTimeout()},
	})
	if err != nil {
		return nil, "", fmt.Errorf("gemini client: %w", err)
	}

	temp := float32(0.2)

	var lastErr error
	for _, model := range geminiModels {
		withSafety, withThinking := true, true
		for attempt := 0; attempt < 3; attempt++ {
			config := &genai.GenerateContentConfig{
				SystemInstruction: &genai.Content{
					Parts: []*genai.Part{genai.NewPartFromText(translatorRole + "\n" + systemInstruction)},
				},
				Temperature:     &temp,
				MaxOutputTokens: int32(maxOutputTokens(len(texts), model)),
			}
			if withThinking {
				budget := int32(0)
				config.ThinkingConfig = &genai.ThinkingConfig{ThinkingBudget: &budget}
			}
			if withSafety {
				config.SafetySettings = geminiSafetySettings()
			}

			resp, cerr := client.Models.GenerateContent(ctx, model, genai.Text(userMsg), config)
			if cerr != nil {
				lastErr = fmt.Errorf("gemini (%s): %w", model, cerr)
				code, msg := geminiAPIErrorDetails(cerr)
				switch {
				case code == http.StatusBadRequest && withSafety && strings.Contains(msg, "safety"):
					withSafety = false
					continue
				case code == http.StatusBadRequest && withThinking && strings.Contains(msg, "thinking"):
					withThinking = false
					continue
				}
				break
			}

			outText := resp.Text()
			if strings.TrimSpace(outText) == "" || len(resp.Candidates) == 0 {
				lastErr = fmt.Errorf("empty response from gemini (%s)", model)
				break
			}

			res, perr := parseNumberedLines(stripMarkdownBold(thinkTagRe.ReplaceAllString(outText, "")), len(texts))
			if perr == nil {
				return res, model, nil
			}
			lastErr = perr
			break
		}
	}

	return nil, "", lastErr
}

// geminiSafetySettings disables blocking across every harm category.
func geminiSafetySettings() []*genai.SafetySetting {
	categories := []genai.HarmCategory{
		genai.HarmCategoryHarassment,
		genai.HarmCategoryHateSpeech,
		genai.HarmCategorySexuallyExplicit,
		genai.HarmCategoryDangerousContent,
		genai.HarmCategoryCivicIntegrity,
	}
	settings := make([]*genai.SafetySetting, 0, len(categories))
	for _, c := range categories {
		settings = append(settings, &genai.SafetySetting{Category: c, Threshold: genai.HarmBlockThresholdBlockNone})
	}
	return settings
}

// geminiAPIErrorDetails extracts the status code and lowercased message from a genai API error.
func geminiAPIErrorDetails(err error) (int, string) {
	var apiErr genai.APIError
	if errors.As(err, &apiErr) {
		return apiErr.Code, strings.ToLower(apiErr.Message)
	}
	return 0, strings.ToLower(err.Error())
}

// OpenAI-compatible endpoints used by the translation providers.
const (
	groqBaseURL     = "https://api.groq.com/openai/v1"
	opencodeBaseURL = "https://opencode.ai/zen/v1"
	nvidiaBaseURL   = "https://integrate.api.nvidia.com/v1"
)

func TranslateWithGroq(ctx context.Context, texts []string, targetLang, sourceLang, apiKey, systemInstruction string, videoTitle string, models ...[]string) ([]string, string, error) {
	return translateOpenAICompatible(ctx, ProviderGroq, groqBaseURL, apiKey, firstModels(models, DefaultGroqModels), texts, targetLang, sourceLang, systemInstruction, videoTitle)
}

// TranslateWithOpencode calls OpenCode Zen (OpenAI-compatible) with the given free-model chain.
func TranslateWithOpencode(ctx context.Context, texts []string, targetLang, sourceLang, apiKey, systemInstruction string, videoTitle string, models ...[]string) ([]string, string, error) {
	return translateOpenAICompatible(ctx, ProviderOpencode, opencodeBaseURL, apiKey, firstModels(models, DefaultOpencodeModels), texts, targetLang, sourceLang, systemInstruction, videoTitle)
}

// TranslateWithNvidia calls NVIDIA NIM (OpenAI-compatible) with the given model chain.
func TranslateWithNvidia(ctx context.Context, texts []string, targetLang, sourceLang, apiKey, systemInstruction string, videoTitle string, models ...[]string) ([]string, string, error) {
	return translateOpenAICompatible(ctx, ProviderNvidia, nvidiaBaseURL, apiKey, firstModels(models, DefaultNvidiaModels), texts, targetLang, sourceLang, systemInstruction, videoTitle)
}

// firstModels returns the caller-supplied model chain or the default one.
func firstModels(models []([]string), def []string) []string {
	if len(models) > 0 && len(models[0]) > 0 {
		return models[0]
	}
	return def
}

// translateOpenAICompatible walks an OpenAI-compatible model chain until one produces parseable output.
func translateOpenAICompatible(ctx context.Context, provider, baseURL, apiKey string, models []string, texts []string, targetLang, sourceLang, systemInstruction, videoTitle string) ([]string, string, error) {
	userMsg := buildTranslateUserMsg(texts, targetLang, sourceLang, videoTitle)

	cfg := openai.DefaultConfig(apiKey)
	cfg.BaseURL = baseURL
	cfg.HTTPClient = &http.Client{Timeout: llmTimeout()}
	client := openai.NewClientWithConfig(cfg)

	var lastErr error
	for _, model := range models {
		req := openai.ChatCompletionRequest{
			Model: model,
			Messages: []openai.ChatCompletionMessage{
				{Role: openai.ChatMessageRoleSystem, Content: translatorRole + "\n" + systemInstruction},
				{Role: openai.ChatMessageRoleUser, Content: userMsg},
			},
			Temperature: 0.2,
			MaxTokens:   maxOutputTokens(len(texts), model),
		}
		if isReasoningModel(model) {
			req.ReasoningEffort = "low"
		}

		resp, err := createChatCompletionWithRetry(ctx, client, req)
		if err != nil {
			lastErr = fmt.Errorf("%s (%s): %w", provider, model, err)
			continue
		}

		outText := thinkTagRe.ReplaceAllString(resp.Choices[0].Message.Content, "")
		res, perr := parseNumberedLines(stripMarkdownBold(outText), len(texts))
		if perr == nil {
			return res, model, nil
		}
		lastErr = perr
	}

	return nil, "", lastErr
}

// createChatCompletionWithRetry sends the request, retrying once on 429/5xx; rejects empty completions.
func createChatCompletionWithRetry(ctx context.Context, client *openai.Client, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
	var resp openai.ChatCompletionResponse
	err := retry.Do(
		func() error {
			r, cerr := client.CreateChatCompletion(ctx, req)
			if cerr != nil {
				return cerr
			}
			resp = r
			return nil
		},
		retry.Context(ctx),
		retry.Attempts(2),
		retry.Delay(3*time.Second),
		retry.RetryIf(isRetryableOpenAIError),
		retry.LastErrorOnly(true),
	)
	if err != nil {
		return openai.ChatCompletionResponse{}, err
	}
	if len(resp.Choices) == 0 || strings.TrimSpace(resp.Choices[0].Message.Content) == "" {
		return resp, fmt.Errorf("empty response")
	}
	return resp, nil
}

// isRetryableOpenAIError reports whether the request should be retried (rate limits and server errors).
func isRetryableOpenAIError(err error) bool {
	var apiErr *openai.APIError
	return errors.As(err, &apiErr) &&
		(apiErr.HTTPStatusCode == http.StatusTooManyRequests || apiErr.HTTPStatusCode >= http.StatusInternalServerError)
}

func buildNumberedLines(texts []string) string {
	var sb strings.Builder
	for i, t := range texts {
		fmt.Fprintf(&sb, "%d. %s\n", i+1, t)
	}
	return sb.String()
}

func buildTranslateUserMsg(texts []string, targetLang, sourceLang, videoTitle string) string {
	var context string
	if l := strings.TrimSpace(sourceLang); l != "" {
		context += fmt.Sprintf("\nSource language: %s (%s)", langDisplayName(l), l)
	}
	if t := strings.TrimSpace(videoTitle); t != "" {
		context += fmt.Sprintf("\nVideo context / Title: %q", t)
	}
	return fmt.Sprintf(
		"Translate the following subtitle lines into %s (BCP-47 code %q). These lines are a contiguous excerpt from one video, translate them consistently with each other.%s\nCopy every emoji and symbol (♪, ~, ★) from the input line into the output unchanged and in place; never replace an emoji with a word, a description or a bracket note. Output plain UTF-8 text; never use \\uXXXX escapes.\nReturn EXACTLY the same number of numbered lines as provided, keeping line numbers (\"1. Translated text\"). Do not add preamble or explanations.\n\nInput lines:\n%s",
		langDisplayName(targetLang),
		targetLang,
		context,
		buildNumberedLines(texts),
	)
}

func parseNumberedLines(raw string, expectedCount int) ([]string, error) {
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	result := make([]string, expectedCount)

	lineIdxRe := regexp.MustCompile(`^(?:\*{0,2})?(\d+)[.)\]:]\s*(?:\*{0,2})?\s*(.*)$`)

	foundCount := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		if m := lineIdxRe.FindStringSubmatch(trimmed); len(m) >= 3 {
			idx, err := strconv.Atoi(m[1])
			if err == nil && idx >= 1 && idx <= expectedCount {
				result[idx-1] = sanitizeSubtitleTypography(strings.TrimSpace(m[2]))
				foundCount++
			}
		}
	}

	if foundCount >= expectedCount/2 && foundCount > 0 {
		for i, r := range result {
			if r == "" {
				result[i] = ""
			}
		}
		return result, nil
	}

	var nonNumbered []string
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			nonNumbered = append(nonNumbered, sanitizeSubtitleTypography(strings.TrimSpace(l)))
		}
	}
	if len(nonNumbered) == expectedCount {
		return nonNumbered, nil
	}

	return nil, fmt.Errorf("translated line count mismatch: expected %d, got %d numbered lines", expectedCount, foundCount)
}
