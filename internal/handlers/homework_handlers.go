package handlers

import (
	"context"
	"fmt"

	"github.com/mymmrac/telego"
)

// HomeworkHandlers handles homework assignments and playlist info.
type HomeworkHandlers struct {
	*BaseHandler
}

// NewHomeworkHandlers initializes a new HomeworkHandlers instance.
func NewHomeworkHandlers(base *BaseHandler) *HomeworkHandlers {
	return &HomeworkHandlers{BaseHandler: base}
}

// Homework handles music assignment requests.
func (h *HomeworkHandlers) Homework(ctx context.Context, message *telego.Message) {
	userID := message.From.ID

	canRequest, err := h.Services.Homework.CanRequest(ctx, userID)
	if err != nil {
		h.HandleError(ctx, message.Chat.ID, err, "Verification error")
		return
	}

	if !canRequest {
		timeUntilNext := h.Services.Homework.GetTimeUntilNext(ctx, userID)
		activeHomework, _ := h.Services.Homework.GetActive(ctx, userID)

		text := fmt.Sprintf("⏰ Next assignment in %0.f min.", timeUntilNext.Minutes())
		if activeHomework != nil {
			text += fmt.Sprintf("\n\n📚 Current: %s - %s (%d times)", activeHomework.Artist, activeHomework.Title, activeHomework.PlayCount)
		}
		_ = h.SendMessage(ctx, message.Chat.ID, text)
		return
	}

	homework, err := h.Services.Homework.GetRandom(ctx, userID)
	if err != nil {
		h.HandleError(ctx, message.Chat.ID, err, "No assignments available")
		return
	}

	text := fmt.Sprintf("📚 <b>Your Homework:</b>\n\n"+
		"🎤 Artist: %s\n"+
		"🎵 Track: %s\n"+
		"🎧 Listen: %d time(s)\n\n"+
		"🔗 <a href=\"https://open.spotify.com/track/%s\">Spotify</a>",
		homework.Artist, homework.Title, homework.PlayCount, homework.TrackID)

	_ = h.SendMessageWithMarkup(ctx, message.Chat.ID, text, h.GetMainKeyboard())
}

// Playlist sends the playlist link.
func (h *HomeworkHandlers) Playlist(ctx context.Context, message *telego.Message) {
	text := "🎵 <b>GemFactory Playlist:</b>\n" + h.Config.PlaylistURL
	_ = h.SendMessage(ctx, message.Chat.ID, text)
}
