package telegram

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/mymmrac/telego"
	"github.com/mymmrac/telego/telegoapi"
	th "github.com/mymmrac/telego/telegohandler"
	"github.com/mymmrac/telego/telegoutil"
	"go.uber.org/zap"
)

const maxMessageLength = 4000

type Client struct {
	bot    *telego.Bot
	logger *zap.Logger
}

// zapLogger routes telego internal logs into zap.
type zapLogger struct{ l *zap.Logger }

func (z zapLogger) Debugf(format string, args ...any) { z.l.Sugar().Debugf(format, args...) }
func (z zapLogger) Errorf(format string, args ...any) { z.l.Sugar().Errorf(format, args...) }

// retryCaller retries an API call once on transient failures (dead keep-alive conns, 5xx).
type retryCaller struct {
	next telegoapi.Caller
	log  *zap.Logger
}

func (c retryCaller) Call(ctx context.Context, url string, data *telegoapi.RequestData) (*telegoapi.Response, error) {
	res, err := c.next.Call(ctx, url, data)
	if err == nil || ctx.Err() != nil {
		return res, err
	}
	c.log.Warn("telegram api call failed, retrying once", zap.Error(err))
	return c.next.Call(ctx, url, data)
}

func NewClient(botToken string, logger *zap.Logger) (*Client, error) {
	bot, err := telego.NewBot(botToken,
		telego.WithAPICaller(retryCaller{next: telegoapi.DefaultFastHTTPCaller, log: logger}),
		telego.WithLogger(zapLogger{l: logger}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create bot API: %w", err)
	}

	return &Client{
		bot:    bot,
		logger: logger,
	}, nil
}

func (c *Client) Start(ctx context.Context, commands []telego.BotCommand, registerRoutes func(*th.BotHandler)) error {

	me, err := c.bot.GetMe(ctx)
	if err != nil {
		return fmt.Errorf("failed to get bot info: %w", err)
	}
	c.logger.Info("Bot started", zap.String("username", me.Username))

	_ = c.bot.DeleteWebhook(ctx, &telego.DeleteWebhookParams{DropPendingUpdates: true})

	if err := c.SetBotCommands(ctx, commands); err != nil {
		c.logger.Error("Failed to set bot commands", zap.Error(err))
	}

	c.logger.Info("Starting to fetch updates")
	updatesChan, err := c.bot.UpdatesViaLongPolling(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to create updates channel: %w", err)
	}

	bh, err := th.NewBotHandler(c.bot, updatesChan)
	if err != nil {
		return fmt.Errorf("failed to create bot handler: %w", err)
	}
	registerRoutes(bh)

	// Blocks until the updates channel closes (ctx cancellation) or Stop is called.
	return bh.Start()
}

func (c *Client) SendMessage(ctx context.Context, chatID int64, text string) error {
	return c.SendMessageWithMarkup(ctx, chatID, text, nil)
}

func (c *Client) SendMessageWithMarkup(ctx context.Context, chatID int64, text string, markup telego.ReplyMarkup) error {
	chunks := splitMessage(text, maxMessageLength)
	for i, chunk := range chunks {
		params := telegoutil.Message(telegoutil.ID(chatID), chunk).WithParseMode(telego.ModeHTML)
		params.LinkPreviewOptions = &telego.LinkPreviewOptions{IsDisabled: true}

		if i == len(chunks)-1 && markup != nil {
			params.ReplyMarkup = markup
		}

		_, err := c.bot.SendMessage(ctx, params)
		if err != nil {
			c.logger.Error("Failed to send message", zap.Int64("chat_id", chatID), zap.Error(err))
			return err
		}
	}
	return nil
}

func (c *Client) EditMessageReplyMarkup(ctx context.Context, chatID int64, messageID int, markup *telego.InlineKeyboardMarkup) error {
	params := &telego.EditMessageReplyMarkupParams{
		ChatID:      telegoutil.ID(chatID),
		MessageID:   messageID,
		ReplyMarkup: markup,
	}

	_, err := c.bot.EditMessageReplyMarkup(ctx, params)
	if err != nil {
		c.logger.Error("Failed to edit message reply markup", zap.Int64("chat_id", chatID), zap.Int("message_id", messageID), zap.Error(err))
	}
	return err
}

func (c *Client) SendMessageRaw(ctx context.Context, chatID int64, text string, replyToMsgID ...int) (*telego.Message, error) {
	params := telegoutil.Message(telegoutil.ID(chatID), text).WithParseMode(telego.ModeHTML)
	params.LinkPreviewOptions = &telego.LinkPreviewOptions{IsDisabled: true}
	if len(replyToMsgID) > 0 && replyToMsgID[0] > 0 {
		params.ReplyParameters = &telego.ReplyParameters{
			MessageID: replyToMsgID[0],
		}
	}
	return c.bot.SendMessage(ctx, params)
}

func (c *Client) EditMessageText(ctx context.Context, chatID int64, messageID int, text string) error {
	params := &telego.EditMessageTextParams{
		ChatID:    telegoutil.ID(chatID),
		MessageID: messageID,
		Text:      text,
		ParseMode: telego.ModeHTML,
	}
	params.LinkPreviewOptions = &telego.LinkPreviewOptions{IsDisabled: true}
	_, err := c.bot.EditMessageText(ctx, params)
	if err != nil && strings.Contains(err.Error(), "message is not modified") {
		return nil
	}
	if err != nil {
		c.logger.Warn("Failed to edit message", zap.Int64("chat_id", chatID), zap.Int("message_id", messageID), zap.Error(err))
	}
	return err
}

func probeVideoMeta(filePath string) (int, int, int) {
	out, err := exec.Command("ffprobe", "-v", "error",
		"-show_entries", "stream=width,height:format=duration",
		"-of", "csv=s=x:p=0", filePath).Output()
	if err != nil {
		return 0, 0, 0
	}
	fields := strings.Fields(strings.ReplaceAll(string(out), "x", " "))
	var w, h, dur int
	if len(fields) >= 2 {
		w, _ = strconv.Atoi(fields[0])
		h, _ = strconv.Atoi(fields[1])
	}
	if len(fields) >= 3 {
		if d, derr := strconv.ParseFloat(fields[2], 64); derr == nil && d > 0 {
			dur = int(d + 0.5)
		}
	}
	return w, h, dur
}

func generateVideoThumbnail(filePath string) string {
	thumbPath := strings.TrimSuffix(filePath, filepath.Ext(filePath)) + "_thumb.jpg"
	cmd := exec.Command("ffmpeg", "-y", "-nostdin",
		"-ss", "00:00:00.500",
		"-i", filePath,
		"-vframes", "1",
		"-q:v", "2",
		"-vf", "scale=320:-2",
		thumbPath)
	if err := cmd.Run(); err != nil {
		cmd0 := exec.Command("ffmpeg", "-y", "-nostdin",
			"-ss", "00:00:00.000",
			"-i", filePath,
			"-vframes", "1",
			"-q:v", "2",
			"-vf", "scale=320:-2",
			thumbPath)
		if err0 := cmd0.Run(); err0 != nil {
			return ""
		}
	}
	if info, err := os.Stat(thumbPath); err == nil && info.Size() > 0 {
		return thumbPath
	}
	return ""
}

type UploadProgressCallback func(pct int, speed, sizeStr string)

type progressFileReader struct {
	file       *os.File
	total      int64
	read       int64
	lastRead   int64
	lastEmitAt time.Time
	onProgress UploadProgressCallback
}

func newProgressFileReader(f *os.File, total int64, onProgress UploadProgressCallback) *progressFileReader {
	return &progressFileReader{
		file:       f,
		total:      total,
		lastEmitAt: time.Now(),
		onProgress: onProgress,
	}
}

func (p *progressFileReader) Read(b []byte) (int, error) {
	n, err := p.file.Read(b)
	if n > 0 {
		p.read += int64(n)
		if p.onProgress != nil && p.total > 0 {
			now := time.Now()
			elapsed := now.Sub(p.lastEmitAt)
			if elapsed >= 1500*time.Millisecond || p.read >= p.total {
				pct := int(float64(p.read) / float64(p.total) * 100)
				if pct > 100 {
					pct = 100
				}
				bytesDiff := p.read - p.lastRead
				var speedStr string
				if elapsed.Seconds() > 0 && bytesDiff > 0 {
					speedBps := float64(bytesDiff) / elapsed.Seconds()
					speedStr = formatSpeed(speedBps)
				}
				sizeStr := fmt.Sprintf("%.1f / %.1f MB", float64(p.read)/(1024*1024), float64(p.total)/(1024*1024))
				p.lastEmitAt = now
				p.lastRead = p.read
				p.onProgress(pct, speedStr, sizeStr)
			}
		}
	}
	return n, err
}

func (p *progressFileReader) Name() string {
	return p.file.Name()
}

func formatSpeed(bytesPerSec float64) string {
	return humanize.Bytes(uint64(bytesPerSec)) + "/s"
}

// openUpload opens filePath for upload, wrapping it to report progress when onProgress is set.
func openUpload(filePath string, onProgress UploadProgressCallback) (*os.File, telegoapi.NamedReader, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, nil, err
	}
	if stat, statErr := f.Stat(); statErr == nil && stat.Size() > 0 && onProgress != nil {
		return f, newProgressFileReader(f, stat.Size(), onProgress), nil
	}
	return f, f, nil
}

func (c *Client) SendVideoFile(ctx context.Context, chatID int64, filePath, caption string, onProgress UploadProgressCallback, replyToMsgID int) error {
	f, fileReader, err := openUpload(filePath, onProgress)
	if err != nil {
		return fmt.Errorf("open video file: %w", err)
	}
	defer func() { _ = f.Close() }()

	video := telegoutil.Video(telegoutil.ID(chatID), telegoutil.File(fileReader))
	if caption != "" {
		video = video.WithCaption(caption).WithParseMode(telego.ModeHTML)
	}
	video = video.WithSupportsStreaming()
	if replyToMsgID > 0 {
		video.ReplyParameters = &telego.ReplyParameters{
			MessageID:                replyToMsgID,
			AllowSendingWithoutReply: true,
		}
	}
	if w, h, dur := probeVideoMeta(filePath); w > 0 && h > 0 {
		video = video.WithWidth(w).WithHeight(h)
		if dur > 0 {
			video = video.WithDuration(dur)
		}
	}

	if thumbPath := generateVideoThumbnail(filePath); thumbPath != "" {
		defer func() { _ = os.Remove(thumbPath) }()
		if thumbFile, err := os.Open(thumbPath); err == nil {
			defer func() { _ = thumbFile.Close() }()
			thumb := telegoutil.File(thumbFile)
			video = video.WithThumbnail(&thumb)
		}
	}

	if _, err := c.bot.SendVideo(ctx, video); err != nil {
		c.logger.Error("Failed to send video", zap.Int64("chat_id", chatID), zap.String("file", filePath), zap.Error(err))
		return fmt.Errorf("send video: %w", err)
	}
	return nil
}

// AlbumItem is one photo or video of a media group.
type AlbumItem struct {
	Path  string
	Video bool
}

// albumMaxItems and albumMaxBytes split large posts into several media groups (Bot API: 2-10 items, ~50 MB per request).
const (
	albumMaxItems = 10
	albumMaxBytes = 49 << 20
)

// SendAlbum sends photos/videos as media groups with the caption on the first item; a lone item goes as a plain photo or video.
func (c *Client) SendAlbum(ctx context.Context, chatID int64, items []AlbumItem, caption string, replyToMsgID int) error {
	for i, chunk := range chunkAlbum(items) {
		if i > 0 {
			caption, replyToMsgID = "", 0
		}
		if err := c.sendAlbumChunk(ctx, chatID, chunk, caption, replyToMsgID); err != nil {
			return err
		}
	}
	return nil
}

func chunkAlbum(items []AlbumItem) [][]AlbumItem {
	var chunks [][]AlbumItem
	var cur []AlbumItem
	var curBytes int64
	for _, it := range items {
		var size int64
		if st, err := os.Stat(it.Path); err == nil {
			size = st.Size()
		}
		if len(cur) == albumMaxItems || (len(cur) > 0 && curBytes+size > albumMaxBytes) {
			chunks = append(chunks, cur)
			cur, curBytes = nil, 0
		}
		cur = append(cur, it)
		curBytes += size
	}
	if len(cur) > 0 {
		chunks = append(chunks, cur)
	}
	return chunks
}

func (c *Client) sendAlbumChunk(ctx context.Context, chatID int64, chunk []AlbumItem, caption string, replyToMsgID int) error {
	if len(chunk) == 1 {
		if chunk[0].Video {
			return c.SendVideoFile(ctx, chatID, chunk[0].Path, caption, nil, replyToMsgID)
		}
		return c.sendPhotoFile(ctx, chatID, chunk[0].Path, caption, replyToMsgID)
	}

	media := make([]telego.InputMedia, 0, len(chunk))
	for i, it := range chunk {
		f, err := os.Open(it.Path)
		if err != nil {
			return fmt.Errorf("open album file: %w", err)
		}
		defer func() { _ = f.Close() }()
		itemCaption := ""
		if i == 0 {
			itemCaption = caption
		}
		if it.Video {
			v := telegoutil.MediaVideo(telegoutil.File(f)).WithSupportsStreaming()
			if w, h, dur := probeVideoMeta(it.Path); w > 0 && h > 0 {
				v = v.WithWidth(w).WithHeight(h).WithDuration(dur)
			}
			if itemCaption != "" {
				v = v.WithCaption(itemCaption).WithParseMode(telego.ModeHTML)
			}
			media = append(media, v)
			continue
		}
		p := telegoutil.MediaPhoto(telegoutil.File(f))
		if itemCaption != "" {
			p = p.WithCaption(itemCaption).WithParseMode(telego.ModeHTML)
		}
		media = append(media, p)
	}

	group := telegoutil.MediaGroup(telegoutil.ID(chatID), media...)
	if replyToMsgID > 0 {
		group.ReplyParameters = &telego.ReplyParameters{MessageID: replyToMsgID, AllowSendingWithoutReply: true}
	}
	if _, err := c.bot.SendMediaGroup(ctx, group); err != nil {
		c.logger.Error("Failed to send media group", zap.Int64("chat_id", chatID), zap.Int("items", len(chunk)), zap.Error(err))
		return fmt.Errorf("send media group: %w", err)
	}
	return nil
}

func (c *Client) sendPhotoFile(ctx context.Context, chatID int64, filePath, caption string, replyToMsgID int) error {
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open photo file: %w", err)
	}
	defer func() { _ = f.Close() }()

	photo := telegoutil.Photo(telegoutil.ID(chatID), telegoutil.File(f))
	if caption != "" {
		photo = photo.WithCaption(caption).WithParseMode(telego.ModeHTML)
	}
	if replyToMsgID > 0 {
		photo.ReplyParameters = &telego.ReplyParameters{MessageID: replyToMsgID, AllowSendingWithoutReply: true}
	}
	if _, err := c.bot.SendPhoto(ctx, photo); err != nil {
		c.logger.Error("Failed to send photo", zap.Int64("chat_id", chatID), zap.String("file", filePath), zap.Error(err))
		return fmt.Errorf("send photo: %w", err)
	}
	return nil
}

func (c *Client) SendAudioFile(ctx context.Context, chatID int64, filePath, caption string, onProgress UploadProgressCallback, replyToMsgID int) error {
	f, fileReader, err := openUpload(filePath, onProgress)
	if err != nil {
		return fmt.Errorf("open audio file: %w", err)
	}
	defer func() { _ = f.Close() }()

	audio := telegoutil.Audio(telegoutil.ID(chatID), telegoutil.File(fileReader))
	if caption != "" {
		audio = audio.WithCaption(caption).WithParseMode(telego.ModeHTML)
	}
	if replyToMsgID > 0 {
		audio.ReplyParameters = &telego.ReplyParameters{
			MessageID:                replyToMsgID,
			AllowSendingWithoutReply: true,
		}
	}

	if _, err := c.bot.SendAudio(ctx, audio); err != nil {
		c.logger.Error("Failed to send audio", zap.Int64("chat_id", chatID), zap.String("file", filePath), zap.Error(err))
		return fmt.Errorf("send audio: %w", err)
	}
	return nil
}

func (c *Client) DeleteMessage(ctx context.Context, chatID int64, messageID int) error {
	return c.bot.DeleteMessage(ctx, &telego.DeleteMessageParams{
		ChatID:    telegoutil.ID(chatID),
		MessageID: messageID,
	})
}

func (c *Client) SetBotCommands(ctx context.Context, commands []telego.BotCommand) error {
	err := c.bot.SetMyCommands(ctx, &telego.SetMyCommandsParams{Commands: commands})
	if err != nil {
		c.logger.Error("Failed to set bot commands", zap.Error(err))
	}
	return err
}

var htmlTagRe = regexp.MustCompile(`</?([a-zA-Z][a-zA-Z0-9]*)[^>]*/?>`)

// splitMessage splits text into chunks under the length limit at line boundaries,
// keeping every chunk valid HTML: tags left open at a chunk end are closed there
// and reopened at the start of the next chunk.
func splitMessage(text string, limit int) []string {
	if len(text) <= limit {
		return []string{text}
	}

	var chunks []string
	var current strings.Builder

	for _, line := range strings.Split(text, "\n") {
		if current.Len()+len(line)+1 > limit && current.Len() > 0 {
			chunks = append(chunks, current.String())
			current.Reset()
		}

		if current.Len() > 0 {
			current.WriteString("\n")
		}
		current.WriteString(line)
	}

	if current.Len() > 0 {
		chunks = append(chunks, current.String())
	}

	return rebalanceHTMLTags(chunks)
}

// rebalanceHTMLTags closes tags still open at the end of each chunk and reopens
// them at the start of the following chunk.
func rebalanceHTMLTags(chunks []string) []string {
	out := make([]string, len(chunks))
	var open []string

	for i, chunk := range chunks {
		var prefix strings.Builder
		for _, tag := range open {
			prefix.WriteString("<" + tag + ">")
		}

		for _, m := range htmlTagRe.FindAllStringSubmatch(chunk, -1) {
			name := strings.ToLower(m[1])
			if strings.HasPrefix(m[0], "</") {
				for j := len(open) - 1; j >= 0; j-- {
					if open[j] == name {
						open = slices.Delete(open, j, j+1)
						break
					}
				}
			} else if !strings.HasSuffix(m[0], "/>") {
				open = append(open, name)
			}
		}

		var suffix strings.Builder
		for j := len(open) - 1; j >= 0; j-- {
			suffix.WriteString("</" + open[j] + ">")
		}

		out[i] = prefix.String() + chunk + suffix.String()
	}

	return out
}
