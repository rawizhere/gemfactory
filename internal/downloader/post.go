package downloader

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"go.uber.org/zap"
)

// PostMedia is one downloaded photo or video of an Instagram/X post.
type PostMedia struct {
	Path  string `json:"path"`
	Video bool   `json:"video"`
}

// postItem is one media entry of a post before download: a direct URL, or a yt-dlp info dict for IG videos.
type postItem struct {
	video bool
	url   string
	info  json.RawMessage
}

type postSource struct {
	text  string
	items []postItem
}

// postManifest records a finished post so repeats are served from disk.
type postManifest struct {
	Title   string   `json:"title"`
	Tags    []string `json:"tags,omitempty"`
	Caption string   `json:"caption"`
	Items   []struct {
		Name  string `json:"name"`
		Video bool   `json:"video"`
	} `json:"items"`
}

// isPostJob routes whole-post requests to runPost; intervals and /mp3 keep the regular yt-dlp path.
func isPostJob(req ClipRequest) bool {
	return IsPostURL(req.URL) && !req.AudioOnly && req.Start == "" && req.End == ""
}

func (s *Service) runPost(ctx context.Context, job *Job) {
	dir := s.workbenchDir(job.VideoID)
	manifestPath := filepath.Join(dir, job.VideoID+"_post.json")
	if m, media := readPostManifest(manifestPath); m != nil {
		s.logger.Info("post cache hit", zap.String("job", job.ID))
		s.reportStage(job, StageMetadata, m.Title)
		if cbs := s.callbacksOf(job); cbs != nil && cbs.OnHashtags != nil && len(m.Tags) > 0 {
			cbs.OnHashtags(m.Tags)
		}
		s.finishPost(job, media, m.Caption)
		return
	}

	s.setStatus(job, StatusDownloading)
	s.reportStage(job, StageMetadata, "")

	cookieFile, cleanup, err := s.prepareCookieFile(ctx, cookieDomainFor(job.Request.URL))
	if err == nil {
		defer cleanup()
	}

	src, err := s.fetchPostSource(ctx, job.Request.URL, cookieFile)
	if err != nil {
		s.fail(job, "post lookup failed: "+err.Error())
		return
	}
	if len(src.items) == 0 {
		s.fail(job, "post has no photos or videos")
		return
	}

	meta := &SourceMeta{Description: src.text, Tags: textHashtags(src.text)}
	// Title is the first text line; setting it lets hashtags() skip tags already shown there.
	meta.Title = DisplayTitle(meta)
	title := meta.Title
	s.reportStage(job, StageMetadata, title)
	if cbs := s.callbacksOf(job); cbs != nil && cbs.OnHashtags != nil {
		cbs.OnHashtags(AllHashtags(meta))
	}

	s.reportStage(job, StageDownload, "")
	limit := s.telegramFileLimitBytes(ctx)
	media := make([]PostMedia, 0, len(src.items))
	for i, it := range src.items {
		base := filepath.Join(dir, fmt.Sprintf("%s_%d", job.VideoID, i+1))
		var p string
		switch {
		case it.info != nil:
			p, err = s.downloadInfoJSON(ctx, it.info, base, cookieFile)
		case it.video:
			p = base + ".mp4"
			err = downloadHTTP(ctx, it.url, p)
		default:
			p = base + mediaExt(it.url)
			err = downloadHTTP(ctx, it.url, p)
		}
		if err != nil {
			s.fail(job, fmt.Sprintf("download failed (item %d/%d): %s", i+1, len(src.items), err))
			return
		}
		if it.video {
			// Same output contract as TikTok/Shorts: H.264/AAC mp4 within the size budget.
			s.setStatus(job, StatusProcessing)
			s.reportStage(job, StageReencode, "")
			if p, err = s.ReencodeWithSubs(ctx, p, "", false, 0, nil); err != nil {
				s.fail(job, "re-encode failed: "+err.Error())
				return
			}
			if err := checkFileSize(p, limit); err != nil {
				s.fail(job, err.Error())
				return
			}
		}
		media = append(media, PostMedia{Path: p, Video: it.video})
		s.reportProgress(job, ProgressUpdate{Stage: StageDownload, Percent: (i + 1) * 100 / len(src.items)})
	}

	caption := FormatCaption(title, CaptionHashtags(meta))
	writePostManifest(manifestPath, title, AllHashtags(meta), caption, media)
	s.finishPost(job, media, caption)
	s.logger.Info("post ready", zap.String("job", job.ID), zap.Int("items", len(media)))
}

func (s *Service) finishPost(job *Job, media []PostMedia, caption string) {
	s.mu.Lock()
	job.Status = StatusDone
	job.OutputDir = media[0].Path
	job.Caption = caption
	cbs := job.callbacks
	s.mu.Unlock()
	s.reportStage(job, StageUpload, "")
	switch {
	case cbs == nil:
	case cbs.OnPostDone != nil:
		cbs.OnPostDone(media, caption)
	case cbs.OnDone != nil:
		cbs.OnDone(media[0].Path, caption)
	}
}

func readPostManifest(p string) (*postManifest, []PostMedia) {
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, nil
	}
	var m postManifest
	if json.Unmarshal(data, &m) != nil || len(m.Items) == 0 {
		return nil, nil
	}
	media := make([]PostMedia, 0, len(m.Items))
	for _, it := range m.Items {
		full := filepath.Join(filepath.Dir(p), it.Name)
		// Cleanup may have expired some files; refetch the whole post then.
		if st, err := os.Stat(full); err != nil || st.Size() == 0 {
			return nil, nil
		}
		media = append(media, PostMedia{Path: full, Video: it.Video})
	}
	return &m, media
}

func writePostManifest(p, title string, tags []string, caption string, media []PostMedia) {
	m := postManifest{Title: title, Tags: tags, Caption: caption}
	for _, it := range media {
		m.Items = append(m.Items, struct {
			Name  string `json:"name"`
			Video bool   `json:"video"`
		}{filepath.Base(it.Path), it.Video})
	}
	if data, err := json.Marshal(m); err == nil {
		_ = os.WriteFile(p, data, 0644)
	}
}

func cookieDomainFor(raw string) string {
	switch {
	case instagramShortcode(raw) != "":
		return "instagram.com"
	case tweetID(raw) != "":
		return "x.com"
	case IsTikTokURL(raw):
		return "tiktok.com"
	default:
		return "youtube.com"
	}
}

func (s *Service) fetchPostSource(ctx context.Context, rawURL, cookieFile string) (*postSource, error) {
	if id := tweetID(rawURL); id != "" {
		return fetchTweet(ctx, id, float64(s.telegramFileLimitBytes(ctx)))
	}
	return s.fetchInstagramPost(ctx, rawURL, cookieFile)
}

// tweetAPI is the public endpoint behind X's embed widget; it needs no auth.
var tweetAPI = "https://cdn.syndication.twimg.com/tweet-result"

type tweetResult struct {
	TypeName  string `json:"__typename"`
	Text      string `json:"text"`
	Tombstone struct {
		Text struct {
			Text string `json:"text"`
		} `json:"text"`
	} `json:"tombstone"`
	Entities struct {
		URLs []struct {
			URL         string `json:"url"`
			ExpandedURL string `json:"expanded_url"`
		} `json:"urls"`
		Media []struct {
			URL string `json:"url"`
		} `json:"media"`
	} `json:"entities"`
	MediaDetails []struct {
		Type          string `json:"type"` // photo, video, animated_gif
		MediaURLHTTPS string `json:"media_url_https"`
		VideoInfo     struct {
			DurationMillis float64 `json:"duration_millis"`
			Variants       []struct {
				ContentType string  `json:"content_type"`
				Bitrate     float64 `json:"bitrate"`
				URL         string  `json:"url"`
			} `json:"variants"`
		} `json:"video_info"`
	} `json:"mediaDetails"`
}

func fetchTweet(ctx context.Context, id string, limitBytes float64) (*postSource, error) {
	q := url.Values{"id": {id}, "token": {tweetToken(id)}, "lang": {"en"}}
	body, err := httpGet(ctx, tweetAPI+"?"+q.Encode())
	if err != nil {
		return nil, fmt.Errorf("tweet unavailable (deleted, protected or age-restricted?): %w", err)
	}
	var tw tweetResult
	if err := json.Unmarshal(body, &tw); err != nil {
		return nil, fmt.Errorf("parse tweet: %w", err)
	}
	if tw.TypeName == "TweetTombstone" {
		// X answers with a tombstone for deleted, suspended and age-restricted (18+) posts alike.
		reason := strings.TrimSuffix(strings.TrimSpace(tw.Tombstone.Text.Text), "Learn more")
		return nil, fmt.Errorf("post is not available for download (deleted, suspended or sensitive/18+): %s", strings.TrimSpace(reason))
	}

	text := tw.Text
	for _, m := range tw.Entities.Media {
		text = strings.ReplaceAll(text, m.URL, "")
	}
	for _, u := range tw.Entities.URLs {
		text = strings.ReplaceAll(text, u.URL, u.ExpandedURL)
	}
	src := &postSource{text: strings.TrimSpace(text)}

	for _, m := range tw.MediaDetails {
		if m.Type == "photo" {
			src.items = append(src.items, postItem{url: m.MediaURLHTTPS + "?name=orig"})
			continue
		}
		dur := m.VideoInfo.DurationMillis / 1000
		// Best mp4 whose estimated size fits the Telegram budget; the smallest one otherwise (re-encode caps it).
		var best, smallest string
		var bestRate, smallRate float64
		for _, v := range m.VideoInfo.Variants {
			if v.ContentType != "video/mp4" {
				continue
			}
			if smallest == "" || v.Bitrate < smallRate {
				smallest, smallRate = v.URL, v.Bitrate
			}
			if v.Bitrate/8*dur <= limitBytes && (best == "" || v.Bitrate > bestRate) {
				best, bestRate = v.URL, v.Bitrate
			}
		}
		if best == "" {
			best = smallest
		}
		if best != "" {
			src.items = append(src.items, postItem{video: true, url: best})
		}
	}
	return src, nil
}

// tweetToken mirrors the embed widget: ((id / 1e15) * Math.PI).toString(36) with zeros and dots stripped.
func tweetToken(id string) string {
	var n float64
	if _, err := fmt.Sscan(id, &n); err != nil {
		return ""
	}
	return strings.NewReplacer("0", "", ".", "").Replace(jsRadixString(n/1e15*math.Pi, 36))
}

// jsRadixString ports V8's DoubleToRadixCString so fractional output matches Number.prototype.toString(radix).
func jsRadixString(value float64, radix int) string {
	const chars = "0123456789abcdefghijklmnopqrstuvwxyz"
	integer := math.Floor(value)
	fraction := value - integer
	delta := math.Max(0.5*(math.Nextafter(value, math.Inf(1))-value), math.SmallestNonzeroFloat64)

	var frac []byte
	if fraction >= delta {
		for {
			fraction *= float64(radix)
			delta *= float64(radix)
			digit := int(fraction)
			frac = append(frac, chars[digit])
			fraction -= float64(digit)
			if fraction > 0.5 || (fraction == 0.5 && digit&1 == 1) {
				if fraction+delta > 1 {
					// Round up, propagating carries into the integer part.
					for {
						last := len(frac) - 1
						if last < 0 {
							integer++
							break
						}
						d := strings.IndexByte(chars, frac[last])
						frac = frac[:last]
						if d+1 < radix {
							frac = append(frac, chars[d+1])
							break
						}
					}
					break
				}
			}
			if fraction < delta {
				break
			}
		}
	}

	var intPart []byte
	for {
		rem := math.Mod(integer, float64(radix))
		intPart = append([]byte{chars[int(rem)]}, intPart...)
		integer = (integer - rem) / float64(radix)
		if integer <= 0 {
			break
		}
	}
	if len(frac) == 0 {
		return string(intPart)
	}
	return string(intPart) + "." + string(frac)
}

// igInfo is the subset of yt-dlp's -J output used for posts; a carousel is a playlist of entries.
type igInfo struct {
	Type        string            `json:"_type"`
	Description string            `json:"description"`
	Entries     []json.RawMessage `json:"entries"`
}

type igEntry struct {
	Formats    []json.RawMessage `json:"formats"`
	Thumbnails []struct {
		URL string `json:"url"`
	} `json:"thumbnails"`
}

// fetchInstagramPost lists post media via yt-dlp, which handles IG's anonymous API; photo entries
// carry no formats, so their full-size image is the best (last) thumbnail.
func (s *Service) fetchInstagramPost(ctx context.Context, rawURL, cookieFile string) (*postSource, error) {
	bin, err := YTDLPBinary()
	if err != nil {
		return nil, err
	}
	args := append(s.baseYTDLPArgs(ctx, cookieFile), "--ignore-no-formats-error", "-J", rawURL)
	cmd := exec.CommandContext(ctx, bin, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("yt-dlp: %w: %s", err, truncate(stderr.String(), 2000))
	}

	var info igInfo
	if err := json.Unmarshal(out, &info); err != nil {
		return nil, fmt.Errorf("parse yt-dlp output: %w", err)
	}
	entries := info.Entries
	if info.Type != "playlist" {
		entries = []json.RawMessage{out}
	}

	src := &postSource{text: info.Description}
	for _, raw := range entries {
		var e igEntry
		if json.Unmarshal(raw, &e) != nil {
			continue
		}
		switch {
		case len(e.Formats) > 0:
			src.items = append(src.items, postItem{video: true, info: raw})
		case len(e.Thumbnails) > 0:
			src.items = append(src.items, postItem{url: e.Thumbnails[len(e.Thumbnails)-1].URL})
		}
	}
	return src, nil
}

// downloadInfoJSON downloads a video from an already extracted info dict, skipping a second IG API round trip.
func (s *Service) downloadInfoJSON(ctx context.Context, info json.RawMessage, base, cookieFile string) (string, error) {
	bin, err := YTDLPBinary()
	if err != nil {
		return "", err
	}
	infoPath := base + ".info.json"
	if err := os.WriteFile(infoPath, info, 0644); err != nil {
		return "", err
	}
	args := append(s.baseYTDLPArgs(ctx, cookieFile),
		"-f", formatSelector("", false, true, false, false),
		"--merge-output-format", "mp4",
		"--load-info-json", infoPath,
		"-o", base+".%(ext)s",
	)
	if out, err := exec.CommandContext(ctx, bin, args...).CombinedOutput(); err != nil {
		return "", fmt.Errorf("yt-dlp: %w: %s", err, truncate(string(out), 2000))
	}
	return base + ".mp4", nil
}

var postHTTPClient = &http.Client{Timeout: 3 * time.Minute}

// fetch GETs rawURL with a browser User-Agent and fails on non-200 responses; the caller closes the body.
func fetch(ctx context.Context, rawURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0 Safari/537.36")
	resp, err := postHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return resp, nil
}

func httpGet(ctx context.Context, rawURL string) ([]byte, error) {
	resp, err := fetch(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	return io.ReadAll(io.LimitReader(resp.Body, 10<<20))
}

func downloadHTTP(ctx context.Context, rawURL, outPath string) error {
	resp, err := fetch(ctx, rawURL)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	tmp := outPath + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, outPath)
}

// mediaExt returns the image extension from a CDN URL path, defaulting to .jpg.
func mediaExt(rawURL string) string {
	if u, err := url.Parse(rawURL); err == nil {
		switch ext := strings.ToLower(path.Ext(u.Path)); ext {
		case ".jpg", ".jpeg", ".png", ".webp":
			return ext
		}
	}
	return ".jpg"
}

var hashtagRe = regexp.MustCompile(`#([\p{L}\p{N}_]+)`)

// textHashtags pulls unique #tags (without '#') from post text.
func textHashtags(text string) []string {
	var tags []string
	seen := map[string]bool{}
	for _, m := range hashtagRe.FindAllStringSubmatch(text, -1) {
		if k := strings.ToLower(m[1]); !seen[k] {
			seen[k] = true
			tags = append(tags, m[1])
		}
	}
	return tags
}
