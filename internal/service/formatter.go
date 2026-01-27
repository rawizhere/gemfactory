package service

import (
	"fmt"
	"gemfactory/internal/model"
	"html"
	"strings"
)

// escapeHTML is a helper to sanitize strings for Telegram's HTML styling.
func escapeHTML(s string) string {
	return html.EscapeString(s)
}

// FormatReleaseForTelegram generates a concise HTML-formatted line for Telegram messages.
func FormatReleaseForTelegram(release *model.Release) string {
	var artistName string
	if release.Artist != nil {
		artistName = release.Artist.Name
	}
	artist := "<b>" + escapeHTML(artistName) + "</b>"

	album := cleanReleaseString(release.AlbumName)
	title := cleanReleaseString(release.Title)
	track := cleanReleaseString(release.TitleTrack)

	// Determine what is the main 'event' text
	var mainEvent string
	isPre := strings.Contains(strings.ToLower(release.Title), "pre-release") ||
		strings.Contains(strings.ToLower(release.AlbumName), "pre-release")

	if isPre {
		// For pre-releases, prioritize showing the specific pre-release title
		if title != "" && strings.Contains(strings.ToLower(title), "pre-release") {
			mainEvent = title
		} else {
			mainEvent = "Pre-release"
			if album != "" {
				mainEvent += " | " + album
			}
		}
	} else {
		// For main releases, use the album name
		mainEvent = album
		if mainEvent == "" {
			mainEvent = title
		}
	}

	// Filter out technical placeholders
	lowEvent := strings.ToLower(mainEvent)
	if lowEvent == "release date" || lowEvent == "album release" || lowEvent == "offline release" {
		mainEvent = album
	}

	// Add Title Track if it's unique and present
	secondary := ""
	if track != "" && !isTBA(track) && !strings.EqualFold(track, mainEvent) && !strings.EqualFold(track, album) {
		secondary = track
	}

	// Build the final string
	dateStr := release.Date.Format("02.01")
	line := fmt.Sprintf("%s | %s | %s", dateStr, artist, escapeHTML(mainEvent))

	if secondary != "" {
		line += " | " + escapeHTML(secondary)
	}

	// Links
	var links []string
	if release.MV != "" && release.MV != "N/A" {
		links = append(links, fmt.Sprintf("<a href=\"%s\">YT</a>", release.MV))
	}
	if release.Spotify != "" && release.Spotify != "N/A" {
		links = append(links, fmt.Sprintf("<a href=\"%s\">SP</a>", release.Spotify))
	}

	if len(links) > 0 {
		line += " | " + strings.Join(links, " / ")
	}

	return line
}

func cleanReleaseString(s string) string {
	if s == "" || s == "N/A" {
		return ""
	}

	s = strings.TrimSpace(s)

	// Only unwrap if string starts and ends with matching quotes/brackets
	for {
		changed := false
		pairs := [][2]string{{"“", "”"}, {"\"", "\""}, {"'", "'"}, {"[", "]"}, {"(", ")"}, {"{", "}"}}
		for _, p := range pairs {
			if strings.HasPrefix(s, p[0]) && strings.HasSuffix(s, p[1]) {
				s = strings.TrimSpace(s[len(p[0]) : len(s)-len(p[1])])
				changed = true
			}
		}
		if !changed {
			break
		}
	}

	// Check for "Pre-release" specifically
	low := strings.ToLower(s)
	if strings.HasPrefix(low, "pre-release") {
		sub := strings.TrimSpace(s[len("pre-release"):])
		sub = strings.Trim(sub, " -–—:\"“")
		if sub != "" {
			return "Pre-release “" + sub + "”"
		}
		return "Pre-release"
	}

	// Handle standard "Prefix – Name" format
	separators := []string{" – ", " — ", " - "}
	for _, sep := range separators {
		if parts := strings.SplitN(s, sep, 2); len(parts) == 2 {
			p1 := strings.ToLower(parts[0])
			if strings.Contains(p1, "album") || strings.Contains(p1, "single") ||
				strings.Contains(p1, "ep") || strings.Contains(p1, "digital") {
				return strings.TrimSpace(parts[1])
			}
		}
	}

	return s
}

func isTBA(s string) bool {
	low := strings.ToLower(s)
	return strings.Contains(low, "to be announced") || strings.Contains(low, "tba")
}

// FormatReleaseForDisplay generates a detailed multi-line string for descriptive release views.
func FormatReleaseForDisplay(release *model.Release) string {
	var parts []string

	// Artist
	var artistName string
	if release.Artist != nil {
		artistName = release.Artist.Name
	}
	parts = append(parts, fmt.Sprintf("🎤 %s", artistName))

	// Title
	title := release.Title
	if release.AlbumName != "" && release.AlbumName != "N/A" {
		title = release.AlbumName
	}
	parts = append(parts, fmt.Sprintf("💿 %s", title))

	// Title Track
	if release.TitleTrack != "" && release.TitleTrack != "N/A" {
		parts = append(parts, fmt.Sprintf("🎵 %s", release.TitleTrack))
	}

	// Date and time
	dateStr := release.Date.Format("02.01.06")
	parts = append(parts, fmt.Sprintf("📅 %s", dateStr))

	// MV
	if release.MV != "" && release.MV != "N/A" {
		parts = append(parts, fmt.Sprintf("🎬 [MV](%s)", release.MV))
	}

	return strings.Join(parts, "\n")
}
