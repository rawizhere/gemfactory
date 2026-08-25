package downloader

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	cookiemonster "github.com/MercuryEngineering/CookieMonster"
	"go.uber.org/zap"

	"gemfactory/internal/translate"
)

// vttTagReplacer strips YouTube auto-caption styling tags.
var vttTagReplacer = strings.NewReplacer("<c>", "", "</c>", "")

// inlineWordTimestampRe matches inline word-timing tags such as <00:00:00.546> found in YouTube auto-generated subtitle payloads.
var inlineWordTimestampRe = regexp.MustCompile(`<\d{2}:\d{2}:\d{2}\.\d{3}>`)

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
			s.logger.Warn("direct subtitle download failed, falling back to yt-dlp",
				zap.String("video_id", videoID), zap.Error(dlErr))
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
		var onAttempt func(string)
		if cbs := job.callbacks; cbs != nil {
			onAttempt = cbs.OnTranslateAttempt
		}
		prov, terr := s.translateVTTFile(ctx, trimmedPath, res.TargetLang, res.SourceLang, videoTitle, job.Request.SubsNoLLM, onAttempt)
		if terr == nil && prov != "" {
			job.Translation = prov
			if job.callbacks != nil && job.callbacks.OnTranslation != nil {
				job.callbacks.OnTranslation(prov)
			}
		}
		if terr != nil {
			s.logger.Warn("vtt translation failed",
				zap.String("target_lang", res.TargetLang),
				zap.String("video_id", videoID),
				zap.Error(terr))
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

func (s *Service) translateVTTFile(ctx context.Context, vttPath, targetLang, sourceLang, videoTitle string, noLLM bool, onAttempt func(string)) (string, error) {
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
	translated, provider, err := translate.Chain(ctx, texts, targetLang, sourceLang, cfg, videoTitle, s.logger, onAttempt)
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
			t := translate.SanitizeTypography(strings.TrimSpace(translated[idx]))
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
	args := append(s.baseYTDLPArgs(ctx, cookieFile),
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
		s.logger.Warn("subtitle download attempt failed",
			zap.String("video_id", job.VideoID), zap.String("lang", lang),
			zap.Int("attempt", attempt), zap.Int("of", subtitleDownloadAttempts),
			zap.String("error", truncate(string(out), 500)))

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
			line = translate.SanitizeTypography(inlineWordTimestampRe.ReplaceAllString(vttTagReplacer.Replace(line), ""))
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
	Width             int                        `json:"width"`
	Height            int                        `json:"height"`
	Subtitles         map[string][]SubtitleTrack `json:"subtitles"`
	AutomaticCaptions map[string][]SubtitleTrack `json:"automatic_captions"`
}

// IsVertical reports whether the source video is taller than wide.
func (m *SourceMeta) IsVertical() bool {
	return m != nil && m.Height > 0 && m.Height > m.Width
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
		s.logger.Info("metadata cache hit", zap.String("video_id", videoID))
		return readSourceMeta(metaPath)
	}

	bin, err := YTDLPBinary()
	if err != nil {
		return nil, err
	}

	s.logger.Info("metadata extraction starting", zap.String("url", url), zap.String("video_id", videoID))
	start := time.Now()
	defer func() {
		s.logger.Info("metadata extraction finished",
			zap.String("url", url),
			zap.String("video_id", videoID),
			zap.Duration("elapsed", time.Since(start)))
	}()

	dir := filepath.Dir(metaPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create workbench dir: %w", err)
	}
	base := strings.TrimSuffix(metaPath, ".info.json")

	args := append(s.baseYTDLPArgs(ctx, cookieFile),
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

// ResolveTranslationConfig layers DB-stored settings on top of env defaults; the DB wins.
func (s *Service) ResolveTranslationConfig(ctx context.Context) translate.Config {
	cfg := translate.DefaultConfig()
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
		cfg.GeminiModels = translate.ParseCSV(v)
	}
	if v, ok := get("GROQ_MODELS"); ok {
		cfg.GroqModels = translate.ParseCSV(v)
	}
	if v, ok := get("OPENCODE_MODELS"); ok {
		cfg.OpencodeModels = translate.ParseCSV(v)
	}
	if v, ok := get("NVIDIA_MODELS"); ok {
		cfg.NvidiaModels = translate.ParseCSV(v)
	}
	if v, ok := get("TRANSLATION_FALLBACK_ORDER"); ok {
		cfg.FallbackOrder = translate.ParseCSV(v)
	}
	if v, ok := get("TRANSLATION_PROMPT"); ok {
		cfg.Prompt = v
	}
	if v, ok := get("SUBS_SOURCE_PREF_RU"); ok {
		cfg.SourcePrefRU = translate.ParseCSV(v)
	}
	if val, err := s.configs.Get(ctx, "SUBS_GOOGLE_ONLY"); err == nil && val != nil {
		cfg.GoogleOnly = translate.IsTruthy(val.Value)
	}
	if v, ok := get("TRANSLATION_TIMEOUT"); ok {
		if sec, err := strconv.Atoi(v); err == nil && sec > 0 {
			cfg.Timeout = time.Duration(sec) * time.Second
		}
	}
	return translate.ApplyDefaults(cfg)
}
