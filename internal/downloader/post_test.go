package downloader

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTweetTokenMatchesEmbedWidget(t *testing.T) {
	// Expected values come from node: ((Number(id)/1e15)*Math.PI).toString(36).replace(/(0+|\.)/g, '').
	cases := map[string]string{
		"2112165463400808469": "54bkaasmqc8",
		"2112208058835406856": "54bp3q6ni6u",
		"20":                  "6dq1a2xwd93",
		"1234567890123456789": "2zqic77uqyk",
		"1585341984679469056": "3uchycv2wqc",
	}
	for id, want := range cases {
		require.Equal(t, want, tweetToken(id), "token for %s", id)
	}
}

func TestFetchTweetParsesMediaAndText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "42", r.URL.Query().Get("id"))
		_, _ = w.Write([]byte(`{
			"text": "hi #tag see https://t.co/link https://t.co/media",
			"entities": {
				"urls": [{"url": "https://t.co/link", "expanded_url": "https://example.com"}],
				"media": [{"url": "https://t.co/media"}]
			},
			"mediaDetails": [
				{"type": "photo", "media_url_https": "https://pbs.twimg.com/media/A.jpg"},
				{"type": "video", "video_info": {"duration_millis": 10000, "variants": [
					{"content_type": "application/x-mpegURL", "url": "https://v/pl.m3u8"},
					{"content_type": "video/mp4", "bitrate": 832000, "url": "https://v/540.mp4"},
					{"content_type": "video/mp4", "bitrate": 8768000, "url": "https://v/1080.mp4"}
				]}}
			]
		}`))
	}))
	defer srv.Close()
	old := tweetAPI
	tweetAPI = srv.URL
	defer func() { tweetAPI = old }()

	src, err := fetchTweet(context.Background(), "42", 49<<20)
	require.NoError(t, err)
	require.Equal(t, "hi #tag see https://example.com", src.text)
	require.Equal(t, []postItem{
		{url: "https://pbs.twimg.com/media/A.jpg?name=orig"},
		{video: true, url: "https://v/1080.mp4"},
	}, src.items)

	// 10s at 8.7 Mbit/s is ~11 MB: a 5 MB budget must fall back to the 540p variant.
	src, err = fetchTweet(context.Background(), "42", 5<<20)
	require.NoError(t, err)
	require.Equal(t, "https://v/540.mp4", src.items[1].url)
}

func TestTextHashtags(t *testing.T) {
	require.Equal(t, []string{"KiiiKiii", "키키", "LEESOL"},
		textHashtags("Sunset\n\n#KiiiKiii #키키\n#LEESOL #leesol"))
}

func TestPostManifestRoundTripAndExpiry(t *testing.T) {
	dir := t.TempDir()
	photo := filepath.Join(dir, "ig_x_1.jpg")
	video := filepath.Join(dir, "ig_x_2.mp4")
	require.NoError(t, os.WriteFile(photo, []byte("p"), 0644))
	require.NoError(t, os.WriteFile(video, []byte("v"), 0644))
	manifest := filepath.Join(dir, "ig_x_post.json")

	writePostManifest(manifest, "Sunset", []string{"#a"}, "<b>Sunset</b>", []PostMedia{{Path: photo}, {Path: video, Video: true}})
	m, media := readPostManifest(manifest)
	require.NotNil(t, m)
	require.Equal(t, "<b>Sunset</b>", m.Caption)
	require.Equal(t, []PostMedia{{Path: photo}, {Path: video, Video: true}}, media)

	// One expired item invalidates the whole cached post.
	require.NoError(t, os.Remove(video))
	m, _ = readPostManifest(manifest)
	require.Nil(t, m)
}

func TestIsPostJob(t *testing.T) {
	url := "https://x.com/u/status/1"
	require.True(t, isPostJob(ClipRequest{URL: url, Shorts: true}))
	require.False(t, isPostJob(ClipRequest{URL: url, AudioOnly: true}), "/mp3 keeps the yt-dlp path")
	require.False(t, isPostJob(ClipRequest{URL: url, Start: "0:01", End: "0:05"}), "intervals keep the yt-dlp path")
	require.False(t, isPostJob(ClipRequest{URL: "https://vm.tiktok.com/x"}))
}

func TestResubmitDonePostDeliversWholeAlbum(t *testing.T) {
	s := newTestService(t)
	req := ClipRequest{URL: "https://www.instagram.com/p/ABC123/", Shorts: true}

	dir := s.workbenchDir("ig_ABC123")
	var media []PostMedia
	for _, name := range []string{"ig_ABC123_1.jpg", "ig_ABC123_2.jpg"} {
		p := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(p, []byte("img"), 0644))
		media = append(media, PostMedia{Path: p})
	}
	writePostManifest(filepath.Join(dir, "ig_ABC123_post.json"), "title", nil, "caption", media)

	s.jobs["ig_ABC123_full"] = &Job{ID: "ig_ABC123_full", VideoID: "ig_ABC123", Request: req, Status: StatusDone, OutputDir: media[0].Path}

	got := make(chan []PostMedia, 1)
	_, err := s.SubmitWithCallbacks(context.Background(), req, &ClipCallbacks{
		OnPostDone: func(m []PostMedia, _ string) { got <- m },
		OnDone:     func(string, string) { t.Error("OnDone must not be used for posts") },
	})
	require.NoError(t, err)
	select {
	case m := <-got:
		require.Equal(t, media, m)
	case <-time.After(5 * time.Second):
		t.Fatal("album was not delivered")
	}
}

// Resubmitting a running job and listing jobs while the worker updates it must be race-free (run with -race).
func TestJobAccessIsRaceFree(t *testing.T) {
	s := newTestService(t)
	req := ClipRequest{URL: "https://www.instagram.com/p/RACE1/", Shorts: true}

	dir := s.workbenchDir("ig_RACE1")
	p := filepath.Join(dir, "ig_RACE1_1.jpg")
	require.NoError(t, os.WriteFile(p, []byte("img"), 0644))
	writePostManifest(filepath.Join(dir, "ig_RACE1_post.json"), "title", []string{"#tag"}, "caption", []PostMedia{{Path: p}})

	done := make(chan struct{}, 2)
	cbs := func() *ClipCallbacks {
		return &ClipCallbacks{
			OnStage:    func(string, string) {},
			OnHashtags: func([]string) {},
			OnPostDone: func([]PostMedia, string) { done <- struct{}{} },
		}
	}
	job, err := s.SubmitWithCallbacks(context.Background(), req, cbs())
	require.NoError(t, err)
	_, err = s.SubmitWithCallbacks(context.Background(), req, cbs())
	require.NoError(t, err)
	for i := 0; i < 50; i++ {
		_, _ = json.Marshal(s.ListJobs())
		_, _ = json.Marshal(job)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("post was not delivered")
	}
}
