package downloader

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/text/language"

	"gemfactory/internal/settings"
)

// timecodePattern matches "SS(.ms)", "MM:SS(.ms)" and "HH:MM:SS(.ms)" tokens.
var timecodePattern = regexp.MustCompile(`\b(?:\d{1,2}:)?(?:\d{1,2}:)?\d+(?:\.\d+)?\b`)

// langPattern matches BCP-47-ish language tags like en, ru, en-US, ko.
// IsValidLangTag reports whether s is a well-formed BCP-47 language tag.
func IsValidLangTag(s string) bool {
	tag, err := language.Parse(s)
	return err == nil && !tag.IsRoot()
}

// commandFlags are stripped from arguments before interval extraction.
var commandFlags = map[string]bool{"hq": true, "meta": true, "nollm": true}

// TimeInterval is a raw start/end timecode pair as provided by the user.
type TimeInterval struct {
	Start string
	End   string
}

// ParsedCommand is the result of parsing a clip-style Telegram command.
type ParsedCommand struct {
	URL       string
	Intervals []TimeInterval
	SubsLang  string // empty when not requested
	Quality   string // e.g. "720p"
	HQ        bool
	Meta      bool
	NoLLM     bool // translate subs via Google Translate only
	GIF       bool
	Shorts    bool
	AudioOnly bool
}

// ParseClipArgs parses telegram command arguments: URL, flags, optional language tag and timecode pairs.
func ParseClipArgs(args []string) (*ParsedCommand, error) {
	var (
		url             string
		lang            string
		quality         string
		hq, meta, noLLM bool
		rest            []string
	)

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case strings.Contains(arg, "http"):
			if url == "" {
				url = strings.TrimSpace(arg)
			}
			// Extra URL-like tokens (e.g. Telegram link previews) are ignored.
		case arg == "-q" || arg == "-quality":
			if i+1 < len(args) {
				next := strings.TrimSpace(args[i+1])
				if next == "720" || next == "720p" || next == "1080" || next == "1080p" {
					quality = next
					i++
					continue
				}
			}
			rest = append(rest, arg)
		case arg == "720p" || arg == "-720p" || arg == "720" || arg == "-720":
			quality = "720p"
		case commandFlags[arg]:
			switch arg {
			case "hq":
				hq = true
			case "meta":
				meta = true
			case "nollm":
				noLLM = true
			}
		case lang == "" && IsValidLangTag(arg):
			lang = arg
		default:
			rest = append(rest, arg)
		}
	}

	if url == "" {
		return nil, fmt.Errorf("no video URL found")
	}
	if lang != "" && !IsValidLangTag(lang) {
		return nil, fmt.Errorf("invalid subtitle language %q", lang)
	}

	shorts := IsDirectDownloadURL(url)
	matches := timecodePattern.FindAllString(strings.Join(rest, " "), -1)

	// Shorts and TikTok videos do not require timecodes (download whole video).
	if len(matches) == 0 && shorts {
		return &ParsedCommand{
			URL:       url,
			Intervals: []TimeInterval{{Start: "", End: ""}},
			SubsLang:  lang,
			Quality:   quality,
			HQ:        hq,
			Meta:      meta,
			NoLLM:     noLLM,
			Shorts:    true,
		}, nil
	}

	if len(matches) == 0 || len(matches)%2 != 0 {
		return nil, fmt.Errorf("no time intervals found; provide start/end pairs, e.g. 0:26 0:32")
	}

	intervals := make([]TimeInterval, 0, len(matches)/2)
	for i := 0; i < len(matches); i += 2 {
		startMs, err := ParseTimecode(matches[i])
		if err != nil {
			return nil, fmt.Errorf("invalid timecode %q: %w", matches[i], err)
		}
		endMs, err := ParseTimecode(matches[i+1])
		if err != nil {
			return nil, fmt.Errorf("invalid timecode %q: %w", matches[i+1], err)
		}
		if endMs <= startMs {
			return nil, fmt.Errorf("invalid interval %s-%s: end time must be after start time", matches[i], matches[i+1])
		}
		intervals = append(intervals, TimeInterval{Start: matches[i], End: matches[i+1]})
	}

	return &ParsedCommand{
		URL:       url,
		Intervals: intervals,
		SubsLang:  lang,
		Quality:   quality,
		HQ:        hq,
		Meta:      meta,
		NoLLM:     noLLM,
		Shorts:    false,
	}, nil
}

func ParseTimecode(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty timecode")
	}

	parts := strings.Split(s, ":")
	if len(parts) > 3 {
		return 0, fmt.Errorf("invalid timecode %q", s)
	}

	values := make([]float64, len(parts))
	for i, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			return 0, fmt.Errorf("invalid timecode %q: empty component", s)
		}
		v, err := strconv.ParseFloat(p, 64)
		if err != nil || v < 0 {
			return 0, fmt.Errorf("invalid timecode component %q in %q", p, s)
		}
		values[i] = v
	}

	var ms float64
	switch len(values) {
	case 1: // seconds (possibly fractional)
		ms = values[0] * 1000
	case 2: // MM:SS(.mmm)
		ms = values[0]*60*1000 + values[1]*1000
	default: // HH:MM:SS(.mmm)
		ms = values[0]*3600*1000 + values[1]*60*1000 + values[2]*1000
	}

	return ms, nil
}

func FormatTimecode(ms float64) string {
	total := int64(ms)
	h := total / (1000 * 60 * 60)
	m := (total % (1000 * 60 * 60)) / (1000 * 60)
	s := (total % (1000 * 60)) / 1000
	millis := total % 1000
	return fmt.Sprintf("%02d:%02d:%02d.%03d", h, m, s, millis)
}

var (
	youtubeIDPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?:youtube\.com/(?:watch\?(?:.*&)?v=|embed/|shorts/|live/))([A-Za-z0-9_-]{11})`),
		regexp.MustCompile(`youtu\.be/([A-Za-z0-9_-]{11})`),
	}
	tiktokIDPatterns = []*regexp.Regexp{
		regexp.MustCompile(`tiktok\.com/@[\w.-]+/video/(\d+)`),
		regexp.MustCompile(`(?:vm|vt)\.tiktok\.com/([A-Za-z0-9_-]+)`),
		regexp.MustCompile(`tiktok\.com/t/([A-Za-z0-9_-]+)`),
		regexp.MustCompile(`tiktok\.com/v/(\d+)`),
	}
)

func videoIDFromURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		raw = u.String()
	}
	for _, re := range youtubeIDPatterns {
		if m := re.FindStringSubmatch(raw); len(m) > 1 {
			return m[1], nil
		}
	}
	for _, re := range tiktokIDPatterns {
		if m := re.FindStringSubmatch(raw); len(m) > 1 {
			return "tt_" + m[1], nil
		}
	}
	if matched, _ := regexp.MatchString(`^[A-Za-z0-9_-]{11}$`, raw); matched {
		return raw, nil
	}
	return "", fmt.Errorf("cannot extract video id from %q", raw)
}

func IsTikTokURL(raw string) bool {
	return strings.Contains(raw, "tiktok.com")
}

func IsShortsURL(raw string) bool {
	return strings.Contains(raw, "/shorts/")
}

func IsDirectDownloadURL(raw string) bool {
	return IsTikTokURL(raw) || IsShortsURL(raw)
}

func ExtractFirstURL(text string) string {
	for _, field := range strings.Fields(text) {
		if strings.HasPrefix(field, "http://") || strings.HasPrefix(field, "https://") {
			return strings.TrimRight(field, ".,)!?]>")
		}
	}
	return ""
}

func (s *Service) authArgs(ctx context.Context, cookieFile string) []string {
	var args []string
	if cookieFile != "" {
		args = append(args, "--cookies", cookieFile)
	}
	if proxy := settings.New(s.configs).Value(ctx, "YTDLP_PROXY", ""); proxy != "" {
		args = append(args, "--proxy", proxy)
	}
	return args
}
