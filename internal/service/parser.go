package service

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

// CurrentYear returns the current year as a string.
func CurrentYear() string {
	return time.Now().Format("2006")
}

// ParseReleaseDate attempts to extract a valid time.Time from various date string formats.
func ParseReleaseDate(dateStr string) (time.Time, error) {
	if dateStr == "" {
		return time.Time{}, fmt.Errorf("empty date string")
	}

	dateStr = strings.ReplaceAll(dateStr, ":", "")
	dateStr = strings.TrimSpace(dateStr)

	if strings.Contains(dateStr, " at ") {
		parts := strings.Split(dateStr, " at ")
		if len(parts) > 0 {
			dateStr = strings.TrimSpace(parts[0])
		}
	}

	return parseDate(dateStr)
}

func parseDate(dateStr string) (time.Time, error) {
	// Handle dot separators (DD.MM.YY).
	if strings.Contains(dateStr, ".") && len(strings.Split(dateStr, ".")) == 3 {
		parts := strings.Split(dateStr, ".")
		day := parts[0]
		month := parts[1]
		year := parts[2]

		if len(year) == 2 {
			yearInt, _ := strconv.Atoi(year)
			if yearInt <= 30 {
				year = "20" + year
			} else {
				year = "19" + year
			}
		}

		return time.Parse("2006-01-02", fmt.Sprintf("%s-%s-%s", year, month, day))
	}

	// Handle long-form month names.
	months := []string{
		"january", "february", "march", "april", "may", "june",
		"july", "august", "september", "october", "november", "december",
	}

	dateStrLower := strings.ToLower(dateStr)
	if !slices.ContainsFunc(months, func(m string) bool {
		return strings.Contains(dateStrLower, m)
	}) {
		return time.Time{}, fmt.Errorf("invalid date string: %s", dateStr)
	}

	parts := strings.Fields(strings.ReplaceAll(dateStr, ",", ""))
	if len(parts) < 2 {
		return time.Time{}, fmt.Errorf("invalid date format: %s", dateStr)
	}

	if len(parts) >= 3 && len(parts[2]) >= 2 {
		return time.Parse("January 2 2006", strings.Join(parts[:3], " "))
	}

	return time.Parse("January 2 2006", strings.Join(parts[:2], " ")+" "+CurrentYear())
}

// ParseReleaseTime extracts time.Time from string, handling bot patterns.
func ParseReleaseTime(timeStr string) (time.Time, error) {
	if timeStr == "" {
		return time.Time{}, fmt.Errorf("empty time string")
	}

	timeStr = strings.ReplaceAll(timeStr, "KST", "")
	timeStr = strings.TrimSpace(timeStr)

	if strings.Contains(timeStr, "at") {
		parts := strings.Split(timeStr, "at")
		if len(parts) > 1 {
			timeStr = strings.TrimSpace(parts[1])
		}
	}

	parsedTime, err := time.Parse("15:04", timeStr)
	if err != nil {
		parsedTime, err = time.Parse("3:04 PM", timeStr)
	}
	return parsedTime, err
}

// CleanReleaseTitle normalizes a release title by removing extra whitespace and decorative characters.
func CleanReleaseTitle(title string) string {
	title = strings.TrimSpace(title)
	title = strings.Trim(title, "{}")
	return strings.Join(strings.Fields(title), " ")
}

// CleanLink filters and normalizes links, removing invalid or unwanted YouTube URLs.
func CleanLink(link string) string {
	if link == "" {
		return ""
	}
	low := strings.ToLower(link)
	if strings.Contains(low, "youtube.com/@") ||
		strings.Contains(low, "youtube.com/channel/") ||
		strings.Contains(low, "youtube.com/user/") ||
		strings.Contains(low, "youtube.com/c/") {
		return ""
	}
	return link
}
