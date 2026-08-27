package translate

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildFallbackChain(t *testing.T) {
	tests := []struct {
		cfg  Config
		want []string
	}{
		{Config{}, []string{ProviderOpenRouter, ProviderOpencode, ProviderNvidia, ProviderGroq}},
		{Config{FallbackOrder: []string{"nvidia", "gemini", "groq", "opencode"}}, []string{"nvidia", "gemini", "groq", "opencode"}},
		{Config{FallbackOrder: []string{"groq", "gemini"}}, []string{"groq", "gemini"}},
		{Config{FallbackOrder: []string{"openrouter", "gemini", "garbage", "groq"}}, []string{"openrouter", "gemini", "groq"}},
		{Config{GoogleOnly: true}, []string{ProviderGoogle}},
	}

	for _, tt := range tests {
		require.Equal(t, tt.want, BuildFallbackChain(tt.cfg), "BuildFallbackChain(%+v)", tt.cfg)
	}
}

func TestParseNumberedLines(t *testing.T) {
	raw := `1. Wow! Today's performance was legendary!
2. So sweet! My heart skipped a beat.
3. Unnie, you look absolutely gorgeous!`

	res, err := parseNumberedLines(raw, 3)
	require.NoError(t, err)
	require.Len(t, res, 3)
	require.Equal(t, "Wow! Today's performance was legendary!", res[0], "line 1 mismatch")
	require.Equal(t, "So sweet! My heart skipped a beat.", res[1], "line 2 mismatch")
	require.Equal(t, "Unnie, you look absolutely gorgeous!", res[2], "line 3 mismatch")
}

func TestParseNumberedLinesWithMarkdownAndParens(t *testing.T) {
	raw := `**1.** Wow! Today's performance was legendary!
2) So sweet!
3. Unnie, you are beautiful!`

	res, err := parseNumberedLines(raw, 3)
	require.NoError(t, err)
	require.Len(t, res, 3)
	require.Equal(t, "Wow! Today's performance was legendary!", res[0], "line 1 mismatch")
	require.Equal(t, "So sweet!", res[1], "line 2 mismatch")
	require.Equal(t, "Unnie, you are beautiful!", res[2], "line 3 mismatch")
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

func TestMaxOutputTokens(t *testing.T) {
	require.Equal(t, 1024, maxOutputTokens(10, "gemini-flash"), "small batch floor")
	require.Equal(t, 400*60+256, maxOutputTokens(400, "gemini-flash"), "scaled budget")
	require.Equal(t, 10*60+256+2048, maxOutputTokens(10, "openai/gpt-oss-120b"), "reasoning budget")
}

func TestIsReasoningModel(t *testing.T) {
	for _, m := range []string{"openai/gpt-oss-120b", "big-pickle", "minimaxai/minimax-m3"} {
		require.True(t, isReasoningModel(m), "isReasoningModel(%q) must be true", m)
	}
	require.False(t, isReasoningModel("gemini-3.7-flash"), "gemini must not be treated as a reasoning model")
}

func TestStripMarkdownBold(t *testing.T) {
	require.Equal(t, "Да. - Ой!", stripMarkdownBold("**Да.** - Ой!"))
	require.Equal(t, "a * b ** c", stripMarkdownBold("a * b ** c"), "lone asterisks must survive")
}

func TestBuildSystemInstruction(t *testing.T) {
	base := "rules"
	require.Equal(t, base, BuildSystemInstruction(base, "en"), "want base without addendum")
	require.Equal(t, base, BuildSystemInstruction(base, "ru"), "custom prompt must not append ruAddendum")
	require.Equal(t, ruAddendum, BuildSystemInstruction("", "ru"), "empty prompt with ru target must use ruAddendum")
	require.Equal(t, "", BuildSystemInstruction("", "en"), "empty prompt with non-ru target returns empty")
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
