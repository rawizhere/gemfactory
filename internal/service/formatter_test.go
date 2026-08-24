package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"gemfactory/internal/model"
)

func TestFormatReleaseForTelegram(t *testing.T) {
	release := &model.Release{
		Artist: &model.Artist{
			Name: model.NewUniqueString("TWICE"),
		},
		AlbumName:  model.NewUniqueString("With YOU-th"),
		TitleTrack: model.NewUniqueString("ONE SPARK"),
		MV:         model.NewUniqueString("https://youtube.com/watch?v=123"),
		Spotify:    model.NewUniqueString("https://open.spotify.com/track/456"),
		Date:       time.Date(2026, 2, 23, 0, 0, 0, 0, time.UTC),
	}

	result := FormatReleaseForTelegram(release)

	require.Contains(t, result, "<b>TWICE</b>", "expected artist name, got %s", result)
	require.Contains(t, result, "With YOU-th", "expected album name, got %s", result)
	require.Contains(t, result, "ONE SPARK", "expected title track, got %s", result)
	require.Contains(t, result, `<a href="https://youtube.com/watch?v=123">YT</a>`, "expected YT link, got %s", result)
	require.Contains(t, result, `<a href="https://open.spotify.com/track/456">SP</a>`, "expected SP link, got %s", result)

	collabRelease := &model.Release{
		Artist: &model.Artist{
			Name: model.NewUniqueString("JEON SOMI"),
		},
		DisplayArtist: model.NewUniqueString("JVKE x JEON SOMI"),
		AlbumName:     model.NewUniqueString("Collab Single"),
		Date:          time.Date(2026, 2, 23, 0, 0, 0, 0, time.UTC),
	}
	collabResult := FormatReleaseForTelegram(collabRelease)
	require.Contains(t, collabResult, "<b>JVKE x JEON SOMI</b>", "expected collab display artist name, got %s", collabResult)
}

func TestCleanReleaseString(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"“Title”", "Title"},
		{"\"Title\"", "Title"},
		{"1st Single – Song", "Song"},
		{"N/A", ""},
		{"", ""},
	}

	for _, tt := range tests {
		require.Equal(t, tt.expected, cleanReleaseString(tt.input), "cleanReleaseString(%q)", tt.input)
	}
}

func TestCleanReleaseTitle(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"{Title with braces}", "Title with braces"},
		{"   Spaced   Title   ", "Spaced Title"},
		{"Clean Title", "Clean Title"},
	}

	for _, tt := range tests {
		require.Equal(t, tt.want, CleanReleaseTitle(tt.input), "CleanReleaseTitle(%q)", tt.input)
	}
}

func TestCleanLink(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"https://youtube.com/watch?v=123", "https://youtube.com/watch?v=123"},
		{"https://youtube.com/@channelName", ""},
		{"https://youtube.com/channel/UC12345", ""},
		{"https://youtube.com/user/userName", ""},
		{"https://youtube.com/c/customName", ""},
		{"", ""},
	}

	for _, tt := range tests {
		require.Equal(t, tt.want, CleanLink(tt.input), "CleanLink(%q)", tt.input)
	}
}
