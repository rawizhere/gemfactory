package handlers

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gemfactory/internal/downloader"
)

func TestRenderStatusCard(t *testing.T) {
	state := clipTaskState{
		url:      "https://www.youtube.com/watch?v=Ta42YSMVePU",
		title:    "Candy Pink Magic Hole Flip Phone (나 어떡해)",
		hashtags: []string{"#Kpop", "#girlgroup", "#Starshiptv", "#starship", "#뮤비"},
		interval: "1:05.7 – 1:25",
		mode:     "Subtitles (ru)",
		percent:  100,
		done:     true,
	}

	got := renderStatusCard(state)
	expected := "Done https://www.youtube.com/watch?v=Ta42YSMVePU\n\n" +
		"Candy Pink Magic Hole Flip Phone (나 어떡해)\n" +
		"<pre>#Kpop #girlgroup #Starshiptv #starship #뮤비</pre>\n" +
		"<code>1:05.7 – 1:25</code> • Subtitles (ru)\n\n" +
		"<code>[████████████████]</code> 100%"

	require.Equal(t, expected, got, "renderStatusCard mismatch")
}

func TestRenderStatusCardWorking(t *testing.T) {
	state := clipTaskState{
		url:        "https://www.youtube.com/watch?v=Ta42YSMVePU",
		title:      "Candy Pink Magic Hole Flip Phone (나 어떡해)",
		hashtags:   []string{"#Kpop", "#girlgroup"},
		interval:   "1:05.7 – 1:25",
		mode:       "Subtitles (ru)",
		percent:    50,
		stage:      "download",
		statusLine: "<b>Download:</b> 50%",
		done:       false,
	}

	got := renderStatusCard(state)
	expected := "Working https://www.youtube.com/watch?v=Ta42YSMVePU\n\n" +
		"Candy Pink Magic Hole Flip Phone (나 어떡해)\n" +
		"<pre>#Kpop #girlgroup</pre>\n" +
		"<code>1:05.7 – 1:25</code> • Subtitles (ru)\n\n" +
		"<code>[████████░░░░░░░░]</code> 50%\n" +
		"<b>Download:</b> 50%"

	require.Equal(t, expected, got, "renderStatusCard working mismatch")
}

func TestFormatMode(t *testing.T) {
	require.Equal(t, "Clip", formatMode(downloader.ClipRequest{}))
	require.Equal(t, "Clip (720p)", formatMode(downloader.ClipRequest{Quality: "720p"}))
	require.Equal(t, "Clip (HQ 2K)", formatMode(downloader.ClipRequest{HQ: true}))
	require.Equal(t, "GIF", formatMode(downloader.ClipRequest{GIF: true}))
	require.Equal(t, "GIF (720p)", formatMode(downloader.ClipRequest{GIF: true, Quality: "720p"}))
	require.Equal(t, "Subtitles (ru)", formatMode(downloader.ClipRequest{SubsLang: "ru"}))
	require.Equal(t, "Subtitles (ru, 720p)", formatMode(downloader.ClipRequest{SubsLang: "ru", Quality: "720p"}))
	require.Equal(t, "Subtitles (ru, HQ)", formatMode(downloader.ClipRequest{SubsLang: "ru", HQ: true}))
	require.Equal(t, "MP3", formatMode(downloader.ClipRequest{AudioOnly: true}))
}
