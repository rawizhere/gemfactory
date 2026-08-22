package service

import (
	"gemfactory/internal/model"
	"strings"
	"testing"
	"time"
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

	if !strings.Contains(result, "<b>TWICE</b>") {
		t.Errorf("Expected result to contain artist name, got %s", result)
	}
	if !strings.Contains(result, "With YOU-th") {
		t.Errorf("Expected result to contain album name, got %s", result)
	}
	if !strings.Contains(result, "ONE SPARK") {
		t.Errorf("Expected result to contain title track, got %s", result)
	}
	if !strings.Contains(result, "<a href=\"https://youtube.com/watch?v=123\">YT</a>") {
		t.Errorf("Expected result to contain YT link, got %s", result)
	}
	if !strings.Contains(result, "<a href=\"https://open.spotify.com/track/456\">SP</a>") {
		t.Errorf("Expected result to contain SP link, got %s", result)
	}

	collabRelease := &model.Release{
		Artist: &model.Artist{
			Name: model.NewUniqueString("JEON SOMI"),
		},
		DisplayArtist: model.NewUniqueString("JVKE x JEON SOMI"),
		AlbumName:     model.NewUniqueString("Collab Single"),
		Date:          time.Date(2026, 2, 23, 0, 0, 0, 0, time.UTC),
	}
	collabResult := FormatReleaseForTelegram(collabRelease)
	if !strings.Contains(collabResult, "<b>JVKE x JEON SOMI</b>") {
		t.Errorf("Expected collab display artist name, got %s", collabResult)
	}
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
		actual := cleanReleaseString(tt.input)
		if actual != tt.expected {
			t.Errorf("cleanReleaseString(%q) = %q, expected %q", tt.input, actual, tt.expected)
		}
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
		got := CleanReleaseTitle(tt.input)
		if got != tt.want {
			t.Errorf("CleanReleaseTitle(%q) = %q, want %q", tt.input, got, tt.want)
		}
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
		got := CleanLink(tt.input)
		if got != tt.want {
			t.Errorf("CleanLink(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
