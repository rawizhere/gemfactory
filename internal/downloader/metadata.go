package downloader

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

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

	// If Title is truncated with ellipsis (common in TikTok), use first line of Description if matching.
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

	// Append up to 5 hashtags from meta.Tags if they are not already in title.
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

// ExtractMetadata downloads info.json for url into workDir (cached) and
// returns the parsed metadata.
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

// ResolveSubsLanguage picks the best available subtitle language code.
//
// Priority mirrors the reference implementation: exact match in manual
// subtitles, then automatic captions; then regional-tag matches. When the
// requested track is missing entirely, an auto-translation form "<lang>-en"
// is derived from the available English track.
func ResolveSubsLanguage(meta *SourceMeta, requested string) (string, bool) {
	requested = normalizeLang(requested)
	subs := orEmptyMap(meta.Subtitles)
	auto := orEmptyMap(meta.AutomaticCaptions)

	if lang, ok := findLanguage(subs, requested); ok {
		return lang, true
	}
	if lang, ok := findLanguage(auto, requested); ok {
		return lang, true
	}

	enTrack, enFound := firstLanguage(subs, "en")
	if !enFound {
		enTrack, enFound = firstLanguage(auto, "en")
	}

	base := baseLanguage(requested)
	if base == "en" {
		// en-US must never become the invalid auto-translation form en-US-en.
		if enFound {
			return enTrack, true
		}
		return "", false
	}

	if enFound {
		return requested + "-" + enTrack, true
	}
	return requested + "-en", true
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

// normalizeLang maps common aliases to canonical BCP-47-ish codes.
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

// baseLanguage strips regional tags: en-US -> en.
func baseLanguage(lang string) string {
	if i := strings.Index(lang, "-"); i > 0 {
		return strings.ToLower(lang[:i])
	}
	return strings.ToLower(lang)
}

func orEmptyMap(m map[string][]SubtitleTrack) map[string][]SubtitleTrack {
	if m == nil {
		return map[string][]SubtitleTrack{}
	}
	return m
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "...(truncated)"
}
