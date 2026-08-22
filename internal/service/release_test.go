package service

import (
	"context"
	"gemfactory/internal/model"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestMonthToInt(t *testing.T) {
	if monthToInt("March") != 3 || monthToInt("DECEMBER") != 12 || monthToInt("notamonth") != 0 {
		t.Error("unexpected monthToInt results")
	}
}

func TestValidateRelease(t *testing.T) {
	valid := &model.Release{ArtistID: 1, Title: model.NewUniqueString("X"), Date: time.Now()}
	if err := NewReleaseService(nil, nil, nil).validateRelease(valid); err != nil {
		t.Errorf("expected valid release, got %v", err)
	}

	cases := []*model.Release{
		nil,
		{Title: model.NewUniqueString("X"), Date: time.Now()},
		{ArtistID: 1, Date: time.Now()},
		{ArtistID: 1, Title: model.NewUniqueString("  ")},
	}
	svc := &ReleaseService{}
	for i, c := range cases {
		if err := svc.validateRelease(c); err == nil {
			t.Errorf("case %d: expected validation error", i)
		}
	}
}

func TestCleanReleaseTitleAndLink(t *testing.T) {
	if got := CleanReleaseTitle("  {Multi  word \n title }"); got != "Multi word title" {
		t.Errorf("CleanReleaseTitle = %q", got)
	}

	for _, link := range []string{
		"https://youtube.com/@channel",
		"https://www.youtube.com/channel/abc",
		"https://youtube.com/user/name",
		"https://youtube.com/c/name",
	} {
		if got := CleanLink(link); got != "" {
			t.Errorf("CleanLink(%q) should be empty, got %q", link, got)
		}
	}
	if got := CleanLink("https://youtu.be/abc123"); got != "https://youtu.be/abc123" {
		t.Errorf("CleanLink dropped valid link: %q", got)
	}
	if CleanLink("") != "" {
		t.Error("CleanLink(\"\") should stay empty")
	}
}

func TestFindArtist(t *testing.T) {
	aespa := &model.Artist{ArtistID: 1, Name: model.NewUniqueString("aespa")}
	blackpink := &model.Artist{ArtistID: 2, Name: model.NewUniqueString("BLACKPINK")}
	artists := map[string]*model.Artist{
		"aespa":     aespa,
		"blackpink": blackpink,
	}

	tests := []struct {
		in   string
		want *model.Artist
	}{
		{"Aespa", aespa},
		{"  AESPA  ", aespa},
		{"aespa (Karina)", aespa},
		{"aespa - Drama", aespa},
		{"BLACKPINK & Rosé", blackpink},
		{"Unknown Artist", nil},
	}
	for _, tc := range tests {
		got := (&ReleaseService{}).findArtist(tc.in, artists)
		if got != tc.want {
			t.Errorf("findArtist(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestGetReleasesForMonthDedupAndGenderFilter(t *testing.T) {
	day := time.Date(2024, time.May, 24, 0, 0, 0, 0, time.UTC)
	girl := &model.Artist{ArtistID: 1, Name: model.NewUniqueString("aespa"), Gender: model.GenderFemale}
	boy := &model.Artist{ArtistID: 2, Name: model.NewUniqueString("Stray Kids"), Gender: model.GenderMale}

	releases := []model.Release{
		{Artist: girl, ArtistID: 1, Title: model.NewUniqueString("How Sweet"), AlbumName: model.NewUniqueString("How Sweet"), Date: day},
		// duplicate of the first one, only adds MV and Spotify links
		{Artist: girl, ArtistID: 1, Title: model.NewUniqueString("How Sweet"), AlbumName: model.NewUniqueString("How Sweet"), Date: day,
			MV: model.NewUniqueString("https://youtu.be/x1"), Spotify: model.NewUniqueString("https://open.spotify.com/x1")},
		{Artist: boy, ArtistID: 2, Title: model.NewUniqueString("Lose My Breath"), AlbumName: model.NewUniqueString("Lose My Breath"), Date: day},
	}

	repo := &mockReleaseRepo{byDateRange: releases}
	svc := &ReleaseService{repo: repo, logger: zap.NewNop()}

	all, err := svc.GetReleasesForMonth(context.Background(), "may-2024", false, false)
	if err != nil {
		t.Fatalf("GetReleasesForMonth error: %v", err)
	}
	if got := strings.Count(all, "<b>aespa</b>"); got != 1 {
		t.Errorf("expected dedup to keep single aespa line, got %d:\n%s", got, all)
	}
	if !strings.Contains(all, "https://youtu.be/x1") || !strings.Contains(all, "https://open.spotify.com/x1") {
		t.Error("dedup must merge MV and Spotify links from duplicates")
	}
	if !strings.Contains(all, "Releases for May 2024") || !strings.Contains(all, "<b>Stray Kids</b>") {
		t.Errorf("unexpected output header/content:\n%s", all)
	}

	female, err := svc.GetReleasesForMonth(context.Background(), "may-2024", true, false)
	if err != nil {
		t.Fatalf("GetReleasesForMonth error: %v", err)
	}
	if strings.Contains(female, "Stray Kids") || !strings.Contains(female, "aespa") {
		t.Errorf("female filter failed:\n%s", female)
	}

	male, err := svc.GetReleasesForMonth(context.Background(), "may-2024", false, true)
	if err != nil {
		t.Fatalf("GetReleasesForMonth error: %v", err)
	}
	if strings.Contains(male, "aespa") || !strings.Contains(male, "Stray Kids") {
		t.Errorf("male filter failed:\n%s", male)
	}

	repoEmpty := &mockReleaseRepo{}
	emptySvc := &ReleaseService{repo: repoEmpty, logger: zap.NewNop()}
	out, err := emptySvc.GetReleasesForMonth(context.Background(), "may-2024", false, false)
	if err != nil {
		t.Fatalf("GetReleasesForMonth error: %v", err)
	}
	if !strings.Contains(out, "No releases found") {
		t.Errorf("expected empty message, got:\n%s", out)
	}
}
