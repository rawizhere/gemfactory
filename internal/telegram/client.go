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

	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	"github.com/mymmrac/telego/telegoapi"
	"github.com/mymmrac/telego/telegoutil"
	"github.com/dustin/go-humanize"
	"go.uber.org/zap"
)

const maxMessageLength = 4000

// Router registers update handlers on the telegohandler bot handler.
type Router interface {
	RegisterRoutes(bh *th.BotHandler)
	RegisterBotCommands() []telego.BotCommand
}

type Client struct {
	bot    *telego.Bot
	router Router
	logger *zap.Logger
}

func NewClient(botToken string, logger *zap.Logger) (*Client, error) {
	bot, err := telego.NewBot(botToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create bot API: %w", err)
	}

	return &Client{
		bot:    bot,
		logger: logger,
	}, nil
}

func (c *Client) Start(ctx context.Context, router Router) error {
	c.router = router

	me, err := c.bot.GetMe(ctx)
	if err != nil {
		return fmt.Errorf("failed to get bot info: %w", err)
	}
	c.logger.Info("Bot started", zap.String("username", me.Username))

	_ = c.bot.DeleteWebhook(ctx, &telego.DeleteWebhookParams{DropPendingUpdates: true})

	if err := c.SetBotCommands(ctx, c.router.RegisterBotCommands()); err != nil {
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
	c.router.RegisterRoutes(bh)

	// Blocks until the updates channel closes (ctx cancellation) or Stop is called.
	return bh.Start()
}

func (c *Client) SendMessage(ctx context.Context, chatID int64, text string) error {
	chunks := splitMessage(text, maxMessageLength)
	for _, chunk := range chunks {
		params := telegoutil.Message(telegoutil.ID(chatID), chunk).WithParseMode(telego.ModeHTML)
		params.LinkPreviewOptions = &telego.LinkPreviewOptions{IsDisabled: true}
		_, err := c.bot.SendMessage(ctx, params)
		if err != nil {
			c.logger.Error("Failed to send message", zap.Int64("chat_id", chatID), zap.Error(err))
			return err
		}
	}
	return nil
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
			c.logger.Error("Failed to send message with markup", zap.Int64("chat_id", chatID), zap.Error(err))
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

func (c *Client) SendVideoFile(ctx context.Context, chatID int64, filePath, caption string, onProgress UploadProgressCallback, replyToMsgID int) error {
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open video file: %w", err)
	}
	defer func() { _ = f.Close() }()

	var fileReader telegoapi.NamedReader = f
	if stat, statErr := f.Stat(); statErr == nil && stat.Size() > 0 && onProgress != nil {
		fileReader = newProgressFileReader(f, stat.Size(), onProgress)
	}

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

func (c *Client) SendAudioFile(ctx context.Context, chatID int64, filePath, caption string, onProgress UploadProgressCallback, replyToMsgID int) error {
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open audio file: %w", err)
	}
	defer func() { _ = f.Close() }()

	var fileReader telegoapi.NamedReader = f
	if stat, statErr := f.Stat(); statErr == nil && stat.Size() > 0 && onProgress != nil {
		fileReader = newProgressFileReader(f, stat.Size(), onProgress)
	}

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
