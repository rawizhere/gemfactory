package downloader

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const sampleVTT = `WEBVTT
Kind: captions
Language: en

1
00:00:00.500 --> 00:00:03.000
Hello world

00:00:04.000 --> 00:00:06.000 align:start size:50%
Second line text

00:00:09.900 --> 00:00:12.500
Third cue with <c>tag</c> content

00:00:20.000 --> 00:00:25.000
Outside window
`

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "in.vtt")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestTrimVTTFileOverlapAndShift(t *testing.T) {
	in := writeTemp(t, sampleVTT)
	out := filepath.Join(t.TempDir(), "trimmed.vtt")

	n, err := TrimVTTFile(in, 4000, 10000, out)
	require.NoError(t, err)

	data, err := os.ReadFile(out)
	require.NoError(t, err)
	got := string(data)

	// First cue (0.5-3.0s) ends exactly at... no, ends before start -> excluded.
	require.NotContains(t, got, "Hello world", "cue fully before clip start must be excluded")
	// Second cue overlaps start; shifted to 0.
	require.Contains(t, got, "00:00:00.000 --> 00:00:02.000", "overlapping cue not shifted correctly:\n%s", got)
	require.Contains(t, got, "Second line text", "overlapping cue not shifted correctly:\n%s", got)
	// Third cue fully inside; shifted by 4s; tag stripped; cue settings dropped.
	require.Contains(t, got, "00:00:05.900 --> 00:00:08.500", "inside cue not shifted correctly:\n%s", got)
	require.NotContains(t, got, "<c>", "<c> tags must be removed")
	require.NotContains(t, got, "align:start", "cue settings must be dropped")
	// Cue starting at clip end excluded.
	require.NotContains(t, got, "Outside window", "cue outside the clip window must be excluded")
	require.Equal(t, 2, n, "expected 2 cues kept")
	require.True(t, strings.HasPrefix(got, "WEBVTT\nKind: captions\nLanguage: en\n"), "missing canonical header:\n%s", got)
}

func TestTrimVTTFileCueEndingAtStartExcluded(t *testing.T) {
	in := writeTemp(t, "WEBVTT\n\n00:00:00.000 --> 00:00:04.000\nends at start\n\n00:00:04.000 --> 00:00:08.000\nstarts at start\n")
	out := filepath.Join(t.TempDir(), "t.vtt")

	n, err := TrimVTTFile(in, 4000, 8000, out)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(out)
	require.NoError(t, err)
	got := string(data)
	require.NotContains(t, got, "ends at start", "cue ending exactly at clip start must be excluded")
	require.Contains(t, got, "starts at start", "cue starting exactly at clip start must be included")
	require.Equal(t, 1, n)
}

func TestTrimVTTFileEmptyResult(t *testing.T) {
	in := writeTemp(t, sampleVTT)
	out := filepath.Join(t.TempDir(), "empty.vtt")

	n, err := TrimVTTFile(in, 100000, 200000, out)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("expected 0 cues, got %d", n)
	}
}

func TestTrimVTTFileYouTubeAutoSubFormat(t *testing.T) {
	// Mirrors the real YouTube auto-caption (rolling cue) format: word-level <c>/<00:00:00.546> tags, whitespace-only filler lines and echo cues.
	in := writeTemp(t, `WEBVTT
Kind: captions
Language: en

00:00:00.080 --> 00:00:03.310 align:start position:0%
 
Will <00:00:00.546><c>they </c><00:00:01.012><c>see </c>

00:00:03.310 --> 00:00:03.320 align:start position:0%
Will they see
 
00:00:10.000 --> 00:00:12.000 align:start position:0%
 
To <00:00:11.000><c>window </c>

00:00:12.000 --> 00:00:12.010 align:start position:0%
To window
 
00:00:23.500 --> 00:00:26.000 align:start position:0%
 
Inside <00:00:24.000><c>limits </c>
`)
	out := filepath.Join(t.TempDir(), "trimmed.vtt")

	n, err := TrimVTTFile(in, 23000, 27000, out)
	require.NoError(t, err)
	data, err := os.ReadFile(out)
	require.NoError(t, err)
	got := string(data)

	require.Equal(t, 1, n, "want 1 cue:\n%s", got)
	// Text of cues outside the window must not leak in as timing-less blocks.
	require.NotContains(t, got, "Will", "text from skipped cues leaked:\n%s", got)
	require.NotContains(t, got, "window", "text from skipped cues leaked:\n%s", got)
	require.NotContains(t, got, "To", "text from skipped cues leaked:\n%s", got)
	// Every kept block must carry a timing line; inline tags stripped.
	require.Contains(t, got, "00:00:00.500 --> 00:00:03.000\nInside limits", "kept cue missing or malformed:\n%s", got)
	require.NotContains(t, got, "<c>", "inline tags must be stripped:\n%s", got)
	require.NotContains(t, got, "<00:00:", "inline tags must be stripped:\n%s", got)
}

func TestVideoIDFromURL(t *testing.T) {
	cases := map[string]string{
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ":        "dQw4w9WgXcQ",
		"https://youtu.be/dQw4w9WgXcQ?t=42":                  "dQw4w9WgXcQ",
		"https://www.youtube.com/shorts/dQw4w9WgXcQ":         "dQw4w9WgXcQ",
		"https://www.youtube.com/live/dQw4w9WgXcQ?feature=x": "dQw4w9WgXcQ",
		"dQw4w9WgXcQ": "dQw4w9WgXcQ",
		"https://www.tiktok.com/@user/video/7123456789012345": "tt_7123456789012345",
		"https://vm.tiktok.com/ZM8abc123/":                    "tt_ZM8abc123",
	}
	for raw, want := range cases {
		got, err := videoIDFromURL(raw)
		require.NoError(t, err, "videoIDFromURL(%q)", raw)
		require.Equal(t, want, got, "videoIDFromURL(%q)", raw)
	}
	_, err := videoIDFromURL("https://example.com/video")
	require.Error(t, err, "expected error for unsupported URL")

	require.True(t, IsTikTokURL("https://www.tiktok.com/@user/video/123"))
	require.True(t, IsShortsURL("https://youtube.com/shorts/abc"))
	require.True(t, IsDirectDownloadURL("https://vm.tiktok.com/123"), "expected true for direct download helpers")
	require.Equal(t, "https://youtu.be/dQw4w9WgXcQ", ExtractFirstURL("Check this: https://youtu.be/dQw4w9WgXcQ!"))
}

func TestResolveSubtitleTrack(t *testing.T) {
	meta := &SourceMeta{
		Subtitles: map[string][]SubtitleTrack{
			"en":    {{Ext: "vtt", URL: "https://example.com/en"}},
			"en-US": {{Ext: "vtt", URL: "https://example.com/en-us"}},
			"ko":    {{Ext: "vtt", URL: "https://example.com/ko"}},
		},
		AutomaticCaptions: map[string][]SubtitleTrack{
			"fr": {{Ext: "vtt", URL: "https://example.com/auto-fr"}},
		},
	}

	koRes, err := ResolveSubtitleTrack(meta, "ko", nil)
	require.NoError(t, err)
	require.Equal(t, "ko", koRes.FinalLang, "want ko direct: %+v", koRes)
	require.Empty(t, koRes.TargetLang, "want ko direct: %+v", koRes)

	enUSRes, err := ResolveSubtitleTrack(meta, "en-US", nil)
	require.NoError(t, err)
	require.Equal(t, "en", enUSRes.FinalLang, "want en direct: %+v", enUSRes)
	require.Empty(t, enUSRes.TargetLang, "want en direct: %+v", enUSRes)

	ruRes, err := ResolveSubtitleTrack(meta, "ru", nil)
	require.NoError(t, err)
	require.Equal(t, "ru", ruRes.FinalLang)
	require.Equal(t, "ru", ruRes.TargetLang, "want translated from ko to ru: %+v", ruRes)
	require.Equal(t, "ko", ruRes.SourceLang, "want translated from ko to ru: %+v", ruRes)

	autoFr := &SourceMeta{
		AutomaticCaptions: map[string][]SubtitleTrack{"fr": {{Ext: "vtt", URL: "https://example.com/auto-fr"}}},
	}
	_, err = ResolveSubtitleTrack(autoFr, "fr", nil)
	require.Error(t, err, "expected error when only auto captions exist")

	autoOnly := &SourceMeta{
		AutomaticCaptions: map[string][]SubtitleTrack{"ko": {{Ext: "vtt", URL: "https://example.com/auto-ko"}}},
	}
	_, err = ResolveSubtitleTrack(autoOnly, "en", nil)
	require.Error(t, err, "expected error when only auto captions exist")

	emptyMeta := &SourceMeta{}
	_, err = ResolveSubtitleTrack(emptyMeta, "en", nil)
	require.Error(t, err, "expected error when no subtitles exist")
}

func TestFormatCaption(t *testing.T) {
	// TikTok case: title is truncated prefix of description.
	tiktokMeta := &SourceMeta{
		Title:       "Theburntpeanut talks about dating clankers..😂☠️#theburntpeanut #thebu...",
		Description: "Theburntpeanut talks about dating clankers..😂☠️#theburntpeanut #theburntpeanutclip #fyp",
		Tags:        []string{"theburntpeanut", "fyp", "funny"},
	}
	gotTikTok := FormatCaption(tiktokMeta)
	require.LessOrEqual(t, strings.Count(gotTikTok, "Theburntpeanut"), 1, "expected no duplicate header for tiktok, got: %s", gotTikTok)
	require.Contains(t, gotTikTok, "#funny", "expected missing tag #funny to be added, got: %s", gotTikTok)

	// YouTube case: separate distinct title and description.
	ytMeta := &SourceMeta{
		Title:       "Kep1er WA DA DA",
		Description: "Official Dance Practice Video with long boilerplate",
		Tags:        []string{"Kep1er", "WADADA", "Kpop"},
	}
	gotYT := FormatCaption(ytMeta)
	require.Contains(t, gotYT, "<b>Kep1er WA DA DA</b>", "expected title in bold, got: %s", gotYT)
	require.NotContains(t, gotYT, "Official Dance Practice Video", "expected no description body in minimalist mode, got: %s", gotYT)
	require.Contains(t, gotYT, "#Kep1er #WADADA #Kpop", "expected tags in caption, got: %s", gotYT)
}

func TestDisplayTitle(t *testing.T) {
	metaWithAlt := &SourceMeta{
		Title:    "아이돌이 결정사를 간다면",
		AltTitle: "If an Idol Goes to a Matchmaking Agency",
	}
	require.Equal(t, "If an Idol Goes to a Matchmaking Agency", DisplayTitle(metaWithAlt), "expected alt title when present")

	metaSame := &SourceMeta{
		Title:    "Hello World",
		AltTitle: "hello world",
	}
	require.Equal(t, "hello world", DisplayTitle(metaSame), "expected alt title")

	metaNoAlt := &SourceMeta{
		Title: "Hello World",
	}
	require.Equal(t, "Hello World", DisplayTitle(metaNoAlt), "expected original title when no alt")
}

func TestSplitDialogueLines(t *testing.T) {
	cases := map[string]string{
		"- Да. - Ой, ты меня испугал!": "- Да.\n- Ой, ты меня испугал!",
		"Обычная строка без диалога":   "Обычная строка без диалога",
		"I - I don't know":    "I - I don't know",
		"- А. - Б. - В.":      "- А.\n- Б.\n- В.",
		"Первая\n- Да. - Ой!": "Первая\n- Да.\n- Ой!",
	}
	for in, want := range cases {
		require.Equal(t, want, splitDialogueLines(in), "splitDialogueLines(%q)", in)
	}
}
