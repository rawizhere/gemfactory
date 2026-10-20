package translate

import (
	"testing"

	"github.com/stretchr/testify/require"
)

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

	require.Equal(t, "[SUI] Pop off, pop off", got[0], "expected [SUI] preserved")
	require.Equal(t, "(CHORUS) Yeah, we are flying high", got[1], "expected (CHORUS) preserved")
	require.Equal(t, "Regular line without speaker", got[2], "expected no tag added")
	require.Equal(t, "[수이] Hello", got[3], "expected [수이] preserved")
	require.Equal(t, "KiiiKiii: Let's dance", got[4], "expected KiiiKiii: preserved")

	// If translation already preserved it, don't duplicate
	alreadyPreserved := []string{"[SUI] Pop off, pop off"}
	origSingle := []string{"[SUI] Pop off"}
	gotSingle := preserveSpeakerTags(origSingle, alreadyPreserved)
	require.Equal(t, "[SUI] Pop off, pop off", gotSingle[0], "expected no duplicate [SUI]")
}

func TestSanitizeSubtitleTypography(t *testing.T) {
	in := "Кэнди\u2011пинк магический флип\u2011фон \u00A0 \u200B тест\u2013слово ‘кавычки’ “двойные”"
	want := "Кэнди-пинк магический флип-фон    тест-слово 'кавычки' \"двойные\""
	require.Equal(t, want, SanitizeTypography(in), "SanitizeTypography(%q)", in)
}

func TestSanitizeKeepsEmojiSequences(t *testing.T) {
	in := "😂 👨‍👩‍👧 ✈️ ♪"
	require.Equal(t, in, SanitizeTypography(in), "emoji sequences must stay unchanged")
}

func TestContextOnlyCaption(t *testing.T) {
	cases := map[string]bool{
		"[신기]":             true,
		"[작고 소중한 비눗방울 전달]": true,
		"Wow!":             true,
		"-What?!":          true,
		"...":              true,
		"":                 true,
		"우와!":              false,
		"Обычная реплика":  false,
		"Give me, give me": false,
		"[설명] обычный текст в скобках": false,
	}
	for in, want := range cases {
		require.Equal(t, want, contextOnlyCaption(in), "contextOnlyCaption(%q)", in)
	}
}

func TestLinePassesTargetContextCues(t *testing.T) {
	require.True(t, linePassesTarget("[작고 소중한 비눗방울 전달]", "[작고 소중한 비눗방울 전달]", "ru"), "bracketed caption kept as-is must pass")
	require.True(t, linePassesTarget("우와!", "Wow!", "ru"), "interjection may stay Latin")
	require.False(t, linePassesTarget("안녕하세요", "안녕하세요", "ru"), "regular untranslated line must fail")
}

func TestParseJSONLines(t *testing.T) {
	res, err := parseJSONLines(`["Привет", "Пока ♪"]`, 2)
	require.NoError(t, err)
	require.Equal(t, []string{"Привет", "Пока ♪"}, res)

	res, err = parseJSONLines("```json\n[\"Да\"]\n```", 1)
	require.NoError(t, err)
	require.Equal(t, []string{"Да"}, res)

	_, err = parseJSONLines(`["Привет"]`, 2)
	require.Error(t, err, "count mismatch")
	_, err = parseJSONLines(`["Привет", " "]`, 2)
	require.Error(t, err, "empty line")
	_, err = parseJSONLines(`not json`, 1)
	require.Error(t, err)
}

func TestLinePassesTargetDroppedAndNames(t *testing.T) {
	require.False(t, linePassesTarget("Hello there", "", "ru"), "dropped line must fail")
	require.True(t, linePassesTarget("트와이스!", "TWICE!", "ru"), "group name kept in Latin")
	require.True(t, linePassesTarget("MBTI?", "MBTI?", "ru"), "acronym kept in Latin")
	require.True(t, linePassesTarget("aespa", "aespa", "ru"), "single lowercase name unchanged")
	require.False(t, linePassesTarget("Thank you so much", "Thank you so much", "ru"), "untranslated sentence must fail")
}

func TestTextLooksTarget(t *testing.T) {
	require.True(t, TextLooksTarget([]string{"Привет", "Как дела?"}, "ru"))
	require.False(t, TextLooksTarget([]string{"Hello", "How are you?"}, "ru"))
	require.False(t, TextLooksTarget(nil, "ru"))
}

func TestCheckTarget(t *testing.T) {
	require.NoError(t, checkTarget([]string{"Hello there", "Bye now"}, []string{"Привет", "Пока"}, "ru"))
	src := []string{"Thank you so much", "See you tomorrow"}
	require.ErrorContains(t, checkTarget(src, src, "ru"), "2 of 2 lines rejected")
	require.Error(t, checkTarget(nil, nil, "ru"), "empty output must fail")
}
