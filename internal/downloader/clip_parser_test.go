package downloader

import "testing"

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
