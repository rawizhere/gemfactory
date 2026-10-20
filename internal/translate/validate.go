package translate

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// checkTarget fails when more than a quarter of the lines stayed out of targetLang (refusals, untranslated output).
func checkTarget(src, res []string, targetLang string) error {
	total, bad, samples := scoreValidation(src, res, targetLang)
	if total > 0 && bad*4 <= total {
		return nil
	}
	return fmt.Errorf("output failed %s language validation: %d of %d lines rejected: %s", targetLang, bad, total, strings.Join(samples, "; "))
}

// scoreValidation counts lines failing language validation and samples bad pairs for the error message.
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

// TextLooksTarget reports whether already-translated lines (e.g. YouTube auto-translate) are mostly in targetLang.
func TextLooksTarget(lines []string, targetLang string) bool {
	return checkTarget(make([]string, len(lines)), lines, targetLang) == nil
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
