package downloader

import (
	"bytes"
	"context"
	"encoding/json"
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

	"go.uber.org/zap"
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
	workbench := s.workbenchDir(videoID)

	res, err := ResolveSubtitleTrack(meta, requestedLang)
	if err != nil {
		return "", err
	}

	fullVTT := filepath.Join(workbench, "fullsubs_"+videoID+"."+res.SourceLang+".vtt")
	if _, err := os.Stat(fullVTT); os.IsNotExist(err) {
		dlErr := downloadSubtitlesDirect(ctx, res.TrackURL, cookieFile, fullVTT)
		if dlErr != nil {
			slog.Warn("direct subtitle download failed, falling back to yt-dlp",
				"video_id", videoID, "error", dlErr)
			if fbErr := s.downloadSubtitlesWithRetry(ctx, job, res.FinalLang, cookieFile, fullVTT); fbErr != nil {
				return "", fmt.Errorf("failed to download subtitles: %w", dlErr)
			}
		}
	}
	if _, err := os.Stat(fullVTT); err != nil {
		return "", fmt.Errorf("no subtitles found for language %q", res.FinalLang)
	}

	trimmedPath := filepath.Join(workbench, fileNameWithTimecode(videoID, startMs, endMs)+"_trimmed.vtt")
	cues, err := TrimVTTFile(fullVTT, startMs, endMs, trimmedPath)
	if err != nil {
		return "", fmt.Errorf("failed to process subtitles: %w", err)
	}
	if cues == 0 {
		return "", nil
	}

	if res.TargetLang != "" && res.TargetLang != res.SourceLang {
		if terr := s.translateVTTFile(ctx, trimmedPath, res.TargetLang); terr != nil {
			slog.Warn("vtt translation failed, using original source subtitles",
				"target_lang", res.TargetLang, "error", terr)
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

func (s *Service) translateVTTFile(ctx context.Context, vttPath, targetLang string) error {
	data, err := os.ReadFile(vttPath)
	if err != nil {
		return err
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
		return nil
	}

	var texts []string
	for _, c := range cues {
		texts = append(texts, c.text)
	}

	translated, _, err := s.TranslateTextsWithFallback(ctx, texts, targetLang)
	if err != nil {
		return err
	}

	var b strings.Builder
	b.WriteString("WEBVTT\nKind: captions\nLanguage: " + targetLang + "\n\n")
	for i, c := range cues {
		t := c.text
		if i < len(translated) && strings.TrimSpace(translated[i]) != "" {
			t = translated[i]
		}
		b.WriteString(c.ts + "\n" + t + "\n\n")
	}

	return os.WriteFile(vttPath, []byte(b.String()), 0644)
}

func parseNetscapeCookies(path string) []*http.Cookie {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var cookies []*http.Cookie
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#HttpOnly_") {
			line = strings.TrimPrefix(line, "#HttpOnly_")
		} else if strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) >= 7 {
			cookies = append(cookies, &http.Cookie{
				Name:  parts[5],
				Value: parts[6],
			})
		}
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
	Description       string                     `json:"description"`
	Tags              []string                   `json:"tags"`
	Duration          float64                    `json:"duration"`
	Subtitles         map[string][]SubtitleTrack `json:"subtitles"`
	AutomaticCaptions map[string][]SubtitleTrack `json:"automatic_captions"`
}

// FormatCaption builds a clean, minimalist HTML caption (Title + up to 5 hashtags).
func FormatCaption(meta *SourceMeta) string {
	if meta == nil {
		return ""
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

	if title == "" {
		return ""
	}

	caption := "<b>" + htmlEscape(title) + "</b>"

	if len(meta.Tags) > 0 {
		var tagList []string
		titleLower := strings.ToLower(title)
		for _, tag := range meta.Tags {
			tag = strings.TrimSpace(tag)
			if tag == "" {
				continue
			}
			tagClean := strings.ReplaceAll(tag, " ", "")
			if !strings.Contains(titleLower, "#"+strings.ToLower(tagClean)) {
				tagList = append(tagList, "#"+tagClean)
				if len(tagList) >= 5 {
					break
				}
			}
		}
		if len(tagList) > 0 {
			caption += "\n\n" + strings.Join(tagList, " ")
		}
	}

	if len(caption) > 1000 {
		caption = caption[:997] + "…"
	}
	return caption
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

var regionalTagRe = func(lang string) *regexp.Regexp {
	return regexp.MustCompile("^" + regexp.QuoteMeta(lang) + "(?:-[A-Za-z0-9]+)*$")
}

// SubtitleTrackResolution contains the selected source track URL and whether it needs translation.
type SubtitleTrackResolution struct {
	TrackURL   string // pre-signed direct YouTube URL from info.json
	SourceLang string // base track language, e.g. "ko", "en", "ja"
	TargetLang string // translation target, e.g. "ru"; "" if direct
	FinalLang  string // resulting language code
}

func ResolveSubtitleTrack(meta *SourceMeta, requested string) (*SubtitleTrackResolution, error) {
	if meta == nil || (len(meta.Subtitles) == 0 && len(meta.AutomaticCaptions) == 0) {
		return nil, fmt.Errorf("no subtitles found for this video")
	}

	target := normalizeLang(requested)
	if target == "" {
		target = "en"
	}

	if directKey, ok := findLanguage(meta.Subtitles, target); ok {
		tracks := meta.Subtitles[directKey]
		if len(tracks) > 0 && tracks[0].URL != "" {
			return &SubtitleTrackResolution{
				TrackURL:   tracks[0].URL,
				SourceLang: directKey,
				TargetLang: "",
				FinalLang:  target,
			}, nil
		}
	}

	preferred := []string{"en", "ko", "ja", "zh", "es", "fr", "de", "vi"}
	if target == "en" {
		preferred = []string{"ko", "ja", "zh", "es", "fr", "de", "vi", "en"}
	}

	if len(meta.Subtitles) > 0 {
		baseKey := pickBestBaseTrack(meta.Subtitles, preferred)
		if baseKey == "" {
			for k := range meta.Subtitles {
				baseKey = k
				break
			}
		}
		if baseKey != "" {
			tracks := meta.Subtitles[baseKey]
			if len(tracks) > 0 && tracks[0].URL != "" {
				return &SubtitleTrackResolution{
					TrackURL:   tracks[0].URL,
					SourceLang: baseKey,
					TargetLang: target,
					FinalLang:  target,
				}, nil
			}
		}
	}

	if directKey, ok := findLanguage(meta.AutomaticCaptions, target); ok {
		tracks := meta.AutomaticCaptions[directKey]
		if len(tracks) > 0 && tracks[0].URL != "" {
			return &SubtitleTrackResolution{
				TrackURL:   tracks[0].URL,
				SourceLang: directKey,
				TargetLang: "",
				FinalLang:  target,
			}, nil
		}
	}

	if len(meta.AutomaticCaptions) > 0 {
		baseKey := pickBestBaseTrack(meta.AutomaticCaptions, preferred)
		if baseKey == "" {
			for k := range meta.AutomaticCaptions {
				baseKey = k
				break
			}
		}
		if baseKey != "" {
			tracks := meta.AutomaticCaptions[baseKey]
			if len(tracks) > 0 && tracks[0].URL != "" {
				return &SubtitleTrackResolution{
					TrackURL:   tracks[0].URL,
					SourceLang: baseKey,
					TargetLang: target,
					FinalLang:  target,
				}, nil
			}
		}
	}

	return nil, fmt.Errorf("no available subtitles for language %q", target)
}

func pickBestBaseTrack(subs map[string][]SubtitleTrack, preferred []string) string {
	for _, pref := range preferred {
		if k, ok := findLanguage(subs, pref); ok {
			return k
		}
	}
	return ""
}

func findLanguage(available map[string][]SubtitleTrack, requested string) (string, bool) {
	if _, ok := available[requested]; ok {
		return requested, true
	}
	return firstLanguage(available, requested)
}

func firstLanguage(available map[string][]SubtitleTrack, prefix string) (string, bool) {
	re := regionalTagRe(prefix)
	keys := make([]string, 0, len(available))
	for key := range available {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if re.MatchString(key) {
			return key, true
		}
	}
	return "", false
}

func normalizeLang(lang string) string {
	switch strings.ToLower(strings.TrimSpace(lang)) {
	case "", "en":
		return "en"
	case "ru":
		return "ru"
	case "ko":
		return "ko"
	default:
		return strings.TrimSpace(lang)
	}
}

const (
	ProviderGoogle = "google"
	ProviderGemini = "gemini"
	ProviderGroq   = "groq"
)

const DefaultTranslationPrompt = `RULES ON NAMES & PROPER NOUNS:
1. NEVER translate proper names, stage names, idol/artist names, group names, or personal names into dictionary words/common nouns (e.g., 'Winter', 'Joy', 'Solar', 'Rain', 'Summer' must remain names and not be translated into common seasonal/weather nouns). Keep names in their original Latin form or as proper name transliterations.
2. NEVER translate or delete speaker/singer tags or bracketed identifiers (e.g. [SUI], [WONYOUNG], [ALL], (Chorus), [수이], SUI:) — keep them verbatim at the start of each line.
3. Preserve emotional tone, slang, honorifics (like Unnie/Oppa/Hyung if appropriate) and conversational/lyric style.`

var speakerTagRe = regexp.MustCompile(`^(\[[^\]]+\]|\([^\)]+\)|[A-Za-z0-9_가-힣]+:)\s*`)

// TranslationConfig holds the active provider selection, API keys, and custom prompt.
type TranslationConfig struct {
	PrimaryProvider string // "google", "gemini", "groq"
	GeminiKey       string
	GroqKey         string
	Prompt          string
}

func (s *Service) ResolveTranslationConfig(ctx context.Context) TranslationConfig {
	cfg := TranslationConfig{
		PrimaryProvider: strings.ToLower(strings.TrimSpace(os.Getenv("TRANSLATION_PROVIDER"))),
		GeminiKey:       strings.TrimSpace(os.Getenv("GEMINI_API_KEY")),
		GroqKey:         strings.TrimSpace(os.Getenv("GROQ_API_KEY")),
		Prompt:          strings.TrimSpace(os.Getenv("TRANSLATION_PROMPT")),
	}

	if s.configs != nil {
		if val, err := s.configs.Get(ctx, "TRANSLATION_PROVIDER"); err == nil && val != nil && val.Value != "" {
			cfg.PrimaryProvider = strings.ToLower(strings.TrimSpace(val.Value))
		}
		if val, err := s.configs.Get(ctx, "GEMINI_API_KEY"); err == nil && val != nil && val.Value != "" {
			cfg.GeminiKey = strings.TrimSpace(val.Value)
		}
		if val, err := s.configs.Get(ctx, "GROQ_API_KEY"); err == nil && val != nil && val.Value != "" {
			cfg.GroqKey = strings.TrimSpace(val.Value)
		}
		if val, err := s.configs.Get(ctx, "TRANSLATION_PROMPT"); err == nil && val != nil && val.Value != "" {
			cfg.Prompt = strings.TrimSpace(val.Value)
		}
	}

	if cfg.PrimaryProvider == "" {
		cfg.PrimaryProvider = ProviderGoogle
	}
	if cfg.Prompt == "" {
		cfg.Prompt = DefaultTranslationPrompt
	}

	return cfg
}

func buildFallbackChain(primary string) []string {
	if primary != ProviderGoogle && primary != ProviderGemini && primary != ProviderGroq {
		primary = ProviderGoogle
	}
	chain := []string{primary}
	all := []string{ProviderGemini, ProviderGroq, ProviderGoogle}
	for _, p := range all {
		if p != primary {
			chain = append(chain, p)
		}
	}
	return chain
}

func (s *Service) TranslateTextsWithFallback(ctx context.Context, texts []string, targetLang string) ([]string, string, error) {
	if len(texts) == 0 {
		return nil, "", nil
	}

	cfg := s.ResolveTranslationConfig(ctx)
	chain := buildFallbackChain(cfg.PrimaryProvider)

	var lastErr error
	for _, provider := range chain {
		switch provider {
		case ProviderGoogle:
			translated, err := TranslateWithGoogle(ctx, texts, targetLang)
			if err == nil {
				return preserveSpeakerTags(texts, translated), ProviderGoogle, nil
			}
			lastErr = err
			s.logger.Warn("Google translate failed, attempting fallback", zap.Error(err))

		case ProviderGemini:
			if cfg.GeminiKey == "" {
				continue
			}
			translated, err := TranslateWithGemini(ctx, texts, targetLang, cfg.GeminiKey, cfg.Prompt)
			if err == nil {
				return preserveSpeakerTags(texts, translated), ProviderGemini, nil
			}
			lastErr = err
			s.logger.Warn("Gemini translate failed, attempting fallback", zap.Error(err))

		case ProviderGroq:
			if cfg.GroqKey == "" {
				continue
			}
			translated, err := TranslateWithGroq(ctx, texts, targetLang, cfg.GroqKey, cfg.Prompt)
			if err == nil {
				return preserveSpeakerTags(texts, translated), ProviderGroq, nil
			}
			lastErr = err
			s.logger.Warn("Groq translate failed, attempting fallback", zap.Error(err))
		}
	}

	if lastErr != nil {
		return nil, "", lastErr
	}
	return nil, "", fmt.Errorf("all translation providers failed or unconfigured")
}

func preserveSpeakerTags(original, translated []string) []string {
	out := make([]string, len(translated))
	for i, tr := range translated {
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

func TranslateWithGoogle(ctx context.Context, texts []string, targetLang string) ([]string, error) {
	joined := strings.Join(texts, "\n")
	apiURL := fmt.Sprintf(
		"https://translate.googleapis.com/translate_a/single?client=gtx&sl=auto&tl=%s&dt=t&q=%s",
		url.QueryEscape(targetLang),
		url.QueryEscape(joined),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("google translate request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google translate returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
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

func TranslateWithGemini(ctx context.Context, texts []string, targetLang, apiKey, systemInstruction string) ([]string, error) {
	geminiModels := []string{"gemini-2.5-flash", "gemini-2.5-flash-lite", "gemini-flash-latest"}

	numbered := buildNumberedLines(texts)
	promptText := fmt.Sprintf(
		"Translate the following subtitle lines into language %q (BCP-47 code).\n\n%s\n\nReturn EXACTLY the same number of numbered lines as provided, keeping line numbers (\"1. Translated text\"). Do not add explanations or notes.\n\nInput lines:\n%s",
		targetLang,
		systemInstruction,
		numbered,
	)

	reqPayload := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"parts": []map[string]string{
					{"text": promptText},
				},
			},
		},
		"generationConfig": map[string]interface{}{
			"temperature":     0.2,
			"maxOutputTokens": 4096,
		},
	}
	payloadBytes, _ := json.Marshal(reqPayload)

	client := &http.Client{Timeout: 45 * time.Second}
	var lastErr error

	for _, model := range geminiModels {
		apiURL := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", model, apiKey)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(payloadBytes))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("gemini request failed (%s): %w", model, err)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			lastErr = fmt.Errorf("gemini api error %d (%s): %s", resp.StatusCode, model, truncate(string(b), 300))
			continue
		}

		var respBody struct {
			Candidates []struct {
				Content struct {
					Parts []struct {
						Text string `json:"text"`
					} `json:"parts"`
				} `json:"content"`
			} `json:"candidates"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
			_ = resp.Body.Close()
			lastErr = fmt.Errorf("failed to decode gemini response (%s): %w", model, err)
			continue
		}
		_ = resp.Body.Close()

		if len(respBody.Candidates) == 0 || len(respBody.Candidates[0].Content.Parts) == 0 {
			lastErr = fmt.Errorf("empty response from gemini (%s)", model)
			continue
		}

		outText := respBody.Candidates[0].Content.Parts[0].Text
		outText = thinkTagRe.ReplaceAllString(outText, "")
		res, perr := parseNumberedLines(outText, len(texts))
		if perr == nil {
			return res, nil
		}
		lastErr = perr
	}

	return nil, lastErr
}

func TranslateWithGroq(ctx context.Context, texts []string, targetLang, apiKey, systemInstruction string) ([]string, error) {
	apiURL := "https://api.groq.com/openai/v1/chat/completions"
	groqModels := []string{"qwen/qwen3.6-27b", "openai/gpt-oss-120b", "groq/compound"}

	numbered := buildNumberedLines(texts)
	userMsg := fmt.Sprintf(
		"Translate the following subtitle lines into language %q (BCP-47 code).\nReturn EXACTLY the same number of numbered lines as provided, keeping line numbers (\"1. Translated text\"). Do not add preamble or explanations.\n\nInput lines:\n%s",
		targetLang,
		numbered,
	)

	client := &http.Client{Timeout: 45 * time.Second}
	var lastErr error

	for _, model := range groqModels {
		reqPayload := map[string]interface{}{
			"model": model,
			"messages": []map[string]string{
				{"role": "system", "content": "You are a professional subtitle translator.\n" + systemInstruction},
				{"role": "user", "content": userMsg},
			},
			"temperature": 0.2,
			"max_tokens":  4096,
		}

		payloadBytes, _ := json.Marshal(reqPayload)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(payloadBytes))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+apiKey)

		resp, err := client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("groq request failed (%s): %w", model, err)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			lastErr = fmt.Errorf("groq api error %d (%s): %s", resp.StatusCode, model, truncate(string(b), 300))
			continue
		}

		var respBody struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
			_ = resp.Body.Close()
			lastErr = fmt.Errorf("failed to decode groq response (%s): %w", model, err)
			continue
		}
		_ = resp.Body.Close()

		if len(respBody.Choices) == 0 {
			lastErr = fmt.Errorf("empty choices from groq (%s)", model)
			continue
		}

		outText := respBody.Choices[0].Message.Content
		outText = thinkTagRe.ReplaceAllString(outText, "")
		res, perr := parseNumberedLines(outText, len(texts))
		if perr == nil {
			return res, nil
		}
		lastErr = perr
	}

	return nil, lastErr
}

func buildNumberedLines(texts []string) string {
	var sb strings.Builder
	for i, t := range texts {
		fmt.Fprintf(&sb, "%d. %s\n", i+1, t)
	}
	return sb.String()
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
				result[idx-1] = strings.TrimSpace(m[2])
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
			nonNumbered = append(nonNumbered, strings.TrimSpace(l))
		}
	}
	if len(nonNumbered) == expectedCount {
		return nonNumbered, nil
	}

	return nil, fmt.Errorf("translated line count mismatch: expected %d, got %d numbered lines", expectedCount, foundCount)
}
