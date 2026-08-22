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

type ClipHandlers struct {
	*BaseHandler
	downloads *downloader.Service
	user      *UserHandlers
}

func NewClipHandlers(base *BaseHandler, downloads *downloader.Service, user *UserHandlers) *ClipHandlers {
	return &ClipHandlers{BaseHandler: base, downloads: downloads, user: user}
}

func (h *ClipHandlers) Clip(ctx context.Context, message *telego.Message) {
	h.handleClipCommand(ctx, message, false, false, false)
}

func (h *ClipHandlers) Gif(ctx context.Context, message *telego.Message) {
	h.handleClipCommand(ctx, message, true, false, false)
}

func (h *ClipHandlers) Subs(ctx context.Context, message *telego.Message) {
	h.handleClipCommand(ctx, message, false, true, false)
}

func (h *ClipHandlers) MP3(ctx context.Context, message *telego.Message) {
	h.handleClipCommand(ctx, message, false, false, true)
}

func (h *ClipHandlers) DirectLink(ctx context.Context, message *telego.Message, rawURL string) {
	chatID := message.Chat.ID
	if downloader.IsDirectDownloadURL(rawURL) {
		req := downloader.ClipRequest{
			URL:    rawURL,
			Shorts: true,
		}
		statusID := h.sendStatus(ctx, chatID, initialStatusCard(req), message.MessageID)
		parsed := &downloader.ParsedCommand{URL: rawURL, Shorts: true}
		cbs := h.newCallbacks(chatID, statusID, parsed, req, message.MessageID)
		if _, err := h.downloads.SubmitWithCallbacks(ctx, req, cbs); err != nil {
			_ = h.TG.EditMessageText(context.Background(), chatID, statusID, "Error: "+html.EscapeString(err.Error()))
		}
		return
	}

	if message.Chat.Type == "private" {
		hint := fmt.Sprintf("To cut a clip, specify a time interval:\n<code>/clip %s 0:10 0:30</code>\n\nOr extract audio:\n<code>/mp3 %s 0:10 0:30</code>",
			html.EscapeString(rawURL), html.EscapeString(rawURL))
		_ = h.SendMessage(ctx, chatID, hint)
	}
}

func (h *ClipHandlers) Help(ctx context.Context, message *telego.Message) {
	parts := strings.Fields(message.Text)
	if len(parts) >= 2 {
		topic := strings.TrimPrefix(parts[1], "/")
		var text string
		switch topic {
		case "clip":
			text = "<b>/clip</b> — Cut video clip\n\n" +
				"<code>/clip &lt;url&gt; &lt;start&gt; &lt;end&gt; [hq]</code>\n\n" +
				"• Timecodes: <code>SS.ms</code>, <code>MM:SS</code>, <code>MM:SS.ms</code>, <code>HH:MM:SS</code>\n" +
				"• <code>hq</code> — Quality up to 2K, max " + fmt.Sprintf("%.0f", h.maxSeconds(true)) + "s\n" +
				"• Standard quality — Up to " + fmt.Sprintf("%.0f", h.maxSeconds(false)/60) + " minutes per clip\n" +
				"• Direct links: Send YouTube Shorts or TikTok directly without commands\n" +
				"Example: <code>/clip https://youtu.be/X56FLo6qslE 0:26 0:32</code>"
		case "gif":
			text = "<b>/gif</b> — Cut clip without audio\n\n" +
				"<code>/gif &lt;url&gt; &lt;start&gt; &lt;end&gt; [hq]</code>\n\n" +
				"Same as /clip, but the audio track is removed.\n" +
				"Example: <code>/gif https://youtu.be/dQw4w9WgXcQ 0:43 0:48</code>"
		case "subs":
			text = "<b>/subs</b> — Cut clip with burned-in subtitles\n\n" +
				"<code>/subs &lt;url&gt; &lt;start&gt; &lt;end&gt; [...] [lang] [hq]</code>\n\n" +
				"• Multiple timecode pairs produce multiple clips in one command\n" +
				"• Default language is en; auto-translated if not available\n" +
				"Example: <code>/subs https://youtu.be/r0u5URS3VXE 4:00 4:01 4:02 4:04 en</code>"
		case "mp3":
			text = "<b>/mp3</b> — Extract audio track from video\n\n" +
				"<code>/mp3 &lt;url&gt; [start] [end]</code>\n\n" +
				"• Extracts high-quality MP3 (192 kbps)\n" +
				"• Timecodes are optional for Shorts and TikTok\n" +
				"Example: <code>/mp3 https://youtu.be/dQw4w9WgXcQ 0:43 1:15</code>"
		default:
			h.user.Help(ctx, message)
			return
		}
		_ = h.SendMessage(ctx, message.Chat.ID, text)
		return
	}
	h.user.Help(ctx, message)
}

func (h *ClipHandlers) maxSeconds(hq bool) float64 {
	if h.downloads == nil {
		if hq {
			return 30
		}
		return 300
	}
	return h.downloads.MaxSegmentDurationSeconds(hq)
}

func (h *ClipHandlers) handleClipCommand(ctx context.Context, message *telego.Message, gif, subs, audioOnly bool) {
	chatID := message.Chat.ID

	args := strings.Fields(message.Text)[1:]
	parsed, err := downloader.ParseClipArgs(args)
	if err != nil {
		_ = h.SendMessage(ctx, chatID, "Error: "+html.EscapeString(err.Error()))
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

		statusText := initialStatusCard(req)
		statusID := h.sendStatus(ctx, chatID, statusText, message.MessageID)
		cbs := h.newCallbacks(chatID, statusID, parsed, req, message.MessageID)

		if _, err := h.downloads.SubmitWithCallbacks(ctx, req, cbs); err != nil {
			_ = h.TG.EditMessageText(context.Background(), chatID, statusID, "Error: "+html.EscapeString(err.Error()))
		}
	}
}

func (h *ClipHandlers) sendStatus(ctx context.Context, chatID int64, text string, replyToMsgID ...int) int {
	msg, err := h.TG.SendMessageRaw(ctx, chatID, text, replyToMsgID...)
	if err != nil {
		h.Logger.Warn("failed to send status message", zap.Error(err))
		return -1
	}
	return msg.MessageID
}

type clipTaskState struct {
	title      string
	interval   string
	mode       string
	subsLang   string
	audioOnly  bool
	gif        bool
	stage      string
	percent    int
	statusLine string
}

func (h *ClipHandlers) newCallbacks(chatID int64, statusID int, parsed *downloader.ParsedCommand, req downloader.ClipRequest, replyToMsgID int) *downloader.ClipCallbacks {
	state := clipTaskState{
		interval:   formatInterval(req),
		mode:       formatMode(req),
		subsLang:   req.SubsLang,
		audioOnly:  req.AudioOnly,
		gif:        req.GIF,
		statusLine: "Initializing...",
	}

	edit := func() {
		if statusID < 0 {
			return
		}
		_ = h.TG.EditMessageText(context.Background(), chatID, statusID, renderStatusCard(state))
	}

	return &downloader.ClipCallbacks{
		OnStage: func(stage, detail string) {
			state.stage = stage
			switch stage {
			case downloader.StageMetadata:
				if detail != "" {
					state.title = detail
					state.statusLine = "Extracting metadata..."
				} else {
					state.statusLine = "Extracting metadata..."
				}
			case downloader.StageSubtitles:
				state.statusLine = fmt.Sprintf("Extracting subtitles (%s)...", detail)
			case downloader.StageDownload:
				state.percent = 0
				if state.audioOnly {
					state.statusLine = "<b>Download:</b> Downloading audio..."
				} else {
					state.statusLine = "<b>Download:</b> Downloading video..."
				}
			case downloader.StageReencode:
				state.percent = 0
				if state.subsLang != "" {
					state.statusLine = fmt.Sprintf("<b>Re-encoding:</b> Burning subtitles (%s)...", state.subsLang)
				} else if state.audioOnly {
					state.statusLine = "<b>Converting:</b> Extracting MP3 (192 kbps)..."
				} else if state.gif {
					state.statusLine = "<b>Re-encoding:</b> Generating GIF..."
				} else {
					state.statusLine = "<b>Re-encoding:</b> Processing video..."
				}
			case downloader.StageUpload:
				state.percent = 0
				state.statusLine = "<b>Upload:</b> Uploading to Telegram..."
			}
			edit()
		},
		OnProgress: func(p downloader.ProgressUpdate) {
			state.stage = p.Stage
			state.percent = p.Percent
			switch p.Stage {
			case downloader.StageDownload:
				if p.Size != "" && p.Speed != "" && p.ETA != "" {
					state.statusLine = fmt.Sprintf("<b>Download:</b> %s (%s • ETA %s)", p.Size, p.Speed, p.ETA)
				} else if p.Size != "" && p.Speed != "" {
					state.statusLine = fmt.Sprintf("<b>Download:</b> %s (%s)", p.Size, p.Speed)
				} else if p.Size != "" {
					state.statusLine = fmt.Sprintf("<b>Download:</b> %s", p.Size)
				} else if p.Speed != "" {
					state.statusLine = fmt.Sprintf("<b>Download:</b> %d%% (%s)", p.Percent, p.Speed)
				} else {
					state.statusLine = fmt.Sprintf("<b>Download:</b> %d%%", p.Percent)
				}
			case downloader.StageReencode:
				if state.subsLang != "" {
					if p.Speed != "" {
						state.statusLine = fmt.Sprintf("<b>Re-encoding:</b> Burning subtitles (%s • %s)", state.subsLang, p.Speed)
					} else {
						state.statusLine = fmt.Sprintf("<b>Re-encoding:</b> Burning subtitles (%s)", state.subsLang)
					}
				} else if state.audioOnly {
					if p.Speed != "" {
						state.statusLine = fmt.Sprintf("<b>Converting:</b> Extracting MP3 (%s)", p.Speed)
					} else {
						state.statusLine = "<b>Converting:</b> Extracting MP3 (192 kbps)..."
					}
				} else if state.gif {
					if p.Speed != "" {
						state.statusLine = fmt.Sprintf("<b>Re-encoding:</b> Generating GIF (%s)", p.Speed)
					} else {
						state.statusLine = "<b>Re-encoding:</b> Generating GIF..."
					}
				} else {
					if p.Speed != "" {
						state.statusLine = fmt.Sprintf("<b>Re-encoding:</b> Processing video (%s)", p.Speed)
					} else {
						state.statusLine = "<b>Re-encoding:</b> Processing video..."
					}
				}
			}
			edit()
		},
		OnDone: func(path, caption string) {
			state.stage = downloader.StageUpload
			state.percent = 0
			state.statusLine = "<b>Upload:</b> Uploading to Telegram..."
			edit()

			bgCtx := context.Background()
			onUploadProgress := func(pct int, speed, sizeStr string) {
				state.stage = downloader.StageUpload
				state.percent = pct
				if speed != "" {
					state.statusLine = fmt.Sprintf("<b>Upload:</b> %s (%s)", sizeStr, speed)
				} else {
					state.statusLine = fmt.Sprintf("<b>Upload:</b> %s", sizeStr)
				}
				edit()
			}

			var sendErr error
			if req.AudioOnly {
				sendErr = h.TG.SendAudioFile(bgCtx, chatID, path, caption, onUploadProgress, replyToMsgID)
			} else {
				sendErr = h.TG.SendVideoFile(bgCtx, chatID, path, caption, onUploadProgress, replyToMsgID)
			}
			if sendErr != nil {
				h.Logger.Error("failed to send resulting media", zap.Error(sendErr))
				_ = h.SendMessage(bgCtx, chatID, "Failed to send file: "+html.EscapeString(sendErr.Error()))
			}
			if statusID > 0 {
				_ = h.TG.DeleteMessage(bgCtx, chatID, statusID)
			}
		},
		OnError: func(errMsg string) {
			if statusID < 0 {
				return
			}
			_ = h.TG.EditMessageText(context.Background(), chatID, statusID, errMsg)
		},
	}
}

func initialStatusCard(req downloader.ClipRequest) string {
	state := clipTaskState{
		interval:   formatInterval(req),
		mode:       formatMode(req),
		statusLine: "Processing " + html.EscapeString(req.URL) + "...",
	}
	return renderStatusCard(state)
}

func renderStatusCard(state clipTaskState) string {
	var sb strings.Builder
	if state.title != "" {
		sb.WriteString("<b>" + html.EscapeString(state.title) + "</b>\n")
	}

	var metaParts []string
	if state.interval != "" {
		metaParts = append(metaParts, "<code>"+html.EscapeString(state.interval)+"</code>")
	}
	if state.mode != "" {
		metaParts = append(metaParts, "<b>"+html.EscapeString(state.mode)+"</b>")
	}
	if len(metaParts) > 0 {
		sb.WriteString(strings.Join(metaParts, " • ") + "\n\n")
	} else if state.title != "" {
		sb.WriteString("\n")
	}

	if (state.stage == downloader.StageDownload || state.stage == downloader.StageReencode || state.stage == downloader.StageUpload) ||
		(state.percent > 0 && state.percent <= 100) {
		sb.WriteString("<code>" + progressBar(state.percent, 16) + "</code>\n")
	}

	if state.statusLine != "" {
		sb.WriteString(state.statusLine)
	}
	return sb.String()
}

func progressBar(percent, length int) string {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	if length <= 0 {
		length = 16
	}
	filled := percent * length / 100
	empty := length - filled
	return "[" + strings.Repeat("█", filled) + strings.Repeat("░", empty) + fmt.Sprintf("] %d%%", percent)
}

func formatMode(req downloader.ClipRequest) string {
	switch {
	case req.AudioOnly:
		return "MP3"
	case req.GIF:
		if req.HQ {
			return "GIF (HQ)"
		}
		return "GIF"
	case req.SubsLang != "":
		if req.HQ {
			return fmt.Sprintf("Subtitles (%s, HQ)", req.SubsLang)
		}
		return fmt.Sprintf("Subtitles (%s)", req.SubsLang)
	case req.HQ:
		return "Clip (HQ 2K)"
	case req.Shorts:
		return ""
	default:
		return "Clip"
	}
}

func formatInterval(req downloader.ClipRequest) string {
	if req.Start != "" && req.End != "" {
		return fmt.Sprintf("%s – %s", req.Start, req.End)
	}
	return ""
}
