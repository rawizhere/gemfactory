package translate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

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

// Translate runs provider's model chain from cfg over texts.
func Translate(ctx context.Context, provider string, cfg Config, texts []string, targetLang, sourceLang, videoTitle string, onAttempt func(string)) ([]string, string, error) {
	if provider == ProviderGemini {
		return translateGemini(ctx, texts, targetLang, sourceLang, cfg.Keys[provider], cfg.Prompt, videoTitle, cfg.Timeout, onAttempt, cfg.modelsFor(provider))
	}
	return translateOpenAICompatible(ctx, provider, providers[provider].baseURL, cfg.Keys[provider], cfg.modelsFor(provider), texts, targetLang, sourceLang, cfg.Prompt, videoTitle, cfg.Timeout, onAttempt)
}

func translateGemini(ctx context.Context, texts []string, targetLang, sourceLang, apiKey, systemInstruction, videoTitle string, timeout time.Duration, onAttempt func(string), geminiModels []string) ([]string, string, error) {
	jsonMsg := buildTranslateUserMsg(texts, targetLang, sourceLang, videoTitle, outputJSON)
	numberedMsg := buildTranslateUserMsg(texts, targetLang, sourceLang, videoTitle, outputNumbered)

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
		withSafety, withThinking, withSchema := true, true, true
		for attempt := 0; attempt < 4; attempt++ {
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
			userMsg := numberedMsg
			if withSchema {
				config.ResponseMIMEType = "application/json"
				config.ResponseSchema = linesSchema(len(texts))
				userMsg = jsonMsg
			}

			resp, cerr := client.Models.GenerateContent(ctx, model, genai.Text(userMsg), config)
			if cerr != nil {
				lastErr = fmt.Errorf("gemini (%s): %w", model, cerr)
				code, _ := geminiAPIErrorDetails(cerr)
				switch {
				case code == http.StatusBadRequest && withThinking:
					withThinking = false
					continue
				case code == http.StatusBadRequest && withSchema:
					withSchema = false
					continue
				case code == http.StatusBadRequest && withSafety:
					withSafety = false
					continue
				}
				break
			}

			outText := resp.Text()
			if strings.TrimSpace(outText) == "" || len(resp.Candidates) == 0 {
				lastErr = fmt.Errorf("empty response from gemini (%s)", model)
				break
			}

			var res []string
			var perr error
			if withSchema {
				res, perr = parseJSONLines(outText, len(texts))
			} else {
				res, perr = parseNumberedLines(stripMarkdownBold(thinkTagRe.ReplaceAllString(outText, "")), len(texts))
			}
			if perr == nil {
				return res, model, nil
			}
			lastErr = fmt.Errorf("gemini (%s): %w", model, perr)
			break
		}
	}

	return nil, "", lastErr
}

// linesSchema constrains Gemini output to exactly n translated strings.
func linesSchema(n int) *genai.Schema {
	count := int64(n)
	return &genai.Schema{
		Type:     genai.TypeArray,
		Items:    &genai.Schema{Type: genai.TypeString},
		MinItems: &count,
		MaxItems: &count,
	}
}

// parseJSONLines decodes a JSON array of translated lines; the count must match and no line may be empty.
func parseJSONLines(raw string, expectedCount int) ([]string, error) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(strings.TrimPrefix(raw, "```json"), "```")
	raw = strings.TrimSpace(strings.TrimSuffix(raw, "```"))
	var lines []string
	if err := json.Unmarshal([]byte(raw), &lines); err != nil {
		return nil, fmt.Errorf("invalid JSON lines: %w", err)
	}
	if len(lines) != expectedCount {
		return nil, fmt.Errorf("translated line count mismatch: expected %d, got %d", expectedCount, len(lines))
	}
	for i, l := range lines {
		l = SanitizeTypography(strings.TrimSpace(l))
		if l == "" {
			return nil, fmt.Errorf("translated line %d is empty", i+1)
		}
		lines[i] = l
	}
	return lines, nil
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

// requestTimeout falls back to the built-in default when the caller passes no explicit value.
func requestTimeout(timeout time.Duration) time.Duration {
	if timeout > 0 {
		return timeout
	}
	return defaultTimeout
}

// translateOpenAICompatible walks an OpenAI-compatible model chain until one produces parseable output.
func translateOpenAICompatible(ctx context.Context, provider, baseURL, apiKey string, models []string, texts []string, targetLang, sourceLang, systemInstruction, videoTitle string, timeout time.Duration, onAttempt func(string)) ([]string, string, error) {
	userMsg := buildTranslateUserMsg(texts, targetLang, sourceLang, videoTitle, outputNumbered)

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
	var err error
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return openai.ChatCompletionResponse{}, ctx.Err()
			case <-time.After(3 * time.Second):
			}
		}
		resp, err = client.CreateChatCompletion(ctx, req)
		if err == nil || !isRetryableOpenAIError(err) {
			return resp, err
		}
	}
	return openai.ChatCompletionResponse{}, err
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

// outputFormat selects how the model must return translated lines.
type outputFormat int

const (
	outputNumbered outputFormat = iota // "N. text" lines, for OpenAI-compatible providers
	outputJSON                         // JSON array of strings, enforced by Gemini's response schema
)

// buildTranslateUserMsg carries the lines and the output-format rules; style rules come from the editable system prompt.
func buildTranslateUserMsg(texts []string, targetLang, sourceLang, videoTitle string, format outputFormat) string {
	var context string
	if l := strings.TrimSpace(sourceLang); l != "" {
		context += fmt.Sprintf("\nSource language: %s (%s)", langDisplayName(l), l)
	}
	if t := strings.TrimSpace(videoTitle); t != "" {
		context += fmt.Sprintf("\nVideo context / Title: %q", t)
	}
	var output string
	switch format {
	case outputJSON:
		output = fmt.Sprintf("Return a JSON array of exactly %d strings: the translation of each input line, in the same order, without line numbers. No line may be empty.", len(texts))
	default:
		output = fmt.Sprintf("Return exactly %d numbered lines in the same order, keeping the line numbers (\"1. Translated text\"). Every line must be translated and non-empty. No preamble, explanations, markdown or code blocks.", len(texts))
	}
	return fmt.Sprintf(
		"Translate the following subtitle lines into %s (BCP-47 code %q). These lines are a contiguous excerpt from one video, translate them consistently with each other.%s\nOutput plain UTF-8 text; never use \\uXXXX escapes.\n%s\n\nInput lines:\n%s",
		langDisplayName(targetLang),
		targetLang,
		context,
		output,
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

	if foundCount > 0 {
		if missing := missingLines(result); len(missing) > 0 {
			return nil, fmt.Errorf("translated lines missing or empty: %v of %d", missing, expectedCount)
		}
		return result, nil
	}

	// No numbering at all: accept plain lines only when the count matches exactly.
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

// missingLines lists the 1-based numbers of lines the model left out or returned empty.
func missingLines(result []string) []int {
	var missing []int
	for i, r := range result {
		if r == "" {
			missing = append(missing, i+1)
		}
	}
	return missing
}

// Complete walks the fallback chain for a free-form chat completion.
func Complete(ctx context.Context, systemPrompt, userMsg string, cfg Config) (string, string, error) {
	var lastErr error
	for _, provider := range BuildFallbackChain(cfg) {
		key := cfg.Keys[provider]
		if key == "" {
			continue
		}
		var text, model string
		var err error
		if provider == ProviderGemini {
			text, model, err = completeGemini(ctx, key, cfg.modelsFor(provider), systemPrompt, userMsg, cfg.Timeout)
		} else {
			text, model, err = completeOpenAICompatible(ctx, provider, providers[provider].baseURL, key, cfg.modelsFor(provider), systemPrompt, userMsg, cfg.Timeout)
		}
		if err == nil && strings.TrimSpace(text) != "" {
			return text, provider + "/" + model, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return "", "", lastErr
	}
	return "", "", fmt.Errorf("all LLM providers failed or unconfigured")
}

func completeOpenAICompatible(ctx context.Context, provider, baseURL, apiKey string, models []string, systemPrompt, userMsg string, timeout time.Duration) (string, string, error) {
	cfg := openai.DefaultConfig(apiKey)
	cfg.BaseURL = baseURL
	cfg.HTTPClient = &http.Client{Timeout: requestTimeout(timeout)}
	client := openai.NewClientWithConfig(cfg)

	var lastErr error
	for _, model := range models {
		req := openai.ChatCompletionRequest{
			Model: model,
			Messages: []openai.ChatCompletionMessage{
				{Role: openai.ChatMessageRoleSystem, Content: systemPrompt},
				{Role: openai.ChatMessageRoleUser, Content: userMsg},
			},
			Temperature: 0.7,
		}
		if isReasoningModel(model) {
			req.ReasoningEffort = "low"
		}
		if kwargs := reasoningDisableKwargs(provider, model); kwargs != nil {
			req.ChatTemplateKwargs = kwargs
		}

		resp, err := createChatCompletionWithRetry(ctx, client, req)
		if err != nil {
			lastErr = fmt.Errorf("%s (%s): %w", provider, model, err)
			continue
		}

		if len(resp.Choices) > 0 {
			out := strings.TrimSpace(thinkTagRe.ReplaceAllString(resp.Choices[0].Message.Content, ""))
			if out != "" {
				return out, model, nil
			}
		}
		lastErr = fmt.Errorf("%s (%s): empty response", provider, model)
	}
	return "", "", lastErr
}

func completeGemini(ctx context.Context, apiKey string, geminiModels []string, systemPrompt, userMsg string, timeout time.Duration) (string, string, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:     apiKey,
		Backend:    genai.BackendGeminiAPI,
		HTTPClient: &http.Client{Timeout: requestTimeout(timeout)},
	})
	if err != nil {
		return "", "", fmt.Errorf("gemini client: %w", err)
	}

	temp := float32(0.7)
	var lastErr error
	for _, model := range geminiModels {
		withSafety, withThinking := true, true
		for attempt := 0; attempt < 3; attempt++ {
			config := &genai.GenerateContentConfig{
				SystemInstruction: &genai.Content{
					Parts: []*genai.Part{genai.NewPartFromText(systemPrompt)},
				},
				Temperature: &temp,
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
				code, _ := geminiAPIErrorDetails(cerr)
				switch {
				case code == http.StatusBadRequest && withThinking:
					withThinking = false
					continue
				case code == http.StatusBadRequest && withSafety:
					withSafety = false
					continue
				}
				break
			}

			out := strings.TrimSpace(thinkTagRe.ReplaceAllString(resp.Text(), ""))
			if out != "" {
				return out, model, nil
			}
			lastErr = fmt.Errorf("empty response from gemini (%s)", model)
			break
		}
	}
	return "", "", lastErr
}
