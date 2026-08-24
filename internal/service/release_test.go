package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"gemfactory/internal/model"
)

func TestMonthToInt(t *testing.T) {
	require.Equal(t, 3, monthToInt("March"))
	require.Equal(t, 12, monthToInt("DECEMBER"))
	require.Zero(t, monthToInt("notamonth"), "unexpected monthToInt results")
}

func TestValidateRelease(t *testing.T) {
	valid := &model.Release{ArtistID: 1, Title: model.NewUniqueString("X"), Date: time.Now()}
	require.NoError(t, NewReleaseService(nil, nil, nil).validateRelease(valid), "expected valid release")

	cases := []*model.Release{
		nil,
		{Title: model.NewUniqueString("X"), Date: time.Now()},
		{ArtistID: 1, Date: time.Now()},
		{ArtistID: 1, Title: model.NewUniqueString("  ")},
	}
	svc := &ReleaseService{}
	for i, c := range cases {
		require.Error(t, svc.validateRelease(c), "case %d: expected validation error", i)
	}
}

func TestCleanReleaseTitleAndLink(t *testing.T) {
	require.Equal(t, "Multi word title", CleanReleaseTitle("  {Multi  word \n title }"))

	for _, link := range []string{
		"https://youtube.com/@channel",
		"https://www.youtube.com/channel/abc",
		"https://youtube.com/user/name",
		"https://youtube.com/c/name",
	} {
		require.Empty(t, CleanLink(link), "CleanLink(%q) should be empty", link)
	}
	require.Equal(t, "https://youtu.be/abc123", CleanLink("https://youtu.be/abc123"), "CleanLink dropped valid link")
	require.Empty(t, CleanLink(""), `CleanLink("") should stay empty`)
}

func TestFindArtist(t *testing.T) {
	aespa := &model.Artist{ArtistID: 1, Name: model.NewUniqueString("aespa")}
	blackpink := &model.Artist{ArtistID: 2, Name: model.NewUniqueString("BLACKPINK")}
	txt := &model.Artist{ArtistID: 3, Name: model.NewUniqueString("TXT")}
	classy := &model.Artist{ArtistID: 4, Name: model.NewUniqueString("CLASS:y")}
	lightsum := &model.Artist{ArtistID: 5, Name: model.NewUniqueString("LIGHTSUM")}
	somi := &model.Artist{ArtistID: 6, Name: model.NewUniqueString("JEON SOMI")}
	solar := &model.Artist{ArtistID: 7, Name: model.NewUniqueString("Solar")}
	zico := &model.Artist{ArtistID: 8, Name: model.NewUniqueString("ZICO")}
	apink := &model.Artist{ArtistID: 9, Name: model.NewUniqueString("Apink")}
	gfriend := &model.Artist{ArtistID: 10, Name: model.NewUniqueString("GFRIEND")}
	omg := &model.Artist{ArtistID: 11, Name: model.NewUniqueString("OH MY GIRL")}
	irene := &model.Artist{ArtistID: 12, Name: model.NewUniqueString("IRENE")}
	redvelvet := &model.Artist{ArtistID: 13, Name: model.NewUniqueString("Red Velvet")}

	artists := map[string]*model.Artist{
		"aespa":      aespa,
		"blackpink":  blackpink,
		"txt":        txt,
		"class:y":    classy,
		"lightsum":   lightsum,
		"jeon somi":  somi,
		"solar":      solar,
		"zico":       zico,
		"apink":      apink,
		"gfriend":    gfriend,
		"oh my girl": omg,
		"irene":      irene,
		"red velvet": redvelvet,
	}

	tests := []struct {
		in   string
		want *model.Artist
	}{
		{"Aespa", aespa},
		{"  AESPA  ", aespa},
		{"aespa (Karina)", aespa},
		{"aespa (JP)", aespa},
		{"aespa - Drama", aespa},
		{"BLACKPINK & Rosé", blackpink},
		{"TOMORROW X TOGETHER", txt},
		{"TOMORROW X TOGETHER (JP)", txt},
		{"CLASSy", classy},
		{"LIGHTSUM(SANGAH, CHOWON, JUHYEON)", lightsum},
		{"JVKE x JEON SOMI", somi},
		{"Solar x Accusefive", solar},
		{"ZICO, Crush", zico},
		{"JEONG EUNJI (Apink)", apink},
		{"YERIN (GFRIEND)", gfriend},
		{"Hyojung/MIMI (OH MY GIRL)", omg},
		{"IRENE (Red Velvet)", irene},
		{"Unknown Artist", nil},
	}
	for _, tc := range tests {
		got := (&ReleaseService{}).findArtist(tc.in, artists)
		require.Equal(t, tc.want, got, "findArtist(%q)", tc.in)
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
	require.NoError(t, err)
	require.Equal(t, 1, strings.Count(all, "<b>aespa</b>"), "expected dedup to keep single aespa line:\n%s", all)
	require.Contains(t, all, "https://youtu.be/x1", "dedup must merge MV and Spotify links from duplicates")
	require.Contains(t, all, "https://open.spotify.com/x1", "dedup must merge MV and Spotify links from duplicates")
	require.Contains(t, all, "Releases for May 2024", "unexpected output header/content:\n%s", all)
	require.Contains(t, all, "<b>Stray Kids</b>", "unexpected output header/content:\n%s", all)

	female, err := svc.GetReleasesForMonth(context.Background(), "may-2024", true, false)
	require.NoError(t, err)
	require.NotContains(t, female, "Stray Kids", "female filter failed:\n%s", female)
	require.Contains(t, female, "aespa", "female filter failed:\n%s", female)

	male, err := svc.GetReleasesForMonth(context.Background(), "may-2024", false, true)
	require.NoError(t, err)
	require.NotContains(t, male, "aespa", "male filter failed:\n%s", male)
	require.Contains(t, male, "Stray Kids", "male filter failed:\n%s", male)

	repoEmpty := &mockReleaseRepo{}
	emptySvc := &ReleaseService{repo: repoEmpty, logger: zap.NewNop()}
	out, err := emptySvc.GetReleasesForMonth(context.Background(), "may-2024", false, false)
	require.NoError(t, err)
	require.Contains(t, out, "No releases found", "expected empty message, got:\n%s", out)
}
