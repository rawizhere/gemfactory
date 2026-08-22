package downloader

import (
	"fmt"
	"regexp"
	"strings"
)

// timecodePattern matches "SS(.ms)", "MM:SS(.ms)" and "HH:MM:SS(.ms)" tokens.
var timecodePattern = regexp.MustCompile(`\b(?:\d{1,2}:)?(?:\d{1,2}:)?\d+(?:\.\d+)?\b`)

// langPattern matches BCP-47-ish language tags like en, ru, en-US, ko.
var langPattern = regexp.MustCompile(`^[a-z]{2,3}(?:-[A-Za-z]{2,4})?$`)

// commandFlags are stripped from arguments before interval extraction.
var commandFlags = map[string]bool{"hq": true, "meta": true}

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
	HQ        bool
	Meta      bool
	GIF       bool
	Shorts    bool
	AudioOnly bool
}

// ParseClipArgs parses telegram command arguments (without the command itself).
//
// The first argument is the URL; remaining tokens are flags ("hq", "meta"),
// an optional language tag and any number of start/end timecode pairs:
//
//	/subs <url> 4:00 4:01 4:02 4:04 ru -> two intervals with ru subtitles.
func ParseClipArgs(args []string) (*ParsedCommand, error) {
	var (
		url      string
		lang     string
		hq, meta bool
		rest     []string
	)

	for _, arg := range args {
		switch {
		case strings.Contains(arg, "http"):
			if url == "" {
				url = strings.TrimSpace(arg)
			}
			// Extra URL-like tokens (e.g. Telegram link previews) are ignored.
		case commandFlags[arg]:
			if arg == "hq" {
				hq = true
			} else {
				meta = true
			}
		case lang == "" && langPattern.MatchString(arg):
			lang = arg
		default:
			rest = append(rest, arg)
		}
	}

	if url == "" {
		return nil, fmt.Errorf("не найдена ссылка на видео")
	}
	if lang != "" && !langPattern.MatchString(lang) {
		return nil, fmt.Errorf("некорректный язык субтитров %q", lang)
	}

	shorts := IsDirectDownloadURL(url)
	matches := timecodePattern.FindAllString(strings.Join(rest, " "), -1)

	// Shorts and TikTok videos do not require timecodes (download whole video).
	if len(matches) == 0 && shorts {
		return &ParsedCommand{
			URL:       url,
			Intervals: []TimeInterval{{Start: "", End: ""}},
			SubsLang:  lang,
			HQ:        hq,
			Meta:      meta,
			Shorts:    true,
		}, nil
	}

	if len(matches) == 0 || len(matches)%2 != 0 {
		return nil, fmt.Errorf("не найдено временных интервалов, задайте пары start/end, например: 0:26 0:32")
	}

	intervals := make([]TimeInterval, 0, len(matches)/2)
	for i := 0; i < len(matches); i += 2 {
		startMs, err := ParseTimecode(matches[i])
		if err != nil {
			return nil, fmt.Errorf("некорректный таймкод %q: %w", matches[i], err)
		}
		endMs, err := ParseTimecode(matches[i+1])
		if err != nil {
			return nil, fmt.Errorf("некорректный таймкод %q: %w", matches[i+1], err)
		}
		if endMs <= startMs {
			return nil, fmt.Errorf("некорректный интервал %s-%s: конец должен быть позже начала", matches[i], matches[i+1])
		}
		intervals = append(intervals, TimeInterval{Start: matches[i], End: matches[i+1]})
	}

	return &ParsedCommand{
		URL:       url,
		Intervals: intervals,
		SubsLang:  lang,
		HQ:        hq,
		Meta:      meta,
		Shorts:    shorts,
	}, nil
}
