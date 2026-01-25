package handlers

import (
	"context"
	"fmt"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// UserHandlers defines the logic for processing standard user-facing commands.
type UserHandlers struct {
	*BaseHandler
}

// NewUserHandlers initializes a new UserHandlers instance.
func NewUserHandlers(base *BaseHandler) *UserHandlers {
	return &UserHandlers{BaseHandler: base}
}

// Start processes the /start command, greeting the user and presenting the initial menu.
func (h *UserHandlers) Start(ctx context.Context, message *tgbotapi.Message) {
	text := "Welcome! Please choose a month:"
	h.sendMessageWithMarkup(message.Chat.ID, text, h.getMainKeyboard())
}

// Help processes the /help command, detailing all available bot features.
func (h *UserHandlers) Help(ctx context.Context, message *tgbotapi.Message) {
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
	h.sendMessageWithMarkup(message.Chat.ID, text, h.getMainKeyboard())
}

// Month processes requests for release lists for a specific month and optional year.
func (h *UserHandlers) Month(ctx context.Context, message *tgbotapi.Message) {
	args := strings.Fields(message.CommandArguments())
	if len(args) == 0 {
		h.sendMessageWithMarkup(message.Chat.ID, "Please choose a month:", h.getMainKeyboard())
		return
	}

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
		h.handleError(message.Chat.ID, err, "Error retrieving releases")
		return
	}

	h.sendMessageWithMarkup(message.Chat.ID, response, h.getMainKeyboard())
}

// Artists processes the command to display a formatted list of all active artists.
func (h *UserHandlers) Artists(ctx context.Context, message *tgbotapi.Message) {
	response, err := h.Services.Artist.FormatList(ctx)
	if err != nil {
		h.handleError(message.Chat.ID, err, "Error retrieving artist list")
		return
	}
	h.sendMessageWithMarkup(message.Chat.ID, response, h.getMainKeyboard())
}

// Search processes the command to find all releases associated with a specific artist name.
func (h *UserHandlers) Search(ctx context.Context, message *tgbotapi.Message) {
	artistName := message.CommandArguments()
	if artistName == "" {
		h.sendMessage(message.Chat.ID, "Usage: /search <artist_name>")
		return
	}

	response, err := h.Services.Release.GetByArtist(ctx, artistName)
	if err != nil {
		h.handleError(message.Chat.ID, err, "Search error")
		return
	}
	h.sendMessage(message.Chat.ID, response)
}

// Metrics processes the command to display aggregate system and collection statistics.
func (h *UserHandlers) Metrics(ctx context.Context, message *tgbotapi.Message) {
	fCount, mCount, tCount, _ := h.Services.Artist.GetCounts(ctx)
	rCount, _ := h.Services.Release.GetTotalReleaseCount(ctx)

	text := fmt.Sprintf("📊 <b>GemFactory Metrics:</b>\n\n"+
		"🎤 Total Artists: %d\n"+
		"💃 Female: %d\n"+
		"🕺 Male: %d\n\n"+
		"🎵 Total Releases: %d",
		tCount, fCount, mCount, rCount)

	h.sendMessage(message.Chat.ID, text)
}
