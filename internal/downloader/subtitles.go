package downloader

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"log/slog"

	"gemfactory/internal/translate"
)

// timedtext track URLs inside info.json expire after a few hours.
const metadataCacheTTL = time.Hour

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

	cfg := translate.ResolveConfig(ctx, s.cfg)
	res, err := ResolveSubtitleTrack(meta, requestedLang, cfg.SourcePrefRU)
	if err != nil {
		return "", err
	}

	fullVTT := filepath.Join(workbench, "fullsubs_"+videoID+"."+res.SourceLang+".vtt")
	if _, err := os.Stat(fullVTT); os.IsNotExist(err) {
		if err := s.fetchFullVTT(ctx, job, videoID, res.TrackURL, res.SourceLang, cookieFile, fullVTT); err != nil {
			return "", err
		}
	}
	if _, err := os.Stat(fullVTT); err != nil {
		return "", fmt.Errorf("no subtitles found for language %q", res.FinalLang)
	}

	trimmedPath := filepath.Join(workbench,
		clipName(videoID, job.Request, startMs, endMs)+"_trimmed.vtt")
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
		if cbs := s.callbacksOf(job); cbs != nil {
			onAttempt = cbs.OnTranslateAttempt
		}
		var prov string
		var terr error
		if job.Request.SubsNoLLM || cfg.YouTubeOnly {
			prov, terr = s.youtubeTranslatedVTT(ctx, job, res, cookieFile, startMs, endMs, trimmedPath)
		} else {
			prov, terr = s.translateVTTFile(ctx, trimmedPath, res.TargetLang, res.SourceLang, videoTitle, onAttempt)
			if terr != nil {
				s.logger.Warn("llm translation failed, falling back to youtube auto-translate",
					slog.String("video_id", videoID), slog.Any("error", terr))
				yprov, yerr := s.youtubeTranslatedVTT(ctx, job, res, cookieFile, startMs, endMs, trimmedPath)
				if yerr == nil {
					prov, terr = yprov+"-fallback", nil
				} else {
					terr = fmt.Errorf("%w; youtube fallback: %v", terr, yerr)
				}
			}
		}
		if terr == nil && prov != "" {
			s.setTranslation(job, prov)
			if cbs := s.callbacksOf(job); cbs != nil && cbs.OnTranslation != nil {
				cbs.OnTranslation(prov)
			}
		}
		if terr != nil {
			s.logger.Warn("vtt translation failed",
				slog.String("target_lang", res.TargetLang),
				slog.String("video_id", videoID),
				slog.Any("error", terr))
			return "", fmt.Errorf("translation into %s failed (%w)", res.TargetLang, terr)
		}
	}

	return trimmedPath, nil
}

// PO token state: one visitorData session and the pot bound to it.
var (
	potMu       sync.Mutex
	visitorData string
	poToken     string
)

// postJSON POSTs body as JSON within timeout and decodes a 200 response into out.
func postJSON(ctx context.Context, url string, timeout time.Duration, body, out any) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// innertubeVisitorData fetches a session visitorData via an anonymous innertube player call.
func innertubeVisitorData(ctx context.Context) string {
	potMu.Lock()
	vd := visitorData
	potMu.Unlock()
	if vd != "" {
		return vd
	}
	var out struct {
		ResponseContext struct {
			VisitorData string `json:"visitorData"`
		} `json:"responseContext"`
	}
	body := map[string]any{"context": map[string]any{"client": map[string]string{
		"clientName":    "WEB",
		"clientVersion": "2.20260918.00.00",
		"hl":            "en",
	}}}
	if postJSON(ctx, "https://www.youtube.com/youtubei/v1/player?key=AIzaSyAO_FJ2SlqU8Q4STEHLGCilw_Y9_11qcW8",
		10*time.Second, body, &out) != nil || out.ResponseContext.VisitorData == "" {
		return ""
	}
	potMu.Lock()
	visitorData = out.ResponseContext.VisitorData
	potMu.Unlock()
	return out.ResponseContext.VisitorData
}

// bgutilPOToken asks the local bgutil provider for a PO token bound to the session visitorData; a video-bound pot is rejected by timedtext (prod-verified).
func bgutilPOToken(ctx context.Context) string {
	base := strings.TrimRight(os.Getenv("BGUTIL_POT_URL"), "/")
	if base == "" {
		return ""
	}
	vd := innertubeVisitorData(ctx)
	if vd == "" {
		return ""
	}
	potMu.Lock()
	tok := poToken
	potMu.Unlock()
	if tok != "" {
		return tok
	}
	var out struct {
		PoToken string `json:"poToken"`
	}
	if postJSON(ctx, base+"/get_pot", 10*time.Second, map[string]string{"content_binding": vd}, &out) != nil || out.PoToken == "" {
		return ""
	}
	potMu.Lock()
	if visitorData == vd { // keep the pot only while its visitorData is still current
		poToken = out.PoToken
	}
	potMu.Unlock()
	return out.PoToken
}

// fetchFullVTT downloads the full subtitle track: direct timedtext first, then yt-dlp, both retried without cookies as a last resort.
func (s *Service) fetchFullVTT(ctx context.Context, job *Job, videoID, trackURL, lang, cookieFile, fullVTT string) error {
	var dlErr error
	for attempt := 1; attempt <= subtitleDownloadAttempts; attempt++ {
		dlErr = downloadSubtitlesDirect(ctx, trackURL, cookieFile, fullVTT)
		if dlErr == nil {
			return nil
		}
		// YouTube currently serves empty timedtext bodies intermittently, a repeat often works.
		s.logger.Warn("direct subtitle download attempt failed",
			slog.String("video_id", videoID), slog.Int("attempt", attempt),
			slog.Int("of", subtitleDownloadAttempts), slog.Any("error", dlErr))
		if attempt < subtitleDownloadAttempts {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(attempt) * 2 * time.Second):
			}
		}
	}

	s.logger.Warn("direct subtitle download failed, falling back to yt-dlp",
		slog.String("video_id", videoID), slog.Any("error", dlErr))
	fbErr := s.downloadSubtitlesWithRetry(ctx, job, lang, cookieFile, fullVTT)
	if fbErr == nil {
		return nil
	}

	if cookieFile == "" {
		return fmt.Errorf("failed to download subtitles: direct: %v; yt-dlp fallback: %v", dlErr, fbErr)
	}

	s.logger.Warn("subtitle download with cookies failed, retrying without cookies",
		slog.String("video_id", videoID))
	if err := downloadSubtitlesDirect(ctx, trackURL, "", fullVTT); err == nil {
		return nil
	}
	if err := s.downloadSubtitlesWithRetry(ctx, job, lang, "", fullVTT); err == nil {
		return nil
	}
	return fmt.Errorf("failed to download subtitles: direct: %v; yt-dlp fallback: %v", dlErr, fbErr)
}

// timedtextPlayerParams are client params the real web player appends at request time; they sit outside the URL signature, and without them YouTube often returns an empty body.
var timedtextPlayerParams = map[string]string{
	"xorb": "2", "xobt": "3", "xovt": "3",
	"c": "WEB", "cver": "2.20260918.00.00", "cplayer": "UNIPLAYER",
	"cbr": "Firefox", "cbrver": "152.0",
	"cos": "Windows", "cosver": "10.0", "cplatform": "DESKTOP",
}

// buildTimedtextURL appends PO token, fmt and player client params to a baseUrl; signature-covered params pass through untouched.
func buildTimedtextURL(rawURL, pot, format string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	q := u.Query()
	q.Del("fmt")
	q.Del("pot")
	q.Del("potc")
	if pot != "" {
		q.Set("pot", pot)
		q.Set("potc", "1")
	}
	q.Set("fmt", format)
	for k, v := range timedtextPlayerParams {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// isVTT reports whether the payload looks like a WebVTT document.
func isVTT(body []byte) bool {
	return strings.HasPrefix(strings.TrimSpace(string(body)), "WEBVTT") || strings.Contains(string(body), "-->")
}

// json3ToVTT converts a YouTube json3 caption payload into a WebVTT document.
func json3ToVTT(data []byte) ([]byte, error) {
	var payload struct {
		Events []struct {
			TStartMs    float64 `json:"tStartMs"`
			DDurationMs float64 `json:"dDurationMs"`
			Segs        []struct {
				UTF8 string `json:"utf8"`
			} `json:"segs"`
		} `json:"events"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("parse json3 captions: %w", err)
	}
	if len(payload.Events) == 0 {
		return nil, fmt.Errorf("json3 captions contain no events")
	}

	var cues []vttCue
	for _, ev := range payload.Events {
		var text strings.Builder
		for _, seg := range ev.Segs {
			text.WriteString(seg.UTF8)
		}
		line := strings.TrimRight(text.String(), "\n")
		if strings.TrimSpace(line) == "" {
			continue
		}
		cues = append(cues, vttCue{start: ev.TStartMs, end: ev.TStartMs + ev.DDurationMs, lines: []string{line}})
	}
	return writeVTT(cues, "en"), nil
}

// fetchTimedtext GETs a timedtext URL and returns the raw body, failing on non-200 status.
func fetchTimedtext(ctx context.Context, rawURL, cookieFile string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	if cookieFile != "" {
		if data, err := os.ReadFile(cookieFile); err == nil {
			for _, c := range ParseNetscapeCookies(string(data)) {
				req.AddCookie(c)
			}
		}
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http error: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http %d: %s", resp.StatusCode, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// invalidatePOToken drops cached PO tokens and visitorData so the next call fetches fresh ones.
func invalidatePOToken() {
	potMu.Lock()
	visitorData, poToken = "", ""
	potMu.Unlock()
}

// downloadSubtitlesDirect fetches a timedtext track (vtt first, then json3->vtt), retrying once with a fresh pot on empty bodies.
func downloadSubtitlesDirect(ctx context.Context, rawURL, cookieFile, outPath string) error {
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		lastErr = trySubtitleFetch(ctx, rawURL, bgutilPOToken(ctx), cookieFile, outPath)
		if lastErr == nil {
			return nil
		}
		invalidatePOToken()
	}
	return lastErr
}

// trySubtitleFetch tries the track as vtt, then as json3 converted to vtt.
func trySubtitleFetch(ctx context.Context, rawURL, pot, cookieFile, outPath string) error {
	vttBody, err := fetchTimedtext(ctx, buildTimedtextURL(rawURL, pot, "vtt"), cookieFile)
	if err == nil && isVTT(vttBody) {
		return os.WriteFile(outPath, vttBody, 0644)
	}
	body, jerr := fetchTimedtext(ctx, buildTimedtextURL(rawURL, pot, "json3"), cookieFile)
	vtt, cerr := json3ToVTT(body)
	switch {
	case jerr != nil && err != nil:
		return err
	case jerr != nil:
		return fmt.Errorf("invalid vtt content: %s", truncate(string(vttBody), 200))
	case cerr != nil && err != nil:
		return err
	case cerr != nil:
		return fmt.Errorf("invalid vtt content: %s (%v)", truncate(string(vttBody), 200), cerr)
	}
	return os.WriteFile(outPath, vtt, 0644)
}

// youtubeTranslatedVTT fetches YouTube's own auto-translation of the source track (tlang) and trims it to the clip.
// yt-dlp is not a fallback here: asked for the source language it would hand back untranslated subtitles.
func (s *Service) youtubeTranslatedVTT(ctx context.Context, job *Job, res *SubtitleTrackResolution, cookieFile string, startMs, endMs float64, outPath string) (string, error) {
	fullVTT := filepath.Join(s.workbenchDir(job.VideoID), "fullsubs_"+job.VideoID+"."+res.SourceLang+"-"+res.TargetLang+".vtt")
	if _, err := os.Stat(fullVTT); err != nil {
		trackURL := withTranslationLang(res.TrackURL, res.TargetLang)
		derr := downloadSubtitlesDirect(ctx, trackURL, cookieFile, fullVTT)
		if derr != nil && cookieFile != "" {
			derr = downloadSubtitlesDirect(ctx, trackURL, "", fullVTT)
		}
		if derr != nil {
			return "", fmt.Errorf("youtube auto-translate: %w", derr)
		}
	}
	// Trim beside outPath and swap in only after validation, so a failure leaves the source subtitles intact.
	tmpPath := outPath + ".yt.tmp"
	defer func() { _ = os.Remove(tmpPath) }()
	cues, err := TrimVTTFile(fullVTT, startMs, endMs, tmpPath)
	if err != nil {
		return "", fmt.Errorf("youtube auto-translate: %w", err)
	}
	if cues == 0 || !translate.TextLooksTarget(vttTextLines(tmpPath), res.TargetLang) {
		_ = os.Remove(fullVTT)
		return "", fmt.Errorf("youtube auto-translate returned no %s text", res.TargetLang)
	}
	if err := os.Rename(tmpPath, outPath); err != nil {
		return "", fmt.Errorf("youtube auto-translate: %w", err)
	}
	return "youtube/auto-translate", nil
}

// withTranslationLang asks timedtext for YouTube's machine translation of the track into lang.
func withTranslationLang(rawURL, lang string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	q := u.Query()
	q.Set("tlang", lang)
	u.RawQuery = q.Encode()
	return u.String()
}

// vttTextLines returns the cue text lines of a VTT file, skipping the header and timestamps.
func vttTextLines(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out []string
	for _, c := range parseVTT(data) {
		out = append(out, c.lines...)
	}
	return out
}

func (s *Service) translateVTTFile(ctx context.Context, vttPath, targetLang, sourceLang, videoTitle string, onAttempt func(string)) (string, error) {
	data, err := os.ReadFile(vttPath)
	if err != nil {
		return "", err
	}
	cues := parseVTT(data)
	if len(cues) == 0 {
		return "", nil
	}
	// Translate physical lines separately so multi-line cues keep their line breaks.
	var texts []string
	for _, c := range cues {
		texts = append(texts, c.lines...)
	}

	cfg := translate.ResolveConfig(ctx, s.cfg)
	provider := "gemini/" + cfg.Model
	if onAttempt != nil {
		onAttempt(provider)
	}
	translated, err := translate.Translate(ctx, cfg, texts, targetLang, sourceLang, videoTitle)
	if err != nil {
		return "", err
	}
	s.logger.Info("subtitles translated", slog.String("model", cfg.Model), slog.Int("lines", len(texts)), slog.String("target", targetLang))

	idx := 0
	for i := range cues {
		c := &cues[i]
		out := make([]string, 0, len(c.lines))
		for j := 0; j < len(c.lines) && idx < len(translated); j++ {
			t := translate.SanitizeTypography(strings.TrimSpace(translated[idx]))
			idx++
			if t == "" {
				t = c.lines[j]
			}
			out = append(out, t)
		}
		if len(out) == 0 {
			out = c.lines
		}
		c.lines = strings.Split(splitDialogueLines(strings.Join(out, "\n")), "\n")
	}
	return provider, os.WriteFile(vttPath, writeVTT(cues, targetLang), 0644)
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

// ParseNetscapeCookies reads cookies from Netscape cookies.txt content; comments, blank and short lines are skipped.
func ParseNetscapeCookies(content string) []*http.Cookie {
	var out []*http.Cookie
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 6 {
			continue
		}
		c := &http.Cookie{
			Domain:   f[0],
			HttpOnly: strings.EqualFold(f[1], "true"),
			Path:     f[2],
			Secure:   strings.EqualFold(f[3], "true"),
			Name:     f[5],
		}
		if len(f) > 6 {
			c.Value = f[6]
		}
		if exp, err := strconv.ParseFloat(f[4], 64); err == nil && exp > 0 {
			c.Expires = time.Unix(int64(exp), 0)
		}
		out = append(out, c)
	}
	return out
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
			slog.String("video_id", job.VideoID), slog.String("lang", lang),
			slog.Int("attempt", attempt), slog.Int("of", subtitleDownloadAttempts),
			slog.String("error", truncate(string(out), 500)))

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(attempt) * 15 * time.Second):
		}
	}
	return lastErr
}

// vttCue is one subtitle cue: timing in ms plus its text lines.
type vttCue struct {
	start, end float64
	lines      []string
}

// parseVTT reads WebVTT cues, tolerating YouTube quirks: CRLF, cue settings after the timing,
// numeric cue ids, styling/inline word-timing tags and whitespace-only filler lines.
// Cues with an unparsable timing or no text are dropped.
func parseVTT(data []byte) []vttCue {
	var cues []vttCue
	var cur *vttCue
	// A malformed timing line keeps its cue; the missing time is filled from the neighbours below.
	var noStart, noEnd []bool
	var curNoStart, curNoEnd bool
	flush := func() {
		if cur != nil && len(cur.lines) > 0 {
			cues = append(cues, *cur)
			noStart, noEnd = append(noStart, curNoStart), append(noEnd, curNoEnd)
		}
		cur = nil
	}
	for _, line := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		if l, r, ok := strings.Cut(line, "-->"); ok {
			flush()
			start, err1 := parseCueTime(l)
			end, err2 := parseCueTime(r)
			cur, curNoStart, curNoEnd = &vttCue{start: start, end: end}, err1 != nil, err2 != nil
			continue
		}
		if cur == nil {
			continue
		}
		if line == "" {
			flush()
			continue
		}
		line = translate.SanitizeTypography(inlineWordTimestampRe.ReplaceAllString(vttTagReplacer.Replace(line), ""))
		// Drop whitespace-only filler lines and numeric cue ids that leaked into the text.
		if strings.TrimSpace(line) == "" || len(cur.lines) == 0 && isNumericLine(line) {
			continue
		}
		cur.lines = append(cur.lines, line)
	}
	flush()
	// Missing start = previous cue's end; missing end = next cue's start, or start + defaultCueMs.
	for i := range cues {
		if noStart[i] {
			cues[i].start = 0
			if i > 0 {
				cues[i].start = cues[i-1].end
			}
		}
		if noEnd[i] {
			cues[i].end = cues[i].start + defaultCueMs
			if i+1 < len(cues) && !noStart[i+1] && cues[i+1].start > cues[i].start {
				cues[i].end = cues[i+1].start
			}
		}
		if (noStart[i] || noEnd[i]) && cues[i].end <= cues[i].start {
			cues[i].end = cues[i].start + defaultCueMs
		}
	}
	return cues
}

// defaultCueMs is the display time given to a cue whose end cannot be recovered.
const defaultCueMs = 2000

// parseCueTime reads the timestamp at the start of one side of a timing line ("00:00:01.000 align:start"), accepting SRT-style commas.
func parseCueTime(s string) (float64, error) {
	f := strings.Fields(s)
	if len(f) == 0 {
		return 0, fmt.Errorf("missing cue time")
	}
	return ParseTimecode(strings.ReplaceAll(f[0], ",", "."))
}

// writeVTT renders cues as a WebVTT document with the canonical header.
func writeVTT(cues []vttCue, lang string) []byte {
	var b strings.Builder
	b.WriteString("WEBVTT\nKind: captions\nLanguage: " + lang + "\n\n")
	for _, c := range cues {
		b.WriteString(FormatTimecode(c.start) + " --> " + FormatTimecode(c.end) + "\n")
		b.WriteString(strings.Join(c.lines, "\n") + "\n\n")
	}
	return []byte(b.String())
}

// TrimVTTFile keeps cues overlapping [startMs, endMs), shifts their timestamps relative to startMs and writes the result to outPath. Returns the number of cues kept.
func TrimVTTFile(inPath string, startMs, endMs float64, outPath string) (int, error) {
	data, err := os.ReadFile(inPath)
	if err != nil {
		return 0, fmt.Errorf("read subtitles: %w", err)
	}
	var kept []vttCue
	for _, c := range parseVTT(data) {
		if c.start >= endMs {
			break
		}
		if c.end <= startMs {
			continue
		}
		c.start, c.end = max(c.start-startMs, 0), c.end-startMs
		kept = append(kept, c)
	}
	kept = dedupeRollingCues(kept)
	if err := os.WriteFile(outPath, writeVTT(kept, "en"), 0644); err != nil {
		return 0, fmt.Errorf("write trimmed subtitles: %w", err)
	}
	return len(kept), nil
}

// dedupeRollingCues collapses the echo repeats YouTube auto-captions carry from cue to cue.
func dedupeRollingCues(cues []vttCue) []vttCue {
	var prevLower []string
	out := make([]vttCue, 0, len(cues))
	for _, c := range cues {
		raw := strings.Join(c.lines, " ")
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
		c.lines = []string{strings.Join(words[skip:], " ")}
		out = append(out, c)
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

// maxTitleRunes caps the caption title so long page dumps never reach the user.
const maxTitleRunes = 200

// FormatCaption builds a clean, minimalist HTML caption (Title + up to 5 hashtags).
func FormatCaption(title string, tags []string) string {
	if title == "" {
		return ""
	}
	// Cut before escaping so "</b>" never gets truncated.
	if runes := []rune(title); len(runes) > maxTitleRunes {
		title = string(runes[:maxTitleRunes-1]) + "…"
	}
	caption := "<b>" + html.EscapeString(title) + "</b>"
	if len(tags) > 0 {
		caption += "\n\n" + strings.Join(tags, " ")
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

func (s *Service) ExtractMetadata(ctx context.Context, url, videoID, cookieFile string) (*SourceMeta, error) {
	metaPath := filepath.Join(s.workbenchDir(videoID), videoID+".info.json")
	if info, err := os.Stat(metaPath); err == nil && time.Since(info.ModTime()) < metadataCacheTTL {
		s.logger.Info("metadata cache hit", slog.String("video_id", videoID))
		return readSourceMeta(metaPath)
	}

	bin, err := YTDLPBinary()
	if err != nil {
		return nil, err
	}

	s.logger.Info("metadata extraction starting", slog.String("url", url), slog.String("video_id", videoID))
	start := time.Now()
	defer func() {
		s.logger.Info("metadata extraction finished",
			slog.String("url", url),
			slog.String("video_id", videoID),
			slog.Duration("elapsed", time.Since(start)))
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
