// Package translate translates subtitles with Gemini and validates the output language.
package translate

import (
	"context"
	"strconv"
	"strings"
	"time"

	"gemfactory/internal/settings"
)

const translatorRole = "You are a professional video subtitle translator."

// Config holds the Gemini key and model, the translation prompt and the request timeout.
type Config struct {
	YouTubeOnly  bool // use YouTube auto-translate, skip Gemini
	Key          string
	Model        string
	Prompt       string
	SourcePrefRU []string // preferred source languages when translating into ru
	Timeout      time.Duration
}

func ParseCSV(s string) []string {
	parts := strings.Split(s, ",")
	var res []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			res = append(res, p)
		}
	}
	return res
}

// ResolveConfig reads the translation config with the DB -> env -> default precedence.
func ResolveConfig(ctx context.Context, s *settings.Provider) Config {
	return Config{
		YouTubeOnly:  s.Bool(ctx, settings.SubsYouTubeOnly),
		Key:          s.Value(ctx, settings.GeminiAPIKey),
		Model:        s.Value(ctx, settings.GeminiModel),
		Prompt:       s.Value(ctx, settings.TranslationPrompt),
		SourcePrefRU: ParseCSV(s.Value(ctx, settings.SubsSourcePrefRU)),
		Timeout:      parseTimeout(s.Value(ctx, settings.TranslationTimeout)),
	}
}

const defaultTimeout = 180 * time.Second

// parseTimeout accepts seconds (web panel) or a Go duration (env, e.g. "3m").
func parseTimeout(v string) time.Duration {
	if sec, err := strconv.Atoi(v); err == nil && sec > 0 {
		return time.Duration(sec) * time.Second
	}
	if d, err := time.ParseDuration(v); err == nil && d > 0 {
		return d
	}
	return defaultTimeout
}
