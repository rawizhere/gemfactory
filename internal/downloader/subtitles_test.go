package downloader

import (
	"os"
	"path/filepath"
	"reflect"
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
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(out)
	got := string(data)

	if n != 1 {
		t.Errorf("want 1 cue, got %d:\n%s", n, got)
	}
	// Text of cues outside the window must not leak in as timing-less blocks.
	if strings.Contains(got, "Will") || strings.Contains(got, "window") || strings.Contains(got, "To") {
		t.Errorf("text from skipped cues leaked into output:\n%s", got)
	}
	// Every kept block must carry a timing line; inline tags stripped.
	if !strings.Contains(got, "00:00:00.500 --> 00:00:03.000\nInside limits") {
		t.Errorf("kept cue missing or malformed:\n%s", got)
	}
	if strings.Contains(got, "<c>") || strings.Contains(got, "<00:00:") {
		t.Errorf("inline tags must be stripped:\n%s", got)
	}
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
		if err != nil || got != want {
			t.Errorf("videoIDFromURL(%q) = (%q, %v), want %q", raw, got, err, want)
		}
	}
	if _, err := videoIDFromURL("https://example.com/video"); err == nil {
		t.Error("expected error for unsupported URL")
	}

	if !IsTikTokURL("https://www.tiktok.com/@user/video/123") || !IsShortsURL("https://youtube.com/shorts/abc") || !IsDirectDownloadURL("https://vm.tiktok.com/123") {
		t.Error("expected true for direct download helpers")
	}
	if first := ExtractFirstURL("Check this: https://youtu.be/dQw4w9WgXcQ!"); first != "https://youtu.be/dQw4w9WgXcQ" {
		t.Errorf("ExtractFirstURL = %q, want %q", first, "https://youtu.be/dQw4w9WgXcQ")
	}
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

	// 1. Direct manual match
	koRes, err := ResolveSubtitleTrack(meta, "ko")
	if err != nil || koRes.FinalLang != "ko" || koRes.TargetLang != "" {
		t.Errorf("ResolveSubtitleTrack(ko) = (%+v, %v), want ko direct", koRes, err)
	}

	// 2. Exact regional manual match wins over en
	enUSRes, err := ResolveSubtitleTrack(meta, "en-US")
	if err != nil || enUSRes.FinalLang != "en-US" || enUSRes.TargetLang != "" {
		t.Errorf("ResolveSubtitleTrack(en-US) = (%+v, %v), want en-US direct", enUSRes, err)
	}

	// 3. RU requested: manual 'en' is translated to 'ru'
	ruRes, err := ResolveSubtitleTrack(meta, "ru")
	if err != nil || ruRes.FinalLang != "ru" || ruRes.TargetLang != "ru" || ruRes.SourceLang != "en" {
		t.Errorf("ResolveSubtitleTrack(ru) = (%+v, %v), want translated from en to ru", ruRes, err)
	}

	// 4. Automatic captions only: direct match in auto-captions
	autoFr := &SourceMeta{
		AutomaticCaptions: map[string][]SubtitleTrack{"fr": {{Ext: "vtt", URL: "https://example.com/auto-fr"}}},
	}
	frRes, err := ResolveSubtitleTrack(autoFr, "fr")
	if err != nil || frRes.FinalLang != "fr" || frRes.TargetLang != "" || frRes.TrackURL != "https://example.com/auto-fr" {
		t.Errorf("ResolveSubtitleTrack(autoFr, fr) = (%+v, %v), want fr from auto captions", frRes, err)
	}

	// 5. Automatic captions only: translates auto 'ko' to 'en'
	autoOnly := &SourceMeta{
		AutomaticCaptions: map[string][]SubtitleTrack{"ko": {{Ext: "vtt", URL: "https://example.com/auto-ko"}}},
	}
	enAutoRes, err := ResolveSubtitleTrack(autoOnly, "en")
	if err != nil || enAutoRes.FinalLang != "en" || enAutoRes.TargetLang != "en" || enAutoRes.SourceLang != "ko" {
		t.Errorf("ResolveSubtitleTrack(autoOnly, en) = (%+v, %v), want translated from auto-ko to en", enAutoRes, err)
	}

	// 6. No subtitles at all -> error
	emptyMeta := &SourceMeta{}
	if _, err := ResolveSubtitleTrack(emptyMeta, "en"); err == nil {
		t.Error("expected error when no subtitles exist")
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

func TestBuildFallbackChain(t *testing.T) {
	tests := []struct {
		primary string
		want    []string
	}{
		{ProviderGoogle, []string{ProviderGoogle, ProviderGemini, ProviderGroq}},
		{ProviderGemini, []string{ProviderGemini, ProviderGroq, ProviderGoogle}},
		{ProviderGroq, []string{ProviderGroq, ProviderGemini, ProviderGoogle}},
		{"unknown", []string{ProviderGoogle, ProviderGemini, ProviderGroq}},
	}

	for _, tt := range tests {
		got := buildFallbackChain(tt.primary)
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("buildFallbackChain(%q) = %v, want %v", tt.primary, got, tt.want)
		}
	}
}

func TestParseNumberedLines(t *testing.T) {
	raw := `1. Wow! Today's performance was legendary!
2. So sweet! My heart skipped a beat.
3. Unnie, you look absolutely gorgeous!`

	res, err := parseNumberedLines(raw, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(res))
	}
	if res[0] != "Wow! Today's performance was legendary!" {
		t.Errorf("line 1 mismatch: %q", res[0])
	}
	if res[1] != "So sweet! My heart skipped a beat." {
		t.Errorf("line 2 mismatch: %q", res[1])
	}
	if res[2] != "Unnie, you look absolutely gorgeous!" {
		t.Errorf("line 3 mismatch: %q", res[2])
	}
}

func TestParseNumberedLinesWithMarkdownAndParens(t *testing.T) {
	raw := `**1.** Wow! Today's performance was legendary!
2) So sweet!
3. Unnie, you are beautiful!`

	res, err := parseNumberedLines(raw, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(res))
	}
	if res[0] != "Wow! Today's performance was legendary!" {
		t.Errorf("line 1 mismatch: %q", res[0])
	}
	if res[1] != "So sweet!" {
		t.Errorf("line 2 mismatch: %q", res[1])
	}
	if res[2] != "Unnie, you are beautiful!" {
		t.Errorf("line 3 mismatch: %q", res[2])
	}
}

func TestPreserveSpeakerTags(t *testing.T) {
	orig := []string{
		"[SUI] Pop off pop off",
		"(CHORUS) Yeah we fly high",
		"Regular line without speaker tag",
		"[수이] 안녕하세요",
		"KiiiKiii: Let's dance",
	}

	trans := []string{
		"Pop off, pop off",
		"Yeah, we are flying high",
		"Regular line without speaker",
		"Hello",
		"Let's dance",
	}

	got := preserveSpeakerTags(orig, trans)

	if got[0] != "[SUI] Pop off, pop off" {
		t.Errorf("expected [SUI] preserved, got: %q", got[0])
	}
	if got[1] != "(CHORUS) Yeah, we are flying high" {
		t.Errorf("expected (CHORUS) preserved, got: %q", got[1])
	}
	if got[2] != "Regular line without speaker" {
		t.Errorf("expected no tag added, got: %q", got[2])
	}
	if got[3] != "[수이] Hello" {
		t.Errorf("expected [수이] preserved, got: %q", got[3])
	}
	if got[4] != "KiiiKiii: Let's dance" {
		t.Errorf("expected KiiiKiii: preserved, got: %q", got[4])
	}

	// If translation already preserved it, don't duplicate
	alreadyPreserved := []string{"[SUI] Pop off, pop off"}
	origSingle := []string{"[SUI] Pop off"}
	gotSingle := preserveSpeakerTags(origSingle, alreadyPreserved)
	if gotSingle[0] != "[SUI] Pop off, pop off" {
		t.Errorf("expected no duplicate [SUI], got: %q", gotSingle[0])
	}
}
