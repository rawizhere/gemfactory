package translate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/sashabaranov/go-openai"
	"google.golang.org/genai"
)

// thinkTagRe strips Qwen-style thinking tags.
var thinkTagRe = regexp.MustCompile(`(?s)<think>.*?</think>`)

// numberedLineStartRe matches a leading "N." / "N)" / "N:" translated line.
var numberedLineStartRe = regexp.MustCompile(`^\s*(?:\*{0,2})?\d+[.)\]:]`)

// stripReasoningPreamble cuts model reasoning from the body, keeping from the first numbered line.
func stripReasoningPreamble(s string) string {
	s = thinkTagRe.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if numberedLineStartRe.MatchString(strings.TrimSpace(line)) {
			return strings.Join(lines[i:], "\n")
		}
	}
	return s
}

var mdBoldRe = regexp.MustCompile(`\*\*([^*]+)\*\*`)

// stripMarkdownBold removes markdown bold markers without touching lone asterisks.
func stripMarkdownBold(s string) string {
	return mdBoldRe.ReplaceAllString(s, "$1")
}

func TranslateWithGoogle(ctx context.Context, texts []string, targetLang string) ([]string, error) {
	var out []string
	for _, batch := range batchByRuneLimit(texts, googleMaxQueryRunes) {
		part, err := translateWithGoogleBatch(ctx, batch, targetLang)
		if err != nil {
			return nil, err
		}
		out = append(out, part...)
	}
	return out, nil
}

// googleMaxQueryRunes keeps each GET query safely under the URL length limit.
const googleMaxQueryRunes = 1400

func batchByRuneLimit(texts []string, limit int) [][]string {
	var batches [][]string
	var cur []string
	size := 0
	for _, t := range texts {
		if size > 0 && size+len(t)+1 > limit {
			batches = append(batches, cur)
			cur = nil
			size = 0
		}
		cur = append(cur, t)
		size += len(t) + 1
	}
	if len(cur) > 0 {
		batches = append(batches, cur)
	}
	return batches
}

func translateWithGoogleBatch(ctx context.Context, texts []string, targetLang string) ([]string, error) {
	joined := strings.Join(texts, "\n")
	apiURL := fmt.Sprintf(
		"https://translate.googleapis.com/translate_a/single?client=gtx&sl=auto&tl=%s&dt=t&q=%s",
		url.QueryEscape(targetLang),
		url.QueryEscape(joined),
	)

	client := &http.Client{Timeout: 30 * time.Second}

	var body []byte
	err := retry.Do(
		func() error {
			req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
			if reqErr != nil {
				return retry.Unrecoverable(reqErr)
			}
			req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")

			resp, doErr := client.Do(req)
			if doErr != nil {
				return fmt.Errorf("google translate request failed: %w", doErr)
			}
			defer func() { _ = resp.Body.Close() }()

			b, readErr := io.ReadAll(resp.Body)
			if readErr != nil {
				return readErr
			}
			if resp.StatusCode == http.StatusOK {
				body = b
				return nil
			}
			statusErr := fmt.Errorf("google translate returned status %d: %s", resp.StatusCode, truncate(string(b), 300))
			if resp.StatusCode != http.StatusTooManyRequests && resp.StatusCode < 500 {
				return retry.Unrecoverable(statusErr)
			}
			return statusErr
		},
		retry.Context(ctx),
		retry.Attempts(4),
		retry.DelayType(retry.BackOffDelay),
		retry.Delay(3*time.Second),
		retry.LastErrorOnly(true),
	)
	if err != nil {
		return nil, err
	}

	var raw []interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse google translate response: %w", err)
	}

	if len(raw) == 0 {
		return nil, fmt.Errorf("empty google translate response")
	}

	segments, ok := raw[0].([]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected google translate response format")
	}

	var sb strings.Builder
	for _, seg := range segments {
		if pair, ok := seg.([]interface{}); ok && len(pair) > 0 {
			if str, ok := pair[0].(string); ok {
				sb.WriteString(str)
			}
		}
	}

	result := sb.String()
	lines := strings.Split(strings.ReplaceAll(result, "\r\n", "\n"), "\n")

	if len(lines) != len(texts) {
		lines = alignTranslatedLines(lines, len(texts))
	}

	return lines, nil
}

func alignTranslatedLines(lines []string, expectedLen int) []string {
	if len(lines) == expectedLen {
		return lines
	}
	out := make([]string, expectedLen)
	for i := range out {
		if i < len(lines) {
			out[i] = lines[i]
		}
	}
	return out
}

func TranslateWithGemini(ctx context.Context, texts []string, targetLang, sourceLang, apiKey, systemInstruction, videoTitle string, timeout time.Duration, onAttempt func(string), models ...[]string) ([]string, string, error) {
	geminiModels := firstModels(models, DefaultGeminiModels)
	userMsg := buildTranslateUserMsg(texts, targetLang, sourceLang, videoTitle)

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:     apiKey,
		Backend:    genai.BackendGeminiAPI,
		HTTPClient: &http.Client{Timeout: requestTimeout(timeout)},
	})
	if err != nil {
		return nil, "", fmt.Errorf("gemini client: %w", err)
	}

	temp := float32(0.2)

	var lastErr error
	for _, model := range geminiModels {
		fireAttempt(onAttempt, ProviderGemini, model)
		withSafety, withThinking := true, true
		for attempt := 0; attempt < 3; attempt++ {
			config := &genai.GenerateContentConfig{
				SystemInstruction: &genai.Content{
					Parts: []*genai.Part{genai.NewPartFromText(translatorRole + "\n" + systemInstruction)},
				},
				Temperature:     &temp,
				MaxOutputTokens: int32(maxOutputTokens(len(texts), model)),
			}
			if withThinking {
				budget := int32(0)
				config.ThinkingConfig = &genai.ThinkingConfig{ThinkingBudget: &budget}
			}
			if withSafety {
				config.SafetySettings = geminiSafetySettings()
			}

			resp, cerr := client.Models.GenerateContent(ctx, model, genai.Text(userMsg), config)
			if cerr != nil {
				lastErr = fmt.Errorf("gemini (%s): %w", model, cerr)
				code, msg := geminiAPIErrorDetails(cerr)
				switch {
				case code == http.StatusBadRequest && withSafety && strings.Contains(msg, "safety"):
					withSafety = false
					continue
				case code == http.StatusBadRequest && withThinking && strings.Contains(msg, "thinking"):
					withThinking = false
					continue
				}
				break
			}

			outText := resp.Text()
			if strings.TrimSpace(outText) == "" || len(resp.Candidates) == 0 {
				lastErr = fmt.Errorf("empty response from gemini (%s)", model)
				break
			}

			res, perr := parseNumberedLines(stripMarkdownBold(thinkTagRe.ReplaceAllString(outText, "")), len(texts))
			if perr == nil {
				return res, model, nil
			}
			lastErr = perr
			break
		}
	}

	return nil, "", lastErr
}

// geminiSafetySettings disables blocking across every harm category.
func geminiSafetySettings() []*genai.SafetySetting {
	categories := []genai.HarmCategory{
		genai.HarmCategoryHarassment,
		genai.HarmCategoryHateSpeech,
		genai.HarmCategorySexuallyExplicit,
		genai.HarmCategoryDangerousContent,
	}
	settings := make([]*genai.SafetySetting, 0, len(categories))
	for _, c := range categories {
		settings = append(settings, &genai.SafetySetting{Category: c, Threshold: genai.HarmBlockThresholdBlockNone})
	}
	return settings
}

// geminiAPIErrorDetails extracts the status code and lowercased message from a genai API error.
func geminiAPIErrorDetails(err error) (int, string) {
	var apiErr genai.APIError
	if errors.As(err, &apiErr) {
		return apiErr.Code, strings.ToLower(apiErr.Message)
	}
	return 0, strings.ToLower(err.Error())
}

// OpenAI-compatible endpoints used by the translation providers.
const (
	groqBaseURL       = "https://api.groq.com/openai/v1"
	opencodeBaseURL   = "https://opencode.ai/zen/v1"
	nvidiaBaseURL     = "https://integrate.api.nvidia.com/v1"
	openRouterBaseURL = "https://openrouter.ai/api/v1"
)

func TranslateWithGroq(ctx context.Context, texts []string, targetLang, sourceLang, apiKey, systemInstruction, videoTitle string, timeout time.Duration, onAttempt func(string), models ...[]string) ([]string, string, error) {
	return translateOpenAICompatible(ctx, ProviderGroq, groqBaseURL, apiKey, firstModels(models, DefaultGroqModels), texts, targetLang, sourceLang, systemInstruction, videoTitle, timeout, onAttempt)
}

// TranslateWithOpencode calls OpenCode Zen (OpenAI-compatible) with the given free-model chain.
func TranslateWithOpencode(ctx context.Context, texts []string, targetLang, sourceLang, apiKey, systemInstruction, videoTitle string, timeout time.Duration, onAttempt func(string), models ...[]string) ([]string, string, error) {
	return translateOpenAICompatible(ctx, ProviderOpencode, opencodeBaseURL, apiKey, firstModels(models, DefaultOpencodeModels), texts, targetLang, sourceLang, systemInstruction, videoTitle, timeout, onAttempt)
}

// TranslateWithNvidia calls NVIDIA NIM (OpenAI-compatible) with the given model chain.
func TranslateWithNvidia(ctx context.Context, texts []string, targetLang, sourceLang, apiKey, systemInstruction, videoTitle string, timeout time.Duration, onAttempt func(string), models ...[]string) ([]string, string, error) {
	return translateOpenAICompatible(ctx, ProviderNvidia, nvidiaBaseURL, apiKey, firstModels(models, DefaultNvidiaModels), texts, targetLang, sourceLang, systemInstruction, videoTitle, timeout, onAttempt)
}

// TranslateWithOpenRouter calls OpenRouter (OpenAI-compatible) with the given model chain.
func TranslateWithOpenRouter(ctx context.Context, texts []string, targetLang, sourceLang, apiKey, systemInstruction, videoTitle string, timeout time.Duration, onAttempt func(string), models ...[]string) ([]string, string, error) {
	return translateOpenAICompatible(ctx, ProviderOpenRouter, openRouterBaseURL, apiKey, firstModels(models, DefaultOpenRouterModels), texts, targetLang, sourceLang, systemInstruction, videoTitle, timeout, onAttempt)
}

// requestTimeout falls back to the built-in default when the caller passes no explicit value.
func requestTimeout(timeout time.Duration) time.Duration {
	if timeout > 0 {
		return timeout
	}
	return defaultTimeout()
}

// firstModels returns the caller-supplied model chain or the default one.
func firstModels(models []([]string), def []string) []string {
	if len(models) > 0 && len(models[0]) > 0 {
		return models[0]
	}
	return def
}

// translateOpenAICompatible walks an OpenAI-compatible model chain until one produces parseable output.
func translateOpenAICompatible(ctx context.Context, provider, baseURL, apiKey string, models []string, texts []string, targetLang, sourceLang, systemInstruction, videoTitle string, timeout time.Duration, onAttempt func(string)) ([]string, string, error) {
	userMsg := buildTranslateUserMsg(texts, targetLang, sourceLang, videoTitle)

	cfg := openai.DefaultConfig(apiKey)
	cfg.BaseURL = baseURL
	cfg.HTTPClient = &http.Client{Timeout: requestTimeout(timeout)}
	client := openai.NewClientWithConfig(cfg)

	var lastErr error
	for _, model := range models {
		fireAttempt(onAttempt, provider, model)

		initial := maxOutputTokens(len(texts), model)
		req := openai.ChatCompletionRequest{
			Model: model,
			Messages: []openai.ChatCompletionMessage{
				{Role: openai.ChatMessageRoleSystem, Content: translatorRole + "\n" + systemInstruction},
				{Role: openai.ChatMessageRoleUser, Content: userMsg},
			},
			Temperature: 0.2,
			// max_tokens (not MaxCompletionTokens): some providers reject the newer field.
			MaxTokens: initial, //nolint:staticcheck // SA1019
		}
		if isReasoningModel(model) {
			req.ReasoningEffort = "low"
		}
		if kwargs := reasoningDisableKwargs(provider, model); kwargs != nil {
			req.ChatTemplateKwargs = kwargs
		}

		budget := initial
		var resp openai.ChatCompletionResponse
		var err error
		for attempt := 0; attempt < reasoningRetryAttempts; attempt++ {
			req.MaxTokens = budget //nolint:staticcheck // SA1019
			resp, err = createChatCompletionWithRetry(ctx, client, req)
			if err != nil {
				lastErr = fmt.Errorf("%s (%s): %w", provider, model, err)
				break
			}

			outText := stripReasoningPreamble(resp.Choices[0].Message.Content)
			if strings.TrimSpace(outText) != "" {
				res, perr := parseNumberedLines(stripMarkdownBold(outText), len(texts))
				if perr == nil {
					return res, model, nil
				}
				lastErr = perr
				break
			}

			if hasReasoningTokens(resp) && attempt < reasoningRetryAttempts-1 {
				budget = nextReasoningBudget(resp, budget, len(texts))
				lastErr = fmt.Errorf("%s (%s): empty content with reasoning, budget -> %d", provider, model, budget)
				continue
			}
			lastErr = fmt.Errorf("%s (%s): empty response", provider, model)
			break
		}
	}

	return nil, "", lastErr
}

// createChatCompletionWithRetry sends the request, retrying once on 429/5xx.
func createChatCompletionWithRetry(ctx context.Context, client *openai.Client, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
	var resp openai.ChatCompletionResponse
	err := retry.Do(
		func() error {
			r, cerr := client.CreateChatCompletion(ctx, req)
			if cerr != nil {
				return cerr
			}
			resp = r
			return nil
		},
		retry.Context(ctx),
		retry.Attempts(2),
		retry.Delay(3*time.Second),
		retry.RetryIf(isRetryableOpenAIError),
		retry.LastErrorOnly(true),
	)
	if err != nil {
		return openai.ChatCompletionResponse{}, err
	}
	return resp, nil
}

// isRetryableOpenAIError reports whether the request should be retried (rate limits and server errors).
func isRetryableOpenAIError(err error) bool {
	var apiErr *openai.APIError
	return errors.As(err, &apiErr) &&
		(apiErr.HTTPStatusCode == http.StatusTooManyRequests || apiErr.HTTPStatusCode >= http.StatusInternalServerError)
}

func isReasoningModel(model string) bool {
	return strings.Contains(model, "gpt-oss") ||
		strings.Contains(model, "big-pickle") ||
		strings.Contains(model, "minimax")
}

// reasoningBudgetCap is the initial max_tokens for heavy reasoning models.
const reasoningBudgetCap = 24000

// reasoningBudgetMax caps the budget when retrying with a larger window.
const reasoningBudgetMax = 32768

// reasoningRetryAttempts bounds empty-content-with-reasoning retries per model.
const reasoningRetryAttempts = 3

// isHeavyReasoningModel reports models that burn the budget on hidden reasoning.
func isHeavyReasoningModel(model string) bool {
	m := strings.ToLower(model)
	return strings.Contains(m, "stepfun") ||
		strings.Contains(m, "step-3.7") ||
		strings.Contains(m, "hy3") ||
		strings.Contains(m, "deepseek-v4-flash") ||
		strings.Contains(m, "deepseek-reasoner") ||
		strings.Contains(m, "nemotron-3.5-lightning")
}

// hasReasoningTokens reports whether the response carried hidden reasoning.
func hasReasoningTokens(resp openai.ChatCompletionResponse) bool {
	if len(resp.Choices) > 0 && strings.TrimSpace(resp.Choices[0].Message.ReasoningContent) != "" {
		return true
	}
	if resp.Usage.CompletionTokensDetails != nil && resp.Usage.CompletionTokensDetails.ReasoningTokens > 0 {
		return true
	}
	return false
}

// nextReasoningBudget grows the budget from observed reasoning tokens, capped.
func nextReasoningBudget(resp openai.ChatCompletionResponse, cur, lines int) int {
	contentEstimate := lines*60 + 256
	var needed int
	if resp.Usage.CompletionTokensDetails != nil && resp.Usage.CompletionTokensDetails.ReasoningTokens > 0 {
		needed = resp.Usage.CompletionTokensDetails.ReasoningTokens + contentEstimate
	} else {
		needed = cur * 2
	}
	if needed < cur+2048 {
		needed = cur + 2048
	}
	if needed > reasoningBudgetMax {
		needed = reasoningBudgetMax
	}
	return needed
}

// reasoningDisableKwargs returns chat_template_kwargs to disable reasoning where safe.
func reasoningDisableKwargs(provider, model string) map[string]any {
	if provider != ProviderOpencode {
		return nil
	}
	m := strings.ToLower(model)
	if strings.Contains(m, "nemotron") || strings.Contains(m, "laguna") {
		return map[string]any{"enable_thinking": false}
	}
	return nil
}

// maxOutputTokens scales the budget with batch size; reasoning models get extra headroom.
func maxOutputTokens(lines int, model string) int {
	tokens := lines*60 + 256
	if isReasoningModel(model) {
		tokens += 2048
	}
	if isHeavyReasoningModel(model) {
		tokens = reasoningBudgetCap
	}
	if tokens < 1024 {
		return 1024
	}
	return tokens
}

func buildNumberedLines(texts []string) string {
	var sb strings.Builder
	for i, t := range texts {
		fmt.Fprintf(&sb, "%d. %s\n", i+1, t)
	}
	return sb.String()
}

var langNames = map[string]string{
	"en": "English", "ru": "Russian", "ko": "Korean", "ja": "Japanese",
	"zh": "Chinese", "es": "Spanish", "pt": "Portuguese", "id": "Indonesian",
}

func langDisplayName(code string) string {
	if n, ok := langNames[code]; ok {
		return n
	}
	return code
}

func buildTranslateUserMsg(texts []string, targetLang, sourceLang, videoTitle string) string {
	var context string
	if l := strings.TrimSpace(sourceLang); l != "" {
		context += fmt.Sprintf("\nSource language: %s (%s)", langDisplayName(l), l)
	}
	if t := strings.TrimSpace(videoTitle); t != "" {
		context += fmt.Sprintf("\nVideo context / Title: %q", t)
	}
	return fmt.Sprintf(
		"Translate the following subtitle lines into %s (BCP-47 code %q). These lines are a contiguous excerpt from one video, translate them consistently with each other.%s\nCopy every emoji and symbol (♪, ~, ★) from the input line into the output unchanged and in place; never replace an emoji with a word, a description or a bracket note. Output plain UTF-8 text; never use \\uXXXX escapes.\nReturn EXACTLY the same number of numbered lines as provided, keeping line numbers (\"1. Translated text\"). Do not add preamble or explanations.\n\nInput lines:\n%s",
		langDisplayName(targetLang),
		targetLang,
		context,
		buildNumberedLines(texts),
	)
}

func parseNumberedLines(raw string, expectedCount int) ([]string, error) {
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	result := make([]string, expectedCount)

	lineIdxRe := regexp.MustCompile(`^(?:\*{0,2})?(\d+)[.)\]:]\s*(?:\*{0,2})?\s*(.*)$`)

	foundCount := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		if m := lineIdxRe.FindStringSubmatch(trimmed); len(m) >= 3 {
			idx, err := strconv.Atoi(m[1])
			if err == nil && idx >= 1 && idx <= expectedCount {
				result[idx-1] = SanitizeTypography(strings.TrimSpace(m[2]))
				foundCount++
			}
		}
	}

	if foundCount >= expectedCount/2 && foundCount > 0 {
		for i, r := range result {
			if r == "" {
				result[i] = ""
			}
		}
		return result, nil
	}

	var nonNumbered []string
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			nonNumbered = append(nonNumbered, SanitizeTypography(strings.TrimSpace(l)))
		}
	}
	if len(nonNumbered) == expectedCount {
		return nonNumbered, nil
	}

	return nil, fmt.Errorf("translated line count mismatch: expected %d, got %d numbered lines", expectedCount, foundCount)
}
