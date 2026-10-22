package translate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"google.golang.org/genai"
)

// maxRetryDelay caps how long a rate-limited request waits before its single retry;
// a longer server-suggested delay means the daily quota is gone, so the call fails fast.
const maxRetryDelay = 30 * time.Second

func newClient(ctx context.Context, key string, timeout time.Duration) (*genai.Client, error) {
	if key == "" {
		return nil, errors.New("GEMINI_API_KEY is not set")
	}
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	return genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:     key,
		Backend:    genai.BackendGeminiAPI,
		HTTPClient: &http.Client{Timeout: timeout},
	})
}

// baseConfig disables thinking and safety blocking: subtitles keep the original swearing and need no reasoning.
func baseConfig(system string, temperature float32) *genai.GenerateContentConfig {
	budget := int32(0)
	return &genai.GenerateContentConfig{
		SystemInstruction: &genai.Content{Parts: []*genai.Part{genai.NewPartFromText(system)}},
		Temperature:       &temperature,
		ThinkingConfig:    &genai.ThinkingConfig{ThinkingBudget: &budget},
		SafetySettings:    safetyOff(),
	}
}

func safetyOff() []*genai.SafetySetting {
	var out []*genai.SafetySetting
	for _, c := range []genai.HarmCategory{
		genai.HarmCategoryHarassment,
		genai.HarmCategoryHateSpeech,
		genai.HarmCategorySexuallyExplicit,
		genai.HarmCategoryDangerousContent,
	} {
		out = append(out, &genai.SafetySetting{Category: c, Threshold: genai.HarmBlockThresholdBlockNone})
	}
	return out
}

// generate sends one request. A 400 drops the next optional setting the model may reject
// (thinking off, response schema, safety); a rate limit or server error is retried once.
func generate(ctx context.Context, client *genai.Client, model string, cfg *genai.GenerateContentConfig, user string) (string, error) {
	retried := false
	for {
		resp, err := client.Models.GenerateContent(ctx, model, genai.Text(user), cfg)
		if err == nil {
			if out := strings.TrimSpace(resp.Text()); out != "" {
				return out, nil
			}
			return "", fmt.Errorf("empty response%s", finishInfo(resp))
		}
		var apiErr genai.APIError
		if !errors.As(err, &apiErr) {
			return "", err
		}
		if keyInvalid(apiErr) {
			return "", fmt.Errorf("API key rejected: %s", apiErr.Message)
		}
		if apiErr.Code == http.StatusBadRequest && dropOption(cfg) {
			continue
		}
		delay := retryDelay(apiErr)
		if retried || !retryable(apiErr.Code) || delay > maxRetryDelay {
			return "", fmt.Errorf("%d %s: %s", apiErr.Code, apiErr.Status, apiErr.Message)
		}
		retried = true
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(delay):
		}
	}
}

// dropOption removes the next optional setting; false when nothing is left to drop.
func dropOption(cfg *genai.GenerateContentConfig) bool {
	switch {
	case cfg.ThinkingConfig != nil:
		cfg.ThinkingConfig = nil
	case cfg.ResponseSchema != nil:
		cfg.ResponseSchema, cfg.ResponseMIMEType = nil, ""
	case cfg.SafetySettings != nil:
		cfg.SafetySettings = nil
	default:
		return false
	}
	return true
}

// keyInvalid reports Gemini's answer to a bad key: 400 with reason API_KEY_INVALID, or 401/403.
func keyInvalid(apiErr genai.APIError) bool {
	if apiErr.Code == http.StatusUnauthorized || apiErr.Code == http.StatusForbidden {
		return true
	}
	for _, d := range apiErr.Details {
		if r, _ := d["reason"].(string); r == "API_KEY_INVALID" {
			return true
		}
	}
	return false
}

func retryable(code int) bool {
	switch code {
	case http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	}
	return false
}

// retryDelay reads google.rpc.RetryInfo from the error details, defaulting to 5s.
func retryDelay(apiErr genai.APIError) time.Duration {
	for _, d := range apiErr.Details {
		if t, _ := d["@type"].(string); strings.HasSuffix(t, "RetryInfo") {
			if s, _ := d["retryDelay"].(string); s != "" {
				if v, err := time.ParseDuration(s); err == nil {
					return v
				}
			}
		}
	}
	return 5 * time.Second
}

func finishInfo(resp *genai.GenerateContentResponse) string {
	if resp.PromptFeedback != nil && resp.PromptFeedback.BlockReason != "" {
		return fmt.Sprintf(" (blocked: %s)", resp.PromptFeedback.BlockReason)
	}
	if len(resp.Candidates) > 0 && resp.Candidates[0].FinishReason != "" {
		return fmt.Sprintf(" (finish reason: %s)", resp.Candidates[0].FinishReason)
	}
	return ""
}

// Translate translates texts line by line with the configured model and rejects output that is not in targetLang.
func Translate(ctx context.Context, cfg Config, texts []string, targetLang, sourceLang, videoTitle string) ([]string, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	client, err := newClient(ctx, cfg.Key, cfg.Timeout)
	if err != nil {
		return nil, err
	}

	gc := baseConfig(translatorRole+"\n"+cfg.Prompt, 0.2)
	count := int64(len(texts))
	gc.ResponseMIMEType = "application/json"
	gc.ResponseSchema = &genai.Schema{
		Type:     genai.TypeArray,
		Items:    &genai.Schema{Type: genai.TypeString},
		MinItems: &count,
		MaxItems: &count,
	}

	out, err := generate(ctx, client, cfg.Model, gc, buildTranslateUserMsg(texts, targetLang, sourceLang, videoTitle))
	if err != nil {
		return nil, fmt.Errorf("gemini (%s): %w", cfg.Model, err)
	}
	res, err := parseJSONLines(out, len(texts))
	if err != nil {
		return nil, fmt.Errorf("gemini (%s): %w", cfg.Model, err)
	}
	if err := checkTarget(texts, res, targetLang); err != nil {
		return nil, fmt.Errorf("gemini (%s): %w", cfg.Model, err)
	}
	return preserveSpeakerTags(texts, res), nil
}

// Complete runs a free-form completion (Grok replies) with the configured model.
func Complete(ctx context.Context, cfg Config, systemPrompt, userMsg string, temperature float32) (string, error) {
	client, err := newClient(ctx, cfg.Key, cfg.Timeout)
	if err != nil {
		return "", err
	}
	gc := baseConfig(systemPrompt, temperature)
	out, err := generate(ctx, client, cfg.Model, gc, userMsg)
	if err != nil {
		return "", fmt.Errorf("gemini (%s): %w", cfg.Model, err)
	}
	return out, nil
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

// buildTranslateUserMsg carries the lines and the output-format rules; style rules come from the editable system prompt.
func buildTranslateUserMsg(texts []string, targetLang, sourceLang, videoTitle string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Translate the following subtitle lines into %s (BCP-47 code %q). These lines are a contiguous excerpt from one video, translate them consistently with each other.", langDisplayName(targetLang), targetLang)
	if l := strings.TrimSpace(sourceLang); l != "" {
		fmt.Fprintf(&sb, "\nSource language: %s (%s)", langDisplayName(l), l)
	}
	if t := strings.TrimSpace(videoTitle); t != "" {
		fmt.Fprintf(&sb, "\nVideo context / Title: %q", t)
	}
	fmt.Fprintf(&sb, "\nOutput plain UTF-8 text; never use \\uXXXX escapes.\nReturn a JSON array of exactly %d strings: the translation of each input line, in the same order, without line numbers. No line may be empty.\n\nInput lines:\n", len(texts))
	for i, t := range texts {
		fmt.Fprintf(&sb, "%d. %s\n", i+1, t)
	}
	return sb.String()
}
