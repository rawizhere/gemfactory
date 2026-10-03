package scraper

import (
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/stretchr/testify/require"
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
		require.Equal(t, tc.artist, a, "splitTitle(%q)", tc.in)
		require.Equal(t, tc.album, al, "splitTitle(%q)", tc.in)
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
		require.Equal(t, tc.want, findDateInString(tc.in), "findDateInString(%q)", tc.in)
	}
}

func TestCleanArtistName(t *testing.T) {
	require.Equal(t, "BLACKPINK extra", cleanArtistName(" BLACKPINK \u202bextra\u202c "))
}

func TestMonthNumber(t *testing.T) {
	require.Equal(t, 4, monthNumber("April"))
	require.Equal(t, 12, monthNumber(" december "))
	require.Zero(t, monthNumber("Foo"), "unexpected monthNumber results")
}

func TestCleanAlbumURL(t *testing.T) {
	require.Equal(t, "https://kpopofficial.com/album/x/", cleanAlbumURL("https://kpopofficial.com/album/x/?utm=1#top"))
}

func TestUniqueStrings(t *testing.T) {
	in := []string{
		"https://kpopofficial.com/album/a/",
		"https://kpopofficial.com/album/a/?x=1",
		"https://kpopofficial.com/album/b/",
	}
	out := uniqueStrings(in)
	require.Len(t, out, 2, "uniqueStrings = %v", out)
	require.Equal(t, in[0], out[0], "uniqueStrings = %v", out)
	require.Equal(t, in[2], out[1], "uniqueStrings = %v", out)
}

func TestReleaseInMonthAndYear(t *testing.T) {
	d := time.Date(2024, time.March, 5, 0, 0, 0, 0, time.UTC)
	require.True(t, releaseInMonth(d, "march", "2024"), "expected date in march 2024")
	require.False(t, releaseInMonth(d, "april", "2024"), "date should not match other month/year")
	require.False(t, releaseInMonth(d, "march", "2023"), "date should not match other month/year")
	require.False(t, releaseInMonth(time.Time{}, "march", "2024"), "zero date must not match")
	require.True(t, releaseInYear(d, "2024"), "unexpected releaseInYear results")
	require.False(t, releaseInYear(d, "2025"), "unexpected releaseInYear results")
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

	f := &Fetcher{logger: zap.NewNop()}
	doc, err := newTestDoc(html)
	require.NoError(t, err, "failed to parse html")

	rels, links, err := f.parseEventPageFromDoc(doc, "https://kpopofficial.com/post/how-sweet")
	require.NoError(t, err)
	require.Len(t, rels, 2)

	first := rels[0]
	require.Equal(t, "NewJeans", first.Artist, "unexpected release metadata: %+v", first)
	require.Equal(t, "How Sweet", first.AlbumName, "unexpected release metadata: %+v", first)
	require.Equal(t, "How Sweet", first.TitleTrack, "unexpected release metadata: %+v", first)
	require.Equal(t, "How Sweet", first.Title, "main event title mismatch")
	require.Equal(t, time.Date(2024, time.May, 24, 0, 0, 0, 0, time.UTC), first.Date)
	require.Equal(t, "https://www.youtube.com/watch?v=fEw9f8u0BaM", first.MV)
	require.Equal(t, "https://open.spotify.com/album/12345", first.Spotify)

	require.Len(t, links, 2, "subLinks = %v", links)
	require.True(t, strings.HasSuffix(links[0], "/album/how-sweet"), "subLinks = %v", links)

	second := rels[1]
	require.Equal(t, "MV Release", second.Title, "secondary event title mismatch")
}

func TestParseEventPageFromDocFallbackDate(t *testing.T) {
	html := `<html><body>
<h1 class="entry-title">Artist – Album</h1>
<div class="entry-content"><p>Available since June 1, 2024 worldwide.</p></div>
</body></html>`

	f := &Fetcher{logger: zap.NewNop()}
	doc, err := newTestDoc(html)
	require.NoError(t, err, "failed to parse html")

	rels, _, err := f.parseEventPageFromDoc(doc, "https://kpopofficial.com/post/x")
	require.NoError(t, err)
	require.Len(t, rels, 1)
	require.Equal(t, time.Date(2024, time.June, 1, 0, 0, 0, 0, time.UTC), rels[0].Date)
}

func TestSplitTitleSpacedDashOnly(t *testing.T) {
	a, al := splitTitle("IVE Pre-release – Looks Can Kill (2026)")
	require.Equal(t, "IVE Pre-release", a, "splitTitle should keep hyphenated words intact")
	require.Equal(t, "Looks Can Kill (2026)", al)
	a, al = splitTitle("KICK OFF THE WALL Pt. 1")
	require.Equal(t, "KICK OFF THE WALL Pt. 1", a)
	require.Empty(t, al)
}

func TestParseEventPageFromDocGreenShift(t *testing.T) {
	html := `<html><body>
<h1 class="entry-title">X:IN 1st Studio Album – Chain Reaction (2026)</h1>
<div class="entry-content">
<div class="gspb_container outer-a">
<div class="gspb_container inner-a"><div class="gspb_text">Artist</div></div>
<div class="gspb_container val-a"><span class="gspb_meta_value">X:IN</span></div>
</div>
<div class="gspb_container outer-b">
<div class="gspb_container inner-b"><div class="gspb_text">Release Date</div></div>
<div class="gspb_container val-b"><span class="gspb_meta_value">October 6, 2026 · Tuesday · 6 PM KST</span></div>
</div>
<div class="gspb_container outer-c">
<div class="gspb_container inner-c"><div class="gspb_text">Album</div></div>
<div class="gspb_container val-c"><span class="gspb_meta_value">1st Studio Album – Chain Reaction</span></div>
</div>
</div></body></html>`

	f := &Fetcher{logger: zap.NewNop()}
	doc, err := newTestDoc(html)
	require.NoError(t, err, "failed to parse html")

	rels, _, err := f.parseEventPageFromDoc(doc, "https://kpopofficial.com/album/xin-chain-reaction")
	require.NoError(t, err)
	require.Len(t, rels, 1, "rels = %+v", rels)
	require.Equal(t, "X:IN", rels[0].Artist)
	require.Equal(t, "1st Studio Album – Chain Reaction", rels[0].AlbumName)
	require.Equal(t, "1st Studio Album – Chain Reaction", rels[0].Title)
	require.Equal(t, time.Date(2026, time.October, 6, 0, 0, 0, 0, time.UTC), rels[0].Date)
}

func TestParseEventPageFromDocTBATrack(t *testing.T) {
	html := `<html><body>
<h1 class="entry-title">RIIZE – Digital Single</h1>
<div class="entry-content">
<table>
<tr><td>Artist</td><td>RIIZE</td></tr>
<tr><td>Album</td><td>Digital Single</td></tr>
<tr><td>Title Track</td><td>To Be Announced (TBA)</td></tr>
<tr><td>Release Date</td><td>October 26, 2026</td></tr>
</table>
</div></body></html>`

	f := &Fetcher{logger: zap.NewNop()}
	doc, err := newTestDoc(html)
	require.NoError(t, err, "failed to parse html")

	rels, _, err := f.parseEventPageFromDoc(doc, "https://kpopofficial.com/album/riize")
	require.NoError(t, err)
	require.Len(t, rels, 1)
	require.Empty(t, rels[0].TitleTrack, "TBA title track must not be stored")
}

func newTestDoc(html string) (*goquery.Document, error) {
	return goquery.NewDocumentFromReader(strings.NewReader(html))
}

func TestMonthAndYearWindow(t *testing.T) {
	after, before, err := monthWindow("May", "2024")
	require.NoError(t, err, "unexpected error for valid month")
	require.False(t, after.After(before), "after %v should be before %v", after, before)

	_, _, err = monthWindow("InvalidMonth", "2024")
	require.Error(t, err, "expected error for invalid month")
	_, _, err = monthWindow("May", "-1")
	require.Error(t, err, "expected error for invalid year")

	yAfter, yBefore, err := yearWindow("2024")
	require.NoError(t, err, "unexpected error for valid year")
	require.False(t, yAfter.After(yBefore), "year after %v should be before %v", yAfter, yBefore)
	_, _, err = yearWindow("invalid")
	require.Error(t, err, "expected error for invalid year")
}
