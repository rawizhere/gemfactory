package handlers

import (
	"context"
	"fmt"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// HomeworkHandlers defines the logic for managing user homework assignments and playlist info.
type HomeworkHandlers struct {
	*BaseHandler
}

// NewHomeworkHandlers initializes a new HomeworkHandlers instance.
func NewHomeworkHandlers(base *BaseHandler) *HomeworkHandlers {
	return &HomeworkHandlers{BaseHandler: base}
}

// Homework allows a user to request a new random music assignment for the day.
func (h *HomeworkHandlers) Homework(ctx context.Context, message *tgbotapi.Message) {
	userID := message.From.ID

	canRequest, err := h.Services.Homework.CanRequest(ctx, userID)
	if err != nil {
		h.handleError(message.Chat.ID, err, "Error during verification.")
		return
	}

	if !canRequest {
		timeUntilNext := h.Services.Homework.GetTimeUntilNext(ctx, userID)
		activeHomework, _ := h.Services.Homework.GetActive(ctx, userID)

		text := fmt.Sprintf("⏰ You've already received an assignment today! Next one in %0.f min.", timeUntilNext.Minutes())
		if activeHomework != nil {
			text += fmt.Sprintf("\n\n📚 Current: %s - %s (%d times)", activeHomework.Artist, activeHomework.Title, activeHomework.PlayCount)
		}
		h.sendMessage(message.Chat.ID, text)
		return
	}

	homework, err := h.Services.Homework.GetRandom(ctx, userID)
	if err != nil {
		h.handleError(message.Chat.ID, err, "No more assignments available today.")
		return
	}

	text := fmt.Sprintf("📚 <b>Your Homework:</b>\n\n"+
		"🎤 Artist: %s\n"+
		"🎵 Track: %s\n"+
		"🎧 Listen: %d time(s)\n\n"+
		"🔗 <a href=\"https://open.spotify.com/track/%s\">Open in Spotify</a>",
		homework.Artist, homework.Title, homework.PlayCount, homework.TrackID)

	h.sendMessageWithMarkup(message.Chat.ID, text, h.getMainKeyboard())
}

// Playlist displays metadata and a link to the curated Spotify playlist.
func (h *HomeworkHandlers) Playlist(ctx context.Context, message *tgbotapi.Message) {
	info, err := h.Services.Playlist.GetInfo(ctx)
	if err != nil {
		h.handleError(message.Chat.ID, err, "Error getting playlist info")
		return
	}

	text := fmt.Sprintf("🎵 <b>GemFactory Playlist:</b>\n\n"+
		"📝 Name: %s\n"+
		"🎶 Tracks: %d\n"+
		"👤 Owner: %s\n\n"+
		"🔗 <a href=\"%s\">Open in Spotify</a>",
		info.Name, info.TrackCount, info.Owner, h.Config.PlaylistURL)

	h.sendMessageWithMarkup(message.Chat.ID, text, h.getMainKeyboard())
}
