package downloader

import (
	"math"
	"testing"
)

func TestParseClipArgsBasic(t *testing.T) {
	p, err := ParseClipArgs([]string{"https://www.youtube.com/watch?v=X56FLo6qslE", "0:26", "0:32"})
	if err != nil {
		t.Fatal(err)
	}
	if p.URL != "https://www.youtube.com/watch?v=X56FLo6qslE" {
		t.Errorf("url = %q", p.URL)
	}
	if len(p.Intervals) != 1 || p.Intervals[0].Start != "0:26" || p.Intervals[0].End != "0:32" {
		t.Errorf("intervals = %+v", p.Intervals)
	}
	if p.SubsLang != "" || p.HQ || p.GIF || p.Shorts {
		t.Errorf("unexpected flags: %+v", p)
	}
}

func TestParseClipArgsMultipleIntervalsAndLang(t *testing.T) {
	p, err := ParseClipArgs([]string{
		"https://www.youtube.com/watch?v=r0u5URS3VXE",
		"4:00", "4:01", "4:02", "4:04", "ru",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Intervals) != 2 {
		t.Fatalf("want 2 intervals, got %d: %+v", len(p.Intervals), p.Intervals)
	}
	if p.Intervals[0].Start != "4:00" || p.Intervals[0].End != "4:01" ||
		p.Intervals[1].Start != "4:02" || p.Intervals[1].End != "4:04" {
		t.Errorf("intervals = %+v", p.Intervals)
	}
	if p.SubsLang != "ru" {
		t.Errorf("lang = %q", p.SubsLang)
	}
}

func TestParseClipArgsSecondsMsAndFlags(t *testing.T) {
	p, err := ParseClipArgs([]string{
		"https://youtu.be/DaLioUGhHZo", "855.44", "872.40", "hq", "meta",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !p.HQ || !p.Meta {
		t.Errorf("flags not parsed: %+v", p)
	}
	if p.Intervals[0].Start != "855.44" || p.Intervals[0].End != "872.40" {
		t.Errorf("intervals = %+v", p.Intervals)
	}
}

func TestParseClipArgsShorts(t *testing.T) {
	p, err := ParseClipArgs([]string{"https://www.youtube.com/shorts/abc123XYZ_-"})
	if err != nil {
		t.Fatal(err)
	}
	if !p.Shorts {
		t.Error("shorts URL not detected")
	}
	if len(p.Intervals) != 1 || p.Intervals[0].Start != "" {
		t.Errorf("expected 1 empty interval for full short, got %+v", p.Intervals)
	}
}

func TestParseClipArgsTikTok(t *testing.T) {
	p, err := ParseClipArgs([]string{"https://www.tiktok.com/@user/video/7123456789012345678"})
	if err != nil {
		t.Fatal(err)
	}
	if !p.Shorts {
		t.Error("tiktok URL not detected as direct full video")
	}
	if len(p.Intervals) != 1 || p.Intervals[0].Start != "" {
		t.Errorf("expected 1 empty interval for full tiktok video, got %+v", p.Intervals)
	}
}

func TestParseClipArgsIgnoresExtraURLTokens(t *testing.T) {
	p, err := ParseClipArgs([]string{
		"https://www.youtube.com/watch?v=VFrd1c8IrmA",
		"(https://www.youtube.com/watch?v=VFrd1c8IrmA&t=53)",
		"0:53.6", "1:01", "(https://youtu.be/VFrd1c8IrmA?t=61)",
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.URL != "https://www.youtube.com/watch?v=VFrd1c8IrmA" {
		t.Errorf("url = %q", p.URL)
	}
	if len(p.Intervals) != 1 || p.Intervals[0].Start != "0:53.6" || p.Intervals[0].End != "1:01" {
		t.Errorf("intervals = %+v", p.Intervals)
	}
}

func TestParseClipArgsErrors(t *testing.T) {
	cases := [][]string{
		{"0:26", "0:32"},                        // no url
		{"https://youtu.be/x", "5"},             // odd timecode count
		{"https://youtu.be/x"},                  // no intervals
		{"https://youtu.be/x", "0:32", "0:26"},  // end before start
		{"https://youtu.be/x", "bad 0:30 junk"}, // no valid pairs
	}
	for i, args := range cases {
		if _, err := ParseClipArgs(args); err == nil {
			t.Errorf("case %d: expected error for %v", i, args)
		}
	}
}

func TestYTDLPProgressRegex(t *testing.T) {
	line := "[download]  45.2% of ~  25.40MiB at   4.12MiB/s ETA 00:03"
	m := ytDlpProgressRe.FindStringSubmatch(line)
	if len(m) < 5 {
		t.Fatalf("expected at least 5 matches, got %d", len(m))
	}
	if m[1] != "45.2" {
		t.Errorf("pct = %q, want 45.2", m[1])
	}
	if m[2] != "25.40MiB" {
		t.Errorf("size = %q, want 25.40MiB", m[2])
	}
	if m[3] != "4.12MiB/s" {
		t.Errorf("speed = %q, want 4.12MiB/s", m[3])
	}
	if m[4] != "00:03" {
		t.Errorf("eta = %q, want 00:03", m[4])
	}
}

func TestFFmpegProgressRegex(t *testing.T) {
	outTimeLine := "out_time_us=4000000"
	m := ffmpegOutTimeRe.FindStringSubmatch(outTimeLine)
	if len(m) < 2 || m[1] != "4000000" {
		t.Errorf("out_time_us match failed: %+v", m)
	}

	speedLine := "speed= 2.45x"
	sm := ffmpegSpeedRe.FindStringSubmatch(speedLine)
	if len(sm) < 2 || sm[1] != "2.45x" {
		t.Errorf("speed match failed: %+v", sm)
	}

	sectionLine := "frame=  150 fps=0.0 q=-1.0 size=    2048kB time=00:00:06.00 bitrate=2796.2kbits/s speed=11.8x"
	tm := ffmpegSectionTimeRe.FindStringSubmatch(sectionLine)
	if len(tm) < 2 || tm[1] != "00:00:06.00" {
		t.Errorf("section time match failed: %+v", tm)
	}
	sm2 := ffmpegSectionSizeRe.FindStringSubmatch(sectionLine)
	if len(sm2) < 2 || sm2[1] != "2048kB" {
		t.Errorf("section size match failed: %+v", sm2)
	}
	spm := ffmpegSectionSpeedRe.FindStringSubmatch(sectionLine)
	if len(spm) < 2 || spm[1] != "11.8x" {
		t.Errorf("section speed match failed: %+v", spm)
	}
}

func TestParseTimecode(t *testing.T) {
	cases := []struct {
		in   string
		want float64
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
