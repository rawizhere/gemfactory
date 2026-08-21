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
		artistName = release.Artist.Name.String()
	}
	artist := "<b>" + escapeHTML(artistName) + "</b>"

	album := cleanReleaseString(release.AlbumName.String())
	title := cleanReleaseString(release.Title.String())
	track := cleanReleaseString(release.TitleTrack.String())

	// Determine what is the main 'event' text
	var mainEvent string
	isPre := strings.Contains(strings.ToLower(release.Title.String()), "pre-release") ||
		strings.Contains(strings.ToLower(release.AlbumName.String()), "pre-release")

	if isPre {
		if title != "" && strings.Contains(strings.ToLower(title), "pre-release") {
			mainEvent = title
		} else {
			mainEvent = "Pre-release"
			if album != "" {
				mainEvent += " | " + album
			}
		}
	} else {
		mainEvent = album
		if mainEvent == "" {
			mainEvent = title
		}
	}

	lowEvent := strings.ToLower(mainEvent)
	if lowEvent == "release date" || lowEvent == "album release" || lowEvent == "offline release" {
		mainEvent = album
	}

	secondary := ""
	if track != "" && !isTBA(track) && !strings.EqualFold(track, mainEvent) && !strings.EqualFold(track, album) {
		secondary = track
	}

	dateStr := release.Date.Format("02.01")
	line := fmt.Sprintf("%s | %s | %s", dateStr, artist, escapeHTML(mainEvent))

	if secondary != "" {
		line += " | " + escapeHTML(secondary)
	}

	var links []string
	if release.MV.String() != "" && release.MV.String() != "N/A" {
		links = append(links, fmt.Sprintf("<a href=\"%s\">YT</a>", release.MV.String()))
	}
	if release.Spotify.String() != "" && release.Spotify.String() != "N/A" {
		links = append(links, fmt.Sprintf("<a href=\"%s\">SP</a>", release.Spotify.String()))
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

	low := strings.ToLower(s)
	if strings.HasPrefix(low, "pre-release") {
		sub := strings.TrimSpace(s[len("pre-release"):])
		sub = strings.Trim(sub, " -–—:\"“")
		if sub != "" {
			return "Pre-release “" + sub + "”"
		}
		return "Pre-release"
	}

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

	var artistName string
	if release.Artist != nil {
		artistName = release.Artist.Name.String()
	}
	parts = append(parts, fmt.Sprintf("🎤 %s", artistName))

	title := release.Title.String()
	if release.AlbumName.String() != "" && release.AlbumName.String() != "N/A" {
		title = release.AlbumName.String()
	}
	parts = append(parts, fmt.Sprintf("💿 %s", title))

	if release.TitleTrack.String() != "" && release.TitleTrack.String() != "N/A" {
		parts = append(parts, fmt.Sprintf("🎵 %s", release.TitleTrack.String()))
	}

	dateStr := release.Date.Format("02.01.06")
	parts = append(parts, fmt.Sprintf("📅 %s", dateStr))

	if release.MV.String() != "" && release.MV.String() != "N/A" {
		parts = append(parts, fmt.Sprintf("🎬 [MV](%s)", release.MV.String()))
	}

	if release.Spotify.String() != "" && release.Spotify.String() != "N/A" {
		parts = append(parts, fmt.Sprintf("🎧 [Spotify](%s)", release.Spotify.String()))
	}

	return strings.Join(parts, "\n")
}
