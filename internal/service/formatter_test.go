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
