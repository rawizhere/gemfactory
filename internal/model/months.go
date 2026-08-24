package model

import "strings"

// Months is the lowercase English month names in calendar order (index 0 = January).
var Months = []string{
	"january", "february", "march", "april", "may", "june",
	"july", "august", "september", "october", "november", "december",
}

// MonthNumber returns the 1-based month number for a (case-insensitive) English
// month name, or 0 when the name is not recognized.
func MonthNumber(name string) int {
	n := strings.ToLower(strings.TrimSpace(name))
	for i, m := range Months {
		if n == m {
			return i + 1
		}
	}
	return 0
}
