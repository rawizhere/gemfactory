package downloader

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)

	// First cue (0.5-3.0s) ends exactly at... no, ends before start -> excluded.
	if strings.Contains(got, "Hello world") {
		t.Error("cue fully before clip start must be excluded")
	}
	// Second cue overlaps start; shifted to 0.
	if !strings.Contains(got, "00:00:00.000 --> 00:00:02.000") || !strings.Contains(got, "Second line text") {
		t.Errorf("overlapping cue not shifted correctly:\n%s", got)
	}
	// Third cue fully inside; shifted by 4s; tag stripped; cue settings dropped.
	if !strings.Contains(got, "00:00:05.900 --> 00:00:08.500") {
		t.Errorf("inside cue not shifted correctly:\n%s", got)
	}
	if strings.Contains(got, "<c>") {
		t.Error("<c> tags must be removed")
	}
	if strings.Contains(got, "align:start") {
		t.Error("cue settings must be dropped")
	}
	// Cue starting at clip end excluded.
	if strings.Contains(got, "Outside window") {
		t.Error("cue outside the clip window must be excluded")
	}
	if n != 2 {
		t.Errorf("expected 2 cues kept, got %d", n)
	}
	if !strings.HasPrefix(got, "WEBVTT\nKind: captions\nLanguage: en\n") {
		t.Errorf("missing canonical header:\n%s", got)
	}
}

func TestTrimVTTFileCueEndingAtStartExcluded(t *testing.T) {
	in := writeTemp(t, "WEBVTT\n\n00:00:00.000 --> 00:00:04.000\nends at start\n\n00:00:04.000 --> 00:00:08.000\nstarts at start\n")
	out := filepath.Join(t.TempDir(), "t.vtt")

	n, err := TrimVTTFile(in, 4000, 8000, out)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(out)
	got := string(data)
	if strings.Contains(got, "ends at start") {
		t.Error("cue ending exactly at clip start must be excluded")
	}
	if !strings.Contains(got, "starts at start") {
		t.Error("cue starting exactly at clip start must be included")
	}
	if n != 1 {
		t.Errorf("want 1 cue, got %d", n)
	}
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
	// Mirrors the real YouTube auto-caption (rolling cue) format: word-level
	// <c>/<00:00:00.546> tags, whitespace-only filler lines and echo cues.
	in := writeTemp(t, `WEBVTT
Kind: captions
Language: ru

00:00:00.080 --> 00:00:03.310 align:start position:0%
 
Увидят <00:00:00.546><c>ли </c><00:00:01.012><c>это </c>

00:00:03.310 --> 00:00:03.320 align:start position:0%
Увидят ли это
 
00:00:10.000 --> 00:00:12.000 align:start position:0%
 
До <00:00:11.000><c>окна </c>

00:00:12.000 --> 00:00:12.010 align:start position:0%
До окна
 
00:00:23.500 --> 00:00:26.000 align:start position:0%
 
Внутри <00:00:24.000><c>предела </c>
`)
	out := filepath.Join(t.TempDir(), "trimmed.vtt")

	n, err := TrimVTTFile(in, 23000, 27000, out)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(out)
	got := string(data)

	if n != 1 {
		t.Errorf("want 1 cue, got %d:\n%s", n, got)
	}
	// Text of cues outside the window must not leak in as timing-less blocks.
	if strings.Contains(got, "Увидят") || strings.Contains(got, "окна") || strings.Contains(got, "До") {
		t.Errorf("text from skipped cues leaked into output:\n%s", got)
	}
	// Every kept block must carry a timing line; inline tags stripped.
	if !strings.Contains(got, "00:00:00.500 --> 00:00:03.000\nВнутри предела") {
		t.Errorf("kept cue missing or malformed:\n%s", got)
	}
	if strings.Contains(got, "<c>") || strings.Contains(got, "<00:00:") {
		t.Errorf("inline tags must be stripped:\n%s", got)
	}
}

func TestVideoIDFromURL(t *testing.T) {
	cases := map[string]string{
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ":         "dQw4w9WgXcQ",
		"https://youtu.be/dQw4w9WgXcQ?t=42":                   "dQw4w9WgXcQ",
		"https://www.youtube.com/shorts/dQw4w9WgXcQ":          "dQw4w9WgXcQ",
		"https://www.youtube.com/live/dQw4w9WgXcQ?feature=x":  "dQw4w9WgXcQ",
		"dQw4w9WgXcQ":                                         "dQw4w9WgXcQ",
		"https://www.tiktok.com/@user/video/7123456789012345": "tt_7123456789012345",
		"https://vm.tiktok.com/ZM8abc123/":                    "tt_ZM8abc123",
	}
	for raw, want := range cases {
		got, err := videoIDFromURL(raw)
		if err != nil || got != want {
			t.Errorf("videoIDFromURL(%q) = (%q, %v), want %q", raw, got, err, want)
		}
	}
	if _, err := videoIDFromURL("https://example.com/video"); err == nil {
		t.Error("expected error for unsupported URL")
	}
}

func TestResolveSubsLanguage(t *testing.T) {
	meta := &SourceMeta{
		Subtitles: map[string][]SubtitleTrack{
			"en":    {{Ext: "vtt"}},
			"en-US": {{Ext: "vtt"}},
			"ko":    {{Ext: "vtt"}},
		},
		AutomaticCaptions: map[string][]SubtitleTrack{
			"ru":    {{Ext: "vtt"}},
			"ru-en": {{Ext: "vtt"}},
			"fr":    {{Ext: "vtt"}},
		},
	}

	cases := []struct {
		requested string
		want      string
		ok        bool
	}{
		{"ko", "ko", true},       // exact manual match
		{"en-US", "en-US", true}, // exact regional manual match wins over en
		{"fr", "fr", true},       // exact auto-caption match wins
		{"de", "de-en", true},    // missing -> derived translation form
	}
	for _, c := range cases {
		got, ok := ResolveSubsLanguage(meta, c.requested)
		if got != c.want || ok != c.ok {
			t.Errorf("ResolveSubsLanguage(%q) = (%q, %v), want (%q, %v)", c.requested, got, ok, c.want, c.ok)
		}
	}

	enOnly := &SourceMeta{Subtitles: map[string][]SubtitleTrack{"en": {}}}
	if lang, ok := ResolveSubsLanguage(enOnly, "en"); !ok || lang != "en" {
		t.Errorf("ResolveSubsLanguage(en-only, en) = (%q, %v), want (en, true)", lang, ok)
	}
}

func TestFormatCaption(t *testing.T) {
	// TikTok case: title is truncated prefix of description.
	tiktokMeta := &SourceMeta{
		Title:       "Theburntpeanut talks about dating clankers..😂☠️#theburntpeanut #thebu...",
		Description: "Theburntpeanut talks about dating clankers..😂☠️#theburntpeanut #theburntpeanutclip #fyp",
		Tags:        []string{"theburntpeanut", "fyp", "funny"},
	}
	gotTikTok := FormatCaption(tiktokMeta)
	if strings.Count(gotTikTok, "Theburntpeanut") > 1 {
		t.Errorf("expected no duplicate header for tiktok, got: %s", gotTikTok)
	}
	if !strings.Contains(gotTikTok, "#funny") {
		t.Errorf("expected missing tag #funny to be added, got: %s", gotTikTok)
	}

	// YouTube case: separate distinct title and description.
	ytMeta := &SourceMeta{
		Title:       "Kep1er WA DA DA",
		Description: "Official Dance Practice Video with long boilerplate",
		Tags:        []string{"Kep1er", "WADADA", "Kpop"},
	}
	gotYT := FormatCaption(ytMeta)
	if !strings.Contains(gotYT, "<b>Kep1er WA DA DA</b>") {
		t.Errorf("expected title in bold, got: %s", gotYT)
	}
	if strings.Contains(gotYT, "Official Dance Practice Video") {
		t.Errorf("expected no description body in minimalist mode, got: %s", gotYT)
	}
	if !strings.Contains(gotYT, "#Kep1er #WADADA #Kpop") {
		t.Errorf("expected tags in caption, got: %s", gotYT)
	}
}
