// Package format holds small human-readable formatting helpers.
package format

import "fmt"

// Bytes renders n in binary units: "512 B", "1.5 KB", "25.4 MB", "1.2 GB".
// Kilobyte values of 10 and above are shown without decimals.
func Bytes(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	val, unit := float64(n)/1024, "KB"
	for _, next := range []string{"MB", "GB", "TB"} {
		if val < 1024 {
			break
		}
		val, unit = val/1024, next
	}
	if unit == "KB" && val >= 10 {
		return fmt.Sprintf("%.0f %s", val, unit)
	}
	return fmt.Sprintf("%.1f %s", val, unit)
}
