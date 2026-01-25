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

	// Clean and prepare names
	album := cleanReleaseString(release.AlbumName)
	track := cleanReleaseString(release.TitleTrack)

	// Filter TBA
	if isTBA(track) {
		track = ""
	}
	if isTBA(album) {
		album = ""
	}

	// Deduplicate: if names match, keep only one
	if strings.EqualFold(album, track) {
		track = ""
	}

	// Build string
	dateStr := release.Date.Format("02.01")
	line := fmt.Sprintf("%s | %s", dateStr, artist)

	if album != "" && album != "N/A" {
		line += " | " + escapeHTML(album)
	}
	if track != "" && trackNameIsValid(track) {
		line += " | " + escapeHTML(track)
	}

	// Add only YT / SP links
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

	// Strip common release prefixes and split by various dashes
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == '–' || r == '—' || (r == '-' && strings.Contains(s, " - "))
	})

	result := s
	if len(parts) > 1 {
		// If separator exists, take second part (usually title)
		result = strings.Join(parts[1:], " ")
	}

	// Clean quotes and extra spaces
	result = strings.Trim(result, " \"“»”«")
	result = strings.ReplaceAll(result, "Title Track:", "")
	result = strings.TrimSpace(result)

	return result
}

func isTBA(s string) bool {
	low := strings.ToLower(s)
	return strings.Contains(low, "to be announced") || strings.Contains(low, "tba")
}

func trackNameIsValid(s string) bool {
	return s != "" && s != "N/A" && !isTBA(s)
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
