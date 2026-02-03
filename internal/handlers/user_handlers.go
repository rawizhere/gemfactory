package handlers

import (
	"context"
	"fmt"
	"strings"

	"github.com/mymmrac/telego"
)

// UserHandlers handles standard user-facing commands.
type UserHandlers struct {
	*BaseHandler
}

// NewUserHandlers creates a new UserHandlers instance.
func NewUserHandlers(base *BaseHandler) *UserHandlers {
	return &UserHandlers{BaseHandler: base}
}

// Start processes the /start command.
func (h *UserHandlers) Start(ctx context.Context, message *telego.Message) {
	text := "Welcome! Please choose a month:"
	_ = h.SendMessageWithMarkup(ctx, message.Chat.ID, text, h.GetMainKeyboard())
}

// Help processes the /help command.
func (h *UserHandlers) Help(ctx context.Context, message *telego.Message) {
	text := "Available commands:\n" +
		"\n/start - Start the bot\n" +
		"/help - Show this message\n" +
		"/month [month] - Releases for the current year\n" +
		"/month [month] [year] - Releases for the selected period\n" +
		"/search [artist] - Search by artist\n" +
		"/artists - Artist lists\n" +
		"/metrics - System metrics\n" +
		"/homework - Daily homework\n" +
		"/playlist - Playlist link\n" +
		"\n" +
		fmt.Sprintf("Admin: @%s", h.Config.AdminUsername)
	_ = h.SendMessageWithMarkup(ctx, message.Chat.ID, text, h.GetMainKeyboard())
}

// Month handles release list requests for a specific period.
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

// Artists shows active artists.
func (h *UserHandlers) Artists(ctx context.Context, message *telego.Message) {
	response, err := h.Services.Artist.FormatList(ctx)
	if err != nil {
		h.HandleError(ctx, message.Chat.ID, err, "Error retrieving artist list")
		return
	}
	_ = h.SendMessageWithMarkup(ctx, message.Chat.ID, response, h.GetMainKeyboard())
}

// Search finds releases by artist.
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

// Metrics displays system and collection statistics.
func (h *UserHandlers) Metrics(ctx context.Context, message *telego.Message) {
	fCount, mCount, tCount, _ := h.Services.Artist.GetCounts(ctx)
	rCount, _ := h.Services.Release.GetTotalReleaseCount(ctx)

	text := fmt.Sprintf("📊 <b>GemFactory Metrics:</b>\n\n"+
		"🎤 Total Artists: %d\n"+
		"💃 Female: %d\n"+
		"🕺 Male: %d\n\n"+
		"🎵 Total Releases: %d",
		tCount, fCount, mCount, rCount)

	_ = h.SendMessage(ctx, message.Chat.ID, text)
}
