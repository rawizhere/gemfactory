package translate

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"go.uber.org/zap"
)

// Chain walks providers in configured order; first validated output wins.
func Chain(ctx context.Context, texts []string, targetLang, sourceLang string, cfg Config, videoTitle string, log *zap.Logger, onAttempt func(string)) ([]string, string, error) {
	if len(texts) == 0 {
		return nil, "", nil
	}
	var lastErr error
	for _, provider := range BuildFallbackChain(cfg) {
		if cfg.Keys[provider] == "" {
			continue
		}
		translated, model, err := Translate(ctx, provider, cfg, texts, targetLang, sourceLang, videoTitle, onAttempt)
		if err == nil && TranslationLooksTarget(texts, translated, targetLang) {
			return preserveSpeakerTags(texts, translated), provider + "/" + model, nil
		}
		logTranslateFailure(log, provider, model, err, texts, translated, targetLang)
		lastErr = fmt.Errorf("%s (%s): %w", provider, modelOrEmpty(model), errIf(err))
	}

	if lastErr != nil {
		return nil, "", lastErr
	}
	return nil, "", fmt.Errorf("all translation providers failed or unconfigured")
}

// fireAttempt reports the provider/model about to be tried.
func fireAttempt(onAttempt func(string), provider, model string) {
	if onAttempt != nil && model != "" {
		onAttempt(provider + "/" + model)
	}
}

// logTranslateFailure logs a failed provider attempt; nil err means the output was rejected by validation.
func logTranslateFailure(log *zap.Logger, provider, model string, err error, src, res []string, targetLang string) {
	if err == nil {
		total, bad, samples := scoreValidation(src, res, targetLang)
		log.Debug("translation output rejected by language validation",
			zap.String("provider", provider),
			zap.String("model", modelOrEmpty(model)),
			zap.String("target_lang", targetLang),
			zap.Int("lines_total", total),
			zap.Int("lines_bad", bad),
			zap.Int("bad_threshold", total/4+1),
			zap.Strings("rejected_samples", samples),
			zap.String("full_output", truncate(strings.Join(res, "\n"), 8000)))
	}
	log.Warn("translation provider failed",
		zap.String("provider", provider),
		zap.String("model", modelOrEmpty(model)),
		zap.Error(errIf(err)))
}

// scoreValidation counts lines failing language validation and samples bad pairs for debug logs.
func scoreValidation(src, res []string, targetLang string) (total, bad int, samples []string) {
	const maxRejectedSamples = 5
	for i, out := range res {
		srcLine := ""
		if i < len(src) {
			srcLine = src[i]
		}
		if linePassesTarget(srcLine, out, targetLang) {
			continue
		}
		bad++
		if len(samples) < maxRejectedSamples {
			samples = append(samples,
				fmt.Sprintf("%d| %s -> %s", i+1, truncate(strings.TrimSpace(srcLine), 80), truncate(strings.TrimSpace(out), 120)))
		}
	}
	return len(res), bad, samples
}

func modelOrEmpty(m string) string {
	if m == "" {
		return "unknown"
	}
	return m
}

func errIf(err error) error {
	if err != nil {
		return err
	}
	return fmt.Errorf("output failed language validation")
}

// TranslationLooksTarget rejects responses that mostly stayed in the source language (refusals, leaked reasoning).
func TranslationLooksTarget(src, res []string, targetLang string) bool {
	if len(res) == 0 || len(src) == 0 {
		return false
	}
	bad := 0
	for i, line := range res {
		srcLine := ""
		if i < len(src) {
			srcLine = src[i]
		}
		if !linePassesTarget(srcLine, line, targetLang) {
			bad++
		}
	}
	return bad*4 <= len(res)
}

// TextLooksTarget reports whether already-translated lines (e.g. YouTube auto-translate) are mostly in targetLang.
func TextLooksTarget(lines []string, targetLang string) bool {
	return TranslationLooksTarget(make([]string, len(lines)), lines, targetLang)
}

// linePassesTarget checks one cue against target-language rules: Cyrillic for ru, no passthrough of the source line, no dropped lines.
func linePassesTarget(srcLine, line, targetLang string) bool {
	srcLine = speakerTagRe.ReplaceAllString(strings.TrimSpace(srcLine), "")
	body := speakerTagRe.ReplaceAllString(strings.TrimSpace(line), "")

	if body == "" {
		return srcLine == ""
	}
	if contextOnlyCaption(body) || keptNameLine(srcLine, body) {
		return true
	}

	if targetLang == "ru" {
		cyr := 0
		for _, r := range body {
			if unicode.Is(unicode.Cyrillic, r) {
				cyr++
			}
		}
		return cyr > 0 && !looksLikePassthrough(srcLine, body)
	}

	lat := false
	for _, r := range body {
		if r < 128 && unicode.IsLetter(r) {
			lat = true
			break
		}
	}
	return lat && !looksLikePassthrough(srcLine, body)
}

// contextOnlyCaption passes sound-effect and description cues like "[신기]" or "Wow!" that translation cannot improve.
func contextOnlyCaption(body string) bool {
	rest := strings.TrimSpace(bracketGroupRe.ReplaceAllString(body, ""))
	if rest == "" || !strings.ContainsFunc(rest, unicode.IsLetter) {
		return true
	}
	return interjectionRe.MatchString(rest)
}

// keptNameLine passes a line that is only a name or acronym the prompt keeps in Latin ("TWICE!", "MBTI?", "aespa").
func keptNameLine(srcLine, body string) bool {
	words := strings.FieldsFunc(body, isNotWordRune)
	if len(words) == 0 || len(words) > 3 {
		return false
	}
	allCaps := true
	for _, w := range words {
		for _, r := range w {
			if r >= 128 {
				return false
			}
		}
		if len([]rune(w)) < 2 || strings.ToUpper(w) != w {
			allCaps = false
		}
	}
	if allCaps {
		return true
	}
	srcWords := strings.FieldsFunc(srcLine, isNotWordRune)
	return len(words) == 1 && len(srcWords) == 1 && strings.EqualFold(words[0], srcWords[0])
}

func isNotWordRune(r rune) bool {
	return !unicode.IsLetter(r) && !unicode.IsDigit(r)
}

func looksLikePassthrough(srcLine, out string) bool {
	if srcLine == "" || out == "" {
		return false
	}
	srcWords := wordSet(srcLine)
	if len(srcWords) == 0 {
		return false
	}
	outLower := strings.ToLower(out)
	hit := 0
	for w := range srcWords {
		if strings.Contains(outLower, w) {
			hit++
		}
	}
	return hit*2 >= len(srcWords)
}

func wordSet(s string) map[string]struct{} {
	set := make(map[string]struct{})
	for _, w := range strings.Fields(strings.ToLower(s)) {
		w = strings.Trim(w, ".,!?\"'()[]")
		set[w] = struct{}{}
	}
	return set
}

// SanitizeTypography normalizes dashes, quotes and spaces, dropping zero-width chars that render as tofu boxes.
func SanitizeTypography(s string) string {
	var sb strings.Builder
	sb.Grow(len(s))
	for _, r := range s {
		switch r {
		case '\u2010', '\u2011', '\u2012', '\u2013', '\u2212', '\u00AD':
			sb.WriteByte('-')
		case '\u00A0', '\u2000', '\u2001', '\u2002', '\u2003', '\u2004', '\u2005', '\u2006', '\u2007', '\u2008', '\u2009', '\u200A', '\u202F', '\u205F', '\u3000':
			sb.WriteByte(' ')
		case '\u200B', '\u200C', '\uFEFF':
		case '\u2018', '\u2019', '\u201A', '\u201B':
			sb.WriteByte('\'')
		case '\u201C', '\u201D', '\u201E', '\u201F':
			sb.WriteByte('"')
		default:
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

func preserveSpeakerTags(original, translated []string) []string {
	out := make([]string, len(translated))
	for i, tr := range translated {
		tr = SanitizeTypography(tr)
		if i >= len(original) {
			out[i] = tr
			continue
		}
		orig := original[i]
		if match := speakerTagRe.FindString(orig); match != "" {
			tag := strings.TrimSpace(match)
			trTrim := strings.TrimSpace(tr)
			if !strings.HasPrefix(trTrim, tag) && !strings.HasPrefix(trTrim, "[") && !strings.HasPrefix(trTrim, "(") {
				out[i] = tag + " " + trTrim
			} else {
				out[i] = tr
			}
		} else {
			out[i] = tr
		}
	}
	return out
}

var speakerTagRe = regexp.MustCompile(`^(\[[^\]]+\]|\([^\)]+\)|[A-Za-z0-9_가-힣]+:)\s*`)
var bracketGroupRe = regexp.MustCompile(`\[[^]]*\]`)
var interjectionRe = regexp.MustCompile(`^[-–—!?.\s]*[A-Za-z]{1,5}[-–—!?.\s]*$`)

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "...(truncated)"
}
