package translate

import (
	"strings"
	"testing"

	"github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/require"
)

func TestIsHeavyReasoningModel(t *testing.T) {
	heavy := []string{"stepfun-ai/step-3.7-flash", "hy3-free", "deepseek-v4-flash", "nvidia/nemotron-3.5-lightning:free", "deepseek/deepseek-reasoner"}
	for _, m := range heavy {
		require.True(t, isHeavyReasoningModel(m), "isHeavyReasoningModel(%q) must be true", m)
	}
	calm := []string{"gemini-flash", "openai/gpt-oss-120b", "minimaxai/minimax-m3", "groq/compound", "x-preview-f-free"}
	for _, m := range calm {
		require.False(t, isHeavyReasoningModel(m), "must leave working models untouched")
	}
}

func TestMaxOutputTokensHeavy(t *testing.T) {
	require.Equal(t, reasoningBudgetCap, maxOutputTokens(10, "deepseek-v4-flash"), "heavy reasoning budget")
	require.Equal(t, reasoningBudgetCap, maxOutputTokens(400, "stepfun-ai/step-3.7-flash"), "heavy budget ignores batch size")
	require.Equal(t, 1024, maxOutputTokens(10, "gemini-flash"))
	require.Equal(t, 10*60+256+2048, maxOutputTokens(10, "openai/gpt-oss-120b"))
}

func TestStripReasoningPreamble(t *testing.T) {
	withReasoning := "Here's a thinking process:\nLet me reason about this.\n1. Привет\n2. Мир"
	out := stripReasoningPreamble(withReasoning)
	require.True(t, strings.HasPrefix(out, "1. Привет"), "must keep from first numbered line, got %q", out)
	require.False(t, strings.Contains(out, "thinking process"), "must drop reasoning preamble")

	clean := "1. Перевод один\n2. Перевод два"
	require.Equal(t, clean, stripReasoningPreamble(clean), "clean numbered output unchanged")

	numeric := "no numbers here at all"
	require.Equal(t, numeric, stripReasoningPreamble(numeric), "no numbered line keeps whole text")
}

func TestHasReasoningTokens(t *testing.T) {
	inContent := openai.ChatCompletionResponse{
		Choices: []openai.ChatCompletionChoice{{Message: openai.ChatCompletionMessage{ReasoningContent: "thinking"}}},
	}
	require.True(t, hasReasoningTokens(inContent), "reasoning_content must count")

	inUsage := openai.ChatCompletionResponse{
		Usage: openai.Usage{CompletionTokensDetails: &openai.CompletionTokensDetails{ReasoningTokens: 7}},
	}
	require.True(t, hasReasoningTokens(inUsage), "usage reasoning_tokens must count")

	none := openai.ChatCompletionResponse{
		Choices: []openai.ChatCompletionChoice{{Message: openai.ChatCompletionMessage{Content: "hi"}}},
	}
	require.False(t, hasReasoningTokens(none), "plain content has no reasoning")
}

func TestNextReasoningBudget(t *testing.T) {
	resp := openai.ChatCompletionResponse{Usage: openai.Usage{CompletionTokensDetails: &openai.CompletionTokensDetails{ReasoningTokens: 12000}}}
	require.Equal(t, 14048, nextReasoningBudget(resp, 12000, 10))

	require.Equal(t, 16000, nextReasoningBudget(openai.ChatCompletionResponse{}, 8000, 10))

	big := openai.ChatCompletionResponse{Usage: openai.Usage{CompletionTokensDetails: &openai.CompletionTokensDetails{ReasoningTokens: 10000}}}
	require.Equal(t, reasoningBudgetMax, nextReasoningBudget(big, 32000, 10))
}

func TestReasoningDisableKwargs(t *testing.T) {
	require.NotNil(t, reasoningDisableKwargs(ProviderOpencode, "nemotron-3-ultra-free"))
	require.NotNil(t, reasoningDisableKwargs(ProviderOpencode, "laguna-x"))

	require.Nil(t, reasoningDisableKwargs(ProviderOpencode, "deepseek-v4-flash"))
	require.Nil(t, reasoningDisableKwargs(ProviderNvidia, "minimaxai/minimax-m3"))
	require.Nil(t, reasoningDisableKwargs(ProviderGroq, "groq/compound"))
}
