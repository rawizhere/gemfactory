package handlers

import (
	"testing"

	"github.com/stretchr/testify/require"
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
