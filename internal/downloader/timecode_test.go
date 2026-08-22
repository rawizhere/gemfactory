package downloader

import (
	"math"
	"testing"
)

func TestParseTimecode(t *testing.T) {
	cases := []struct {
		in   string
		want float64 // milliseconds
	}{
		{"6:05.9", 365900},
		{"00:06:05.9", 365900},
		{"10", 10000},
		{"59.999", 59999},
		{"1:00", 60000},
		{"1:15:05", 4505000},
		{"0:0:05.277", 5277},
		{"01:00:00.000", 3600000},
	}
	for _, c := range cases {
		got, err := ParseTimecode(c.in)
		if err != nil {
			t.Errorf("ParseTimecode(%q) unexpected error: %v", c.in, err)
			continue
		}
		if math.Abs(got-c.want) > 0.001 {
			t.Errorf("ParseTimecode(%q) = %v ms, want %v ms", c.in, got, c.want)
		}
	}
}

func TestParseTimecodeErrors(t *testing.T) {
	for _, in := range []string{"", "abc", "1:2:3:4", "-5", "1:", "1:-2"} {
		if _, err := ParseTimecode(in); err == nil {
			t.Errorf("ParseTimecode(%q) expected error, got none", in)
		}
	}
}

func TestFormatTimecode(t *testing.T) {
	if got := FormatTimecode(365900); got != "00:06:05.900" {
		t.Errorf("FormatTimecode(365900) = %q, want 00:06:05.900", got)
	}
}

func TestFileNameWithTimecode(t *testing.T) {
	got := fileNameWithTimecode("dQw4w9WgXcQ", 365900, 376100)
	if got != "dQw4w9WgXcQ_000605-900-000616-100" {
		t.Errorf("unexpected name %q", got)
	}
}
