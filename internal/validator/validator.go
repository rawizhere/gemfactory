package validator

import (
	"regexp"
	"strings"
)

// ValidationError represents a single failure in data validation.
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	return e.Field + ": " + e.Message
}

// ValidationErrors encapsulates a group of multiple validation failures.
type ValidationErrors []ValidationError

func (e ValidationErrors) Error() string {
	if len(e) == 0 {
		return ""
	}

	var messages []string
	for _, err := range e {
		messages = append(messages, err.Error())
	}
	return strings.Join(messages, "; ")
}

// HasErrors checks if there are any validation errors.
func (e ValidationErrors) HasErrors() bool {
	return len(e) > 0
}

// ValidateRequired checks if the field is not empty.
func ValidateRequired(field, value string) error {
	if strings.TrimSpace(value) == "" {
		return ValidationError{Field: field, Message: "is required"}
	}
	return nil
}

// ValidateURL checks the URL format.
func ValidateURL(field, url string) error {
	if url == "" {
		return nil // URL is optional
	}
	if !urlRegex.MatchString(url) {
		return ValidationError{Field: field, Message: "invalid URL format"}
	}
	return nil
}

// Regular expressions for validation.
var (
	urlRegex = regexp.MustCompile(`^https?://[^\s/$.?#].[^\s]*$`)
)
