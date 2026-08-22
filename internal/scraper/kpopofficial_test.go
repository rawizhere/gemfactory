package scraper

import (
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
	"go.uber.org/zap"
)

func TestSplitTitle(t *testing.T) {
	tests := []struct {
		in, artist, album string
	}{
		{"Artist – Album Name", "Artist", "Album Name"},
		{"Artist - Album Name", "Artist", "Album Name"},
		{"Just Artist", "Just Artist", ""},
	}
	for _, tc := range tests {
		a, al := splitTitle(tc.in)
		if a != tc.artist || al != tc.album {
			t.Errorf("splitTitle(%q) = (%q, %q), want (%q, %q)", tc.in, a, al, tc.artist, tc.album)
		}
	}
}

func TestFindDateInString(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"Release Date: March 15, 2024", "March 15, 2024"},
		{"march 5 2024", "March 5, 2024"},
		{"Offline\u00a0Date: December 31, 2023", "December 31, 2023"},
		{"no date here", ""},
	}
	for _, tc := range tests {
		if got := findDateInString(tc.in); got != tc.want {
			t.Errorf("findDateInString(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestCleanArtistName(t *testing.T) {
	got := cleanArtistName(" BLACKPINK \u202bextra\u202c ")
	if got != "BLACKPINK extra" {
		t.Errorf("cleanArtistName = %q, want %q", got, "BLACKPINK extra")
	}
}

func TestMonthNumber(t *testing.T) {
	if monthNumber("April") != 4 || monthNumber(" december ") != 12 || monthNumber("Foo") != 0 {
		t.Error("unexpected monthNumber results")
	}
}

func TestCleanAlbumURL(t *testing.T) {
	got := cleanAlbumURL("https://kpopofficial.com/album/x/?utm=1#top")
	want := "https://kpopofficial.com/album/x/"
	if got != want {
		t.Errorf("cleanAlbumURL = %q, want %q", got, want)
	}
}

func TestUniqueStrings(t *testing.T) {
	in := []string{
		"https://kpopofficial.com/album/a/",
		"https://kpopofficial.com/album/a/?x=1",
		"https://kpopofficial.com/album/b/",
	}
	out := uniqueStrings(in)
	if len(out) != 2 || out[0] != in[0] || out[1] != in[2] {
		t.Errorf("uniqueStrings = %v, want first and third entries", out)
	}
}

func TestReleaseInMonthAndYear(t *testing.T) {
	d := time.Date(2024, time.March, 5, 0, 0, 0, 0, time.UTC)
	if !releaseInMonth(d, "march", "2024") {
		t.Error("expected date in march 2024")
	}
	if releaseInMonth(d, "april", "2024") || releaseInMonth(d, "march", "2023") {
		t.Error("date should not match other month/year")
	}
	if releaseInMonth(time.Time{}, "march", "2024") {
		t.Error("zero date must not match")
	}
	if !releaseInYear(d, "2024") || releaseInYear(d, "2025") {
		t.Error("unexpected releaseInYear results")
	}
}

func TestParseEventPageFromDoc(t *testing.T) {
	html := `<html><body>
<h1 class="entry-title">NewJeans – How Sweet</h1>
<div class="entry-content">
<table>
<tr><td>Artist</td><td>NewJeans</td></tr>
<tr><td>Album</td><td>How Sweet</td></tr>
<tr><td>Title Track</td><td>How Sweet</td></tr>
<tr><td>Release Date</td><td>May 24, 2024</td></tr>
<tr><td>MV Release</td><td>May 24, 2024 <a href="https://shop.example.com">Buy</a></td></tr>
<tr><td>Tracklist</td><td><a href="/album/how-sweet">View</a></td></tr>
</table>
<iframe src="https://www.youtube.com/embed/fEw9f8u0BaM?si=abc"></iframe>
<a href="https://open.spotify.com/album/12345?si=x">Spotify</a>
</div></body></html>`

	f := &fetcherImpl{logger: zap.NewNop()}
	doc, err := newTestDoc(html)
	if err != nil {
		t.Fatalf("failed to parse html: %v", err)
	}

	rels, links, err := f.parseEventPageFromDoc(doc, "https://kpopofficial.com/post/how-sweet")
	if err != nil {
		t.Fatalf("parseEventPageFromDoc error: %v", err)
	}
	if len(rels) != 2 {
		t.Fatalf("got %d releases, want 2", len(rels))
	}

	first := rels[0]
	if first.Artist != "NewJeans" || first.AlbumName != "How Sweet" || first.TitleTrack != "How Sweet" {
		t.Errorf("unexpected release metadata: %+v", first)
	}
	if first.Title != "How Sweet" {
		t.Errorf("main event title = %q, want %q", first.Title, "How Sweet")
	}
	if want := time.Date(2024, time.May, 24, 0, 0, 0, 0, time.UTC); !first.Date.Equal(want) {
		t.Errorf("date = %v, want %v", first.Date, want)
	}
	if first.MV != "https://www.youtube.com/watch?v=fEw9f8u0BaM" {
		t.Errorf("MV = %q", first.MV)
	}
	if first.Spotify != "https://open.spotify.com/album/12345" {
		t.Errorf("Spotify = %q", first.Spotify)
	}
	if len(links) != 2 || !strings.HasSuffix(links[0], "/album/how-sweet") {
		t.Errorf("subLinks = %v, want album and spotify links", links)
	}

	second := rels[1]
	if second.Title != "MV Release" {
		t.Errorf("secondary event title = %q, want %q", second.Title, "MV Release")
	}
}

func TestParseEventPageFromDocFallbackDate(t *testing.T) {
	html := `<html><body>
<h1 class="entry-title">Artist – Album</h1>
<div class="entry-content"><p>Available since June 1, 2024 worldwide.</p></div>
</body></html>`

	f := &fetcherImpl{logger: zap.NewNop()}
	doc, err := newTestDoc(html)
	if err != nil {
		t.Fatalf("failed to parse html: %v", err)
	}

	rels, _, err := f.parseEventPageFromDoc(doc, "https://kpopofficial.com/post/x")
	if err != nil {
		t.Fatalf("parseEventPageFromDoc error: %v", err)
	}
	if len(rels) != 1 {
		t.Fatalf("got %d releases, want 1", len(rels))
	}
	want := time.Date(2024, time.June, 1, 0, 0, 0, 0, time.UTC)
	if !rels[0].Date.Equal(want) {
		t.Errorf("date = %v, want %v", rels[0].Date, want)
	}
}

func newTestDoc(html string) (*goquery.Document, error) {
	return goquery.NewDocumentFromReader(strings.NewReader(html))
}

func TestMonthAndYearWindow(t *testing.T) {
	after, before, err := monthWindow("May", "2024")
	if err != nil {
		t.Fatalf("unexpected error for valid month: %v", err)
	}
	if after.After(before) {
		t.Errorf("after %v should be before %v", after, before)
	}

	if _, _, err := monthWindow("InvalidMonth", "2024"); err == nil {
		t.Error("expected error for invalid month")
	}
	if _, _, err := monthWindow("May", "-1"); err == nil {
		t.Error("expected error for invalid year")
	}

	yAfter, yBefore, err := yearWindow("2024")
	if err != nil {
		t.Fatalf("unexpected error for valid year: %v", err)
	}
	if yAfter.After(yBefore) {
		t.Errorf("year after %v should be before %v", yAfter, yBefore)
	}
	if _, _, err := yearWindow("invalid"); err == nil {
		t.Error("expected error for invalid year")
	}
}
