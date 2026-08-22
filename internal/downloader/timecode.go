// Package downloader implements yt-dlp based video clip extraction with
// optional hardsubbed subtitles.
package downloader

import (
	"fmt"
	"strconv"
	"strings"
)

// TimecodeParser parses and formats video timecodes.
type TimecodeParser struct{}

// ParseTimecode parses a "SS.mmm", "MM:SS", "MM:SS.mmm", "HH:MM:SS" or
// "HH:MM:SS.mmm" string into milliseconds.
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

// FormatTimecode formats milliseconds as HH:MM:SS.mmm.
func FormatTimecode(ms float64) string {
	total := int64(ms)
	h := total / (1000 * 60 * 60)
	m := (total % (1000 * 60 * 60)) / (1000 * 60)
	s := (total % (1000 * 60)) / 1000
	millis := total % 1000
	return fmt.Sprintf("%02d:%02d:%02d.%03d", h, m, s, millis)
}
