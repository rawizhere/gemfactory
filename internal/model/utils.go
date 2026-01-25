// Package model provides utility functions for data normalization.
package model

import (
	"strings"
)

// CachedUtils performs string cleaning with basic memoization.
type CachedUtils struct {
	cache map[string]string
}

// CleanText normalizes whitespace and removes control characters.
func (u *CachedUtils) CleanText(text string) string {
	if cached, exists := u.cache[text]; exists {
		return cached
	}

	cleaned := strings.TrimSpace(text)
	cleaned = strings.ReplaceAll(cleaned, "\n", " ")
	cleaned = strings.ReplaceAll(cleaned, "\t", " ")

	for strings.Contains(cleaned, "  ") {
		cleaned = strings.ReplaceAll(cleaned, "  ", " ")
	}

	u.cache[text] = cleaned
	return cleaned
}

// GetUtils initializes a new utility helper.
func GetUtils() *CachedUtils {
	return &CachedUtils{
		cache: make(map[string]string),
	}
}
