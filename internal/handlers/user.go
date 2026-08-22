package handlers

import (
	"context"
	"fmt"
	"strings"

	"github.com/mymmrac/telego"
)

type UserHandlers struct {
	*BaseHandler
}

func NewUserHandlers(base *BaseHandler) *UserHandlers {
	return &UserHandlers{BaseHandler: base}
}

func (h *UserHandlers) Start(ctx context.Context, message *telego.Message) {
	text := "Welcome! Please choose a month:"
	_ = h.SendMessageWithMarkup(ctx, message.Chat.ID, text, h.GetMainKeyboard())
}

func (h *UserHandlers) Help(ctx context.Context, message *telego.Message) {
	text := "<b>Commands:</b>\n\n" +
		"/clip &lt;url&gt; &lt;start&gt; &lt;end&gt; [hq] - Cut video clip\n" +
		"/gif &lt;url&gt; &lt;start&gt; &lt;end&gt; [hq] - Cut clip without audio\n" +
		"/subs &lt;url&gt; &lt;start&gt; &lt;end&gt; [lang] [hq] - Cut clip with subtitles\n" +
		"/mp3 &lt;url&gt; [start] [end] - Extract audio track\n" +
		"/month [month] [year] [-f|-m] - Releases for a month\n" +
		"/search &lt;artist&gt; - Search releases by artist\n" +
		"/artists - List tracked artists\n" +
		"/metrics - Show system metrics\n" +
		"\nDirect links: Send TikTok or YouTube Shorts directly.\n" +
		"Topic help: <code>/help clip</code>, <code>/help subs</code>, etc.\n\n" +
		fmt.Sprintf("Admin: @%s", h.Config.AdminUsername)
	_ = h.SendMessageWithMarkup(ctx, message.Chat.ID, text, h.GetMainKeyboard())
}

func (h *UserHandlers) Month(ctx context.Context, message *telego.Message) {
	parts := strings.Fields(message.Text)
	if len(parts) < 2 {
		_ = h.SendMessageWithMarkup(ctx, message.Chat.ID, "Please choose a month:", h.GetMainKeyboard())
		return
	}

	args := parts[1:]
	month := strings.ToLower(args[0])
	femaleOnly := false
	maleOnly := false
	year := ""

	for i, arg := range args[1:] {
		switch arg {
		case "-f":
			femaleOnly = true
		case "-m":
			maleOnly = true
		default:
			if year == "" && i == 0 {
				year = arg
			}
		}
	}

	monthQuery := month
	if year != "" {
		monthQuery = fmt.Sprintf("%s-%s", month, year)
	}

	response, err := h.Services.Release.GetReleasesForMonth(ctx, monthQuery, femaleOnly, maleOnly)
	if err != nil {
		h.HandleError(ctx, message.Chat.ID, err, "Error retrieving releases")
		return
	}

	_ = h.SendMessageWithMarkup(ctx, message.Chat.ID, response, h.GetMainKeyboard())
}

func (h *UserHandlers) Artists(ctx context.Context, message *telego.Message) {
	response, err := h.Services.Artist.FormatList(ctx)
	if err != nil {
		h.HandleError(ctx, message.Chat.ID, err, "Error retrieving artist list")
		return
	}
	_ = h.SendMessageWithMarkup(ctx, message.Chat.ID, response, h.GetMainKeyboard())
}

func (h *UserHandlers) Search(ctx context.Context, message *telego.Message) {
	parts := strings.SplitN(message.Text, " ", 2)
	if len(parts) < 2 {
		_ = h.SendMessage(ctx, message.Chat.ID, "Usage: /search <artist_name>")
		return
	}

	artistName := strings.TrimSpace(parts[1])
	response, err := h.Services.Release.GetByArtist(ctx, artistName)
	if err != nil {
		h.HandleError(ctx, message.Chat.ID, err, "Search error")
		return
	}
	_ = h.SendMessage(ctx, message.Chat.ID, response)
}

func (h *UserHandlers) Metrics(ctx context.Context, message *telego.Message) {
	fCount, mCount, tCount, _ := h.Services.Artist.GetCounts(ctx)
	rCount, _ := h.Services.Release.GetTotalReleaseCount(ctx)

	text := fmt.Sprintf("<b>GemFactory Metrics:</b>\n\n"+
		"Total Artists: %d\n"+
		"Female: %d\n"+
		"Male: %d\n\n"+
		"Total Releases: %d",
		tCount, fCount, mCount, rCount)

	_ = h.SendMessage(ctx, message.Chat.ID, text)
}
