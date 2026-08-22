package downloader

import (
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
)

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

// videoIDFromURL extracts a video ID from YouTube, TikTok, or generic URLs.
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

// IsTikTokURL reports whether the URL points to TikTok.
func IsTikTokURL(raw string) bool {
	return strings.Contains(raw, "tiktok.com")
}

// IsShortsURL reports whether the URL points to a YouTube Short.
func IsShortsURL(raw string) bool {
	return strings.Contains(raw, "/shorts/")
}

// IsDirectDownloadURL reports whether the URL is a short video (TikTok / Shorts)
// suitable for instant 1-click download without timecode parameters.
func IsDirectDownloadURL(raw string) bool {
	return IsTikTokURL(raw) || IsShortsURL(raw)
}

// ExtractFirstURL finds the first http/https URL in text.
func ExtractFirstURL(text string) string {
	for _, field := range strings.Fields(text) {
		if strings.HasPrefix(field, "http://") || strings.HasPrefix(field, "https://") {
			// Strip trailing punctuation like ), ], ., etc.
			return strings.TrimRight(field, ".,)!?]>")
		}
	}
	return ""
}

// authArgs returns proxy/cookie flags shared by every yt-dlp invocation.
func (s *Service) authArgs(cookieFile string) []string {
	var args []string
	if cookieFile != "" {
		args = append(args, "--cookies", cookieFile)
	}
	if proxy := s.proxy(); proxy != "" {
		args = append(args, "--proxy", proxy)
	}
	return args
}

// proxy reads the optional downloader proxy from the environment.
func (s *Service) proxy() string { return os.Getenv("YTDLP_PROXY") }
