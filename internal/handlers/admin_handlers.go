package handlers

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/mymmrac/telego"
)

// AdminHandlers handles administrator-level commands.
type AdminHandlers struct {
	*BaseHandler
}

// NewAdminHandlers initializes a new AdminHandlers instance.
func NewAdminHandlers(base *BaseHandler) *AdminHandlers {
	return &AdminHandlers{BaseHandler: base}
}

// Admin shows available administrative commands.
func (h *AdminHandlers) Admin(ctx context.Context, message *telego.Message) {
	if !h.IsAdmin(message.From) {
		_ = h.SendMessage(ctx, message.Chat.ID, "You don't have admin permissions")
		return
	}

	text := "🔧 <b>Admin Commands:</b>\n\n" +
		"/add_artist [names] [-f|-m] - Add artists\n" +
		"/remove_artist [names] - Deactivate artists\n" +
		"/export - Export artist list\n" +
		"/config [key] [value] - Configuration\n" +
		"/parse [month] [year] - Run parser\n" +
		"/reload_playlist - Reload playlist"

	_ = h.SendMessage(ctx, message.Chat.ID, text)
}

// AddArtist registers new artists.
func (h *AdminHandlers) AddArtist(ctx context.Context, message *telego.Message) {
	if !h.IsAdmin(message.From) {
		return
	}

	parts := strings.Fields(message.Text)
	if len(parts) < 3 {
		_ = h.SendMessage(ctx, message.Chat.ID, "Usage: /add_artist <names> [-f|-m]")
		return
	}

	args := parts[1:]
	flag := args[len(args)-1]
	isFemale := flag == "-f"
	if flag != "-f" && flag != "-m" {
		_ = h.SendMessage(ctx, message.Chat.ID, "Please specify gender flag: -f or -m")
		return
	}

	artistNamesStr := strings.Join(args[:len(args)-1], " ")
	artistNames := h.parseArtistList(artistNamesStr)

	count, err := h.Services.Artist.Add(ctx, artistNames, isFemale)
	if err != nil {
		h.HandleError(ctx, message.Chat.ID, err, "Failed to add artists")
		return
	}

	_ = h.SendMessage(ctx, message.Chat.ID, fmt.Sprintf("✅ Added artists: %d", count))
}

// RemoveArtist deactivates artists.
func (h *AdminHandlers) RemoveArtist(ctx context.Context, message *telego.Message) {
	if !h.IsAdmin(message.From) {
		return
	}

	parts := strings.Fields(message.Text)
	if len(parts) < 2 {
		_ = h.SendMessage(ctx, message.Chat.ID, "Usage: /remove_artist <names>")
		return
	}

	artistNamesStr := strings.Join(parts[1:], " ")
	artistNames := h.parseArtistList(artistNamesStr)

	count, err := h.Services.Artist.Deactivate(ctx, artistNames)
	if err != nil {
		h.HandleError(ctx, message.Chat.ID, err, "Failed to deactivate artists")
		return
	}

	_ = h.SendMessage(ctx, message.Chat.ID, fmt.Sprintf("✅ Deactivated artists: %d", count))
}

// Parse triggers manual release sync.
func (h *AdminHandlers) Parse(ctx context.Context, message *telego.Message) {
	if !h.IsAdmin(message.From) {
		return
	}

	args := strings.Fields(message.Text)[1:]
	month := strings.ToLower(time.Now().Format("January"))
	year := time.Now().Year()

	if len(args) >= 1 {
		month = strings.ToLower(args[0])
	}
	if len(args) >= 2 {
		if y, err := strconv.Atoi(args[1]); err == nil {
			year = y
		}
	}

	_ = h.SendMessage(ctx, message.Chat.ID, fmt.Sprintf("🔄 Running parser for %s %d...", month, year))

	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		monthQuery := fmt.Sprintf("%s-%d", month, year)
		count, err := h.Services.Release.ParseReleasesForMonth(bgCtx, monthQuery)
		if err != nil {
			h.HandleError(bgCtx, message.Chat.ID, err, "Parser error")
			return
		}
		_ = h.SendMessage(bgCtx, message.Chat.ID, fmt.Sprintf("✅ Parsing complete. Found %d releases", count))
	}()
}

// Export sends the list of registered artists.
func (h *AdminHandlers) Export(ctx context.Context, message *telego.Message) {
	if !h.IsAdmin(message.From) {
		return
	}

	response, err := h.Services.Artist.Export(ctx)
	if err != nil {
		h.HandleError(ctx, message.Chat.ID, err, "Export error")
		return
	}

	_ = h.SendMessage(ctx, message.Chat.ID, response)
}

// parseArtistList converts a comma-separated string into a slice of names.
func (h *AdminHandlers) parseArtistList(input string) []string {
	parts := strings.Split(input, ",")
	var result []string
	for _, p := range parts {
		if s := strings.TrimSpace(p); s != "" {
			result = append(result, s)
		}
	}
	return result
}
