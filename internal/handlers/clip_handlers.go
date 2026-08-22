package handlers

import (
	"context"
	"fmt"
	"html"
	"strings"

	"github.com/mymmrac/telego"
	"go.uber.org/zap"

	"gemfactory/internal/downloader"
)

// ClipHandlers implement /clip, /gif and /subs Telegram commands.
type ClipHandlers struct {
	*BaseHandler
	downloads *downloader.Service
	user      *UserHandlers
}

// NewClipHandlers creates a new ClipHandlers instance.
func NewClipHandlers(base *BaseHandler, downloads *downloader.Service, user *UserHandlers) *ClipHandlers {
	return &ClipHandlers{BaseHandler: base, downloads: downloads, user: user}
}

// Clip handles "/clip <url> <start> <end> [hq] [meta]".
func (h *ClipHandlers) Clip(ctx context.Context, message *telego.Message) {
	h.handleClipCommand(ctx, message, false, false, false)
}

// Gif handles "/gif <url> <start> <end> [hq]" — clip without audio.
func (h *ClipHandlers) Gif(ctx context.Context, message *telego.Message) {
	h.handleClipCommand(ctx, message, true, false, false)
}

// Subs handles "/subs <url> <pairs...> [lang] [hq]" — clips with burned-in subtitles.
func (h *ClipHandlers) Subs(ctx context.Context, message *telego.Message) {
	h.handleClipCommand(ctx, message, false, true, false)
}

// MP3 handles "/mp3 <url> [start] [end]" — extract audio track as MP3.
func (h *ClipHandlers) MP3(ctx context.Context, message *telego.Message) {
	h.handleClipCommand(ctx, message, false, false, true)
}

// DirectLink downloads a TikTok or Shorts URL without requiring an explicit command,
// or provides usage hint for regular full videos.
func (h *ClipHandlers) DirectLink(ctx context.Context, message *telego.Message, rawURL string) {
	chatID := message.Chat.ID
	if downloader.IsDirectDownloadURL(rawURL) {
		req := downloader.ClipRequest{
			URL:    rawURL,
			Shorts: true,
		}
		statusID := h.sendStatus(ctx, chatID, fmt.Sprintf("⏳ Скачиваю %s…", rawURL))
		parsed := &downloader.ParsedCommand{URL: rawURL, Shorts: true}
		cbs := h.newCallbacks(chatID, statusID, parsed, req)
		if _, err := h.downloads.SubmitWithCallbacks(ctx, req, cbs); err != nil {
			_ = h.TG.EditMessageText(context.Background(), chatID, statusID, "❌ "+html.EscapeString(err.Error()))
		}
		return
	}

	// For standard long YouTube videos sent as raw links in private chats, provide a helpful quick guide.
	if message.Chat.Type == "private" {
		hint := fmt.Sprintf("🎬 Чтобы вырезать клип, укажите интервал:\n<code>/clip %s 0:10 0:30</code>\n\nИли извлечь аудио:\n<code>/mp3 %s 0:10 0:30</code>",
			html.EscapeString(rawURL), html.EscapeString(rawURL))
		_ = h.SendMessage(ctx, chatID, hint)
	}
}

// Help handles "/help <topic>" for clip-related topics; falls back to the
// generic help handler otherwise.
func (h *ClipHandlers) Help(ctx context.Context, message *telego.Message) {
	parts := strings.Fields(message.Text)
	if len(parts) >= 2 {
		topic := strings.TrimPrefix(parts[1], "/")
		var text string
		switch topic {
		case "clip":
			text = "<b>/clip</b> — вырезать клип из видео\n\n" +
				"<code>/clip &lt;url&gt; &lt;start&gt; &lt;end&gt; [hq]</code>\n\n" +
				"• Таймкоды: <code>SS.ms</code>, <code>MM:SS</code>, <code>MM:SS.ms</code>, <code>HH:MM:SS</code>\n" +
				"• <code>hq</code> — качество до 2К, но не длиннее " + fmt.Sprintf("%.0f", h.maxSeconds(true)) + " c\n" +
				"• Обычное качество — до " + fmt.Sprintf("%.0f", h.maxSeconds(false)/60) + " минут на клип\n" +
				"• Для Shorts и TikTok можно отправлять прямую ссылку без команды!\n" +
				"Пример: <code>/clip https://youtu.be/X56FLo6qslE 0:26 0:32</code>"
		case "gif":
			text = "<b>/gif</b> — вырезать клип без звука\n\n" +
				"<code>/gif &lt;url&gt; &lt;start&gt; &lt;end&gt; [hq]</code>\n\n" +
				"Аналогично /clip, но аудиодорожка удаляется.\n" +
				"Пример: <code>/gif https://youtu.be/dQw4w9WgXcQ 0:43 0:48</code>"
		case "subs":
			text = "<b>/subs</b> — вырезать клип с вшитыми субтитрами\n\n" +
				"<code>/subs &lt;url&gt; &lt;start&gt; &lt;end&gt; [...] [язык] [hq]</code>\n\n" +
				"• Несколько пар таймкодов — несколько клипов одной командой\n" +
				"• Язык по умолчанию — en; если трека нет, подтянется автоперевод (например ru-en)\n" +
				"Пример: <code>/subs https://youtu.be/r0u5URS3VXE 4:00 4:01 4:02 4:04 ru</code>"
		case "mp3":
			text = "<b>/mp3</b> — извлечь аудиодорожку из видео\n\n" +
				"<code>/mp3 &lt;url&gt; &lt;start&gt; &lt;end&gt;</code>\n\n" +
				"• Извлекает качественный MP3 (192 kbps)\n" +
				"• Для Shorts и TikTok можно без таймкодов\n" +
				"Пример: <code>/mp3 https://youtu.be/dQw4w9WgXcQ 0:43 1:15</code>"
		default:
			h.user.Help(ctx, message)
			return
		}
		_ = h.SendMessage(ctx, message.Chat.ID, text)
		return
	}
	h.user.Help(ctx, message)
}

// maxSeconds returns the configured per-tier clip length limit in seconds
// (minutes for the normal tier, used directly in the help text).
func (h *ClipHandlers) maxSeconds(hq bool) float64 {
	if h.downloads == nil {
		if hq {
			return 30
		}
		return 300
	}
	return h.downloads.MaxSegmentDurationSeconds(hq)
}

// handleClipCommand parses arguments and submits one job per interval,
// reporting progress by editing a per-job status message.
func (h *ClipHandlers) handleClipCommand(ctx context.Context, message *telego.Message, gif, subs, audioOnly bool) {
	chatID := message.Chat.ID

	args := strings.Fields(message.Text)[1:]
	parsed, err := downloader.ParseClipArgs(args)
	if err != nil {
		_ = h.SendMessage(ctx, chatID, "❌ "+html.EscapeString(err.Error()))
		return
	}
	parsed.GIF = gif
	parsed.AudioOnly = audioOnly

	for _, interval := range parsed.Intervals {
		req := downloader.ClipRequest{
			URL:       parsed.URL,
			Start:     interval.Start,
			End:       interval.End,
			HQ:        parsed.HQ,
			GIF:       parsed.GIF,
			Shorts:    parsed.Shorts,
			AudioOnly: parsed.AudioOnly,
			SubsLang:  "",
		}
		if subs && parsed.SubsLang != "" {
			req.SubsLang = parsed.SubsLang
		} else if subs {
			req.SubsLang = "en"
		}

		statusID := h.sendStatus(ctx, chatID, statusText(parsed.URL, interval.Start, interval.End))
		cbs := h.newCallbacks(chatID, statusID, parsed, req)

		if _, err := h.downloads.SubmitWithCallbacks(ctx, req, cbs); err != nil {
			_ = h.TG.EditMessageText(context.Background(), chatID, statusID, "❌ "+html.EscapeString(err.Error()))
		}
	}
}

// sendStatus posts a new status message and returns its ID, or -1 on failure.
func (h *ClipHandlers) sendStatus(ctx context.Context, chatID int64, text string) int {
	msg, err := h.TG.SendMessageRaw(ctx, chatID, text)
	if err != nil {
		h.Logger.Warn("failed to send status message", zap.Error(err))
		return -1
	}
	return msg.MessageID
}

func (h *ClipHandlers) newCallbacks(chatID int64, statusID int, parsed *downloader.ParsedCommand, req downloader.ClipRequest) *downloader.ClipCallbacks {
	edit := func(text string) {
		if statusID < 0 {
			return
		}
		_ = h.TG.EditMessageText(context.Background(), chatID, statusID, text)
	}
	return &downloader.ClipCallbacks{
		OnStage: func(stage, detail string) {
			switch stage {
			case downloader.StageMetadata:
				if detail == "" {
					edit("🔍 Extracting metadata…\n" + html.EscapeString(req.URL))
				} else {
					edit("🔍 Extracting metadata\n<b>" + html.EscapeString(detail) + "</b>")
				}
			case downloader.StageSubtitles:
				edit("💬 Extracting subtitles (" + html.EscapeString(detail) + ")…")
			case downloader.StageDownload:
				if req.AudioOnly {
					edit("⬇️ Downloading audio…")
				} else {
					edit("⬇️ Downloading video…")
				}
			case downloader.StageReencode:
				if req.AudioOnly {
					edit("🎵 Converting to MP3…")
				} else {
					edit("🎞 Video reencoding…")
				}
			case downloader.StageUpload:
				edit("📤 Uploading to Telegram…")
			}
		},
		OnProgress: func(stage string, percent int) {
			switch stage {
			case downloader.StageDownload:
				if req.AudioOnly {
					edit(fmt.Sprintf("⬇️ Downloading audio… %d%%", percent))
				} else {
					edit(fmt.Sprintf("⬇️ Downloading video… %d%%", percent))
				}
			case downloader.StageReencode:
				if req.AudioOnly {
					edit(fmt.Sprintf("🎵 Converting to MP3… %d%%", percent))
				} else {
					edit(fmt.Sprintf("🎞 Video reencoding… %d%%", percent))
				}
			}
		},
		OnDone: func(path, caption string) {
			edit("✅ Готово")
			bgCtx := context.Background()
			var sendErr error
			if req.AudioOnly {
				sendErr = h.TG.SendAudioFile(bgCtx, chatID, path, caption)
			} else {
				sendErr = h.TG.SendVideoFile(bgCtx, chatID, path, caption)
			}
			if sendErr != nil {
				h.Logger.Error("failed to send resulting media", zap.Error(sendErr))
				_ = h.SendMessage(bgCtx, chatID, "❌ Не удалось отправить файл: "+html.EscapeString(sendErr.Error()))
			}
			if statusID > 0 {
				_ = h.TG.DeleteMessage(bgCtx, chatID, statusID)
			}
		},
		OnError: func(errMsg string) {
			edit(errMsg)
		},
	}
}

func statusText(url, start, end string) string {
	if start == "" && end == "" {
		return fmt.Sprintf("⏳ Working on %s", url)
	}
	return fmt.Sprintf("⏳ Working on %s\n%s – %s", url, start, end)
}
