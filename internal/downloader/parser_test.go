package downloader

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseClipArgsBasic(t *testing.T) {
	p, err := ParseClipArgs([]string{"https://www.youtube.com/watch?v=X56FLo6qslE", "0:26", "0:32"})
	require.NoError(t, err)
	require.Equal(t, "https://www.youtube.com/watch?v=X56FLo6qslE", p.URL)
	require.Len(t, p.Intervals, 1)
	require.Equal(t, "0:26", p.Intervals[0].Start)
	require.Equal(t, "0:32", p.Intervals[0].End)
	require.Empty(t, p.SubsLang)
	require.False(t, p.HQ, "unexpected flags: %+v", p)
	require.False(t, p.GIF)
	require.False(t, p.Shorts)
}

func TestParseClipArgsMultipleIntervalsAndLang(t *testing.T) {
	p, err := ParseClipArgs([]string{
		"https://www.youtube.com/watch?v=r0u5URS3VXE",
		"4:00", "4:01", "4:02", "4:04", "ru",
	})
	require.NoError(t, err)
	require.Len(t, p.Intervals, 2, "intervals = %+v", p.Intervals)
	require.Equal(t, "4:00", p.Intervals[0].Start, "intervals = %+v", p.Intervals)
	require.Equal(t, "4:01", p.Intervals[0].End, "intervals = %+v", p.Intervals)
	require.Equal(t, "4:02", p.Intervals[1].Start, "intervals = %+v", p.Intervals)
	require.Equal(t, "4:04", p.Intervals[1].End, "intervals = %+v", p.Intervals)
	require.Equal(t, "ru", p.SubsLang)
}

func TestParseClipArgsSecondsMsAndFlags(t *testing.T) {
	p, err := ParseClipArgs([]string{
		"https://youtu.be/DaLioUGhHZo", "855.44", "872.40", "hq", "meta",
	})
	require.NoError(t, err)
	require.True(t, p.HQ, "flags not parsed: %+v", p)
	require.True(t, p.Meta, "flags not parsed: %+v", p)
	require.Equal(t, "855.44", p.Intervals[0].Start, "intervals = %+v", p.Intervals)
	require.Equal(t, "872.40", p.Intervals[0].End, "intervals = %+v", p.Intervals)
}

func TestParseClipArgsShorts(t *testing.T) {
	p, err := ParseClipArgs([]string{"https://www.youtube.com/shorts/abc123XYZ_-"})
	require.NoError(t, err)
	require.True(t, p.Shorts, "shorts URL not detected")
	require.Len(t, p.Intervals, 1, "expected 1 empty interval for full short, got %+v", p.Intervals)
	require.Empty(t, p.Intervals[0].Start, "expected 1 empty interval for full short, got %+v", p.Intervals)
}

func TestParseClipArgsShortsWithIntervals(t *testing.T) {
	p, err := ParseClipArgs([]string{"https://www.youtube.com/shorts/abc123XYZ_-", "0:10", "0:25"})
	require.NoError(t, err)
	require.False(t, p.Shorts, "expected Shorts to be false when intervals are provided")
	require.Len(t, p.Intervals, 1, "expected 1 interval (0:10 - 0:25), got %+v", p.Intervals)
	require.Equal(t, "0:10", p.Intervals[0].Start, "expected 1 interval (0:10 - 0:25), got %+v", p.Intervals)
	require.Equal(t, "0:25", p.Intervals[0].End, "expected 1 interval (0:10 - 0:25), got %+v", p.Intervals)
}

func TestParseClipArgsTikTok(t *testing.T) {
	p, err := ParseClipArgs([]string{"https://www.tiktok.com/@user/video/7123456789012345678"})
	require.NoError(t, err)
	require.True(t, p.Shorts, "tiktok URL not detected as direct full video")
	require.Len(t, p.Intervals, 1, "expected 1 empty interval for full tiktok video, got %+v", p.Intervals)
	require.Empty(t, p.Intervals[0].Start, "expected 1 empty interval for full tiktok video, got %+v", p.Intervals)
}

func TestParseClipArgsIgnoresExtraURLTokens(t *testing.T) {
	p, err := ParseClipArgs([]string{
		"https://www.youtube.com/watch?v=VFrd1c8IrmA",
		"(https://www.youtube.com/watch?v=VFrd1c8IrmA&t=53)",
		"0:53.6", "1:01", "(https://youtu.be/VFrd1c8IrmA?t=61)",
	})
	require.NoError(t, err)
	require.Equal(t, "https://www.youtube.com/watch?v=VFrd1c8IrmA", p.URL)
	require.Len(t, p.Intervals, 1, "intervals = %+v", p.Intervals)
	require.Equal(t, "0:53.6", p.Intervals[0].Start, "intervals = %+v", p.Intervals)
	require.Equal(t, "1:01", p.Intervals[0].End, "intervals = %+v", p.Intervals)
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
		_, err := ParseClipArgs(args)
		require.Error(t, err, "case %d: expected error for %v", i, args)
	}
}

func TestYTDLPProgressRegex(t *testing.T) {
	line := "[download]  45.2% of ~  25.40MiB at   4.12MiB/s ETA 00:03"
	m := ytDlpProgressRe.FindStringSubmatch(line)
	require.GreaterOrEqual(t, len(m), 5, "expected at least 5 matches, got %d", len(m))
	require.Equal(t, "45.2", m[1], "pct mismatch")
	require.Equal(t, "25.40MiB", m[2], "size mismatch")
	require.Equal(t, "4.12MiB/s", m[3], "speed mismatch")
	require.Equal(t, "00:03", m[4], "eta mismatch")
}

func TestFFmpegProgressRegex(t *testing.T) {
	outTimeLine := "out_time_us=4000000"
	m := ffmpegOutTimeRe.FindStringSubmatch(outTimeLine)
	require.GreaterOrEqual(t, len(m), 2, "out_time_us match failed: %+v", m)
	require.Equal(t, "4000000", m[1], "out_time_us match failed: %+v", m)

	speedLine := "speed= 2.45x"
	sm := ffmpegSpeedRe.FindStringSubmatch(speedLine)
	require.GreaterOrEqual(t, len(sm), 2, "speed match failed: %+v", sm)
	require.Equal(t, "2.45x", sm[1], "speed match failed: %+v", sm)

	sectionLine := "frame=  150 fps=0.0 q=-1.0 size=    2048kB time=00:00:06.00 bitrate=2796.2kbits/s speed=11.8x"
	tm := ffmpegSectionTimeRe.FindStringSubmatch(sectionLine)
	require.GreaterOrEqual(t, len(tm), 2, "section time match failed: %+v", tm)
	require.Equal(t, "00:00:06.00", tm[1], "section time match failed: %+v", tm)

	sm2 := ffmpegSectionSizeRe.FindStringSubmatch(sectionLine)
	require.GreaterOrEqual(t, len(sm2), 2, "section size match failed: %+v", sm2)
	require.Equal(t, "2048kB", sm2[1], "section size match failed: %+v", sm2)

	spm := ffmpegSectionSpeedRe.FindStringSubmatch(sectionLine)
	require.GreaterOrEqual(t, len(spm), 2, "section speed match failed: %+v", spm)
	require.Equal(t, "11.8x", spm[1], "section speed match failed: %+v", spm)
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
		require.NoError(t, err, "ParseTimecode(%q)", c.in)
		require.LessOrEqual(t, math.Abs(got-c.want), 0.001,
			"ParseTimecode(%q) = %v ms, want %v ms", c.in, got, c.want)
	}
}

func TestParseTimecodeErrors(t *testing.T) {
	for _, in := range []string{"", "abc", "1:2:3:4", "-5", "1:", "1:-2"} {
		_, err := ParseTimecode(in)
		require.Error(t, err, "ParseTimecode(%q) expected error, got none", in)
	}
}

func TestFormatTimecode(t *testing.T) {
	require.Equal(t, "00:06:05.900", FormatTimecode(365900))
}

func TestFileNameWithTimecode(t *testing.T) {
	require.Equal(t, "dQw4w9WgXcQ_000605-900-000616-100", fileNameWithTimecodeVariant("dQw4w9WgXcQ", 365900, 376100, ""))
}

func TestFormatHumanSize(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"7168kB", "7.0 MB"},
		{"7168KiB", "7.0 MB"},
		{"512kB", "512 KB"},
		{"25.40MiB", "25.4 MB"},
		{"1024MiB", "1.0 GB"},
		{"", ""},
	}
	for _, c := range cases {
		require.Equal(t, c.want, FormatHumanSize(c.in), "FormatHumanSize(%q)", c.in)
	}
}
