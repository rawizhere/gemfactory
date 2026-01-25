package handlers

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// AdminHandlers defines the logic for processing administrator-level commands.
type AdminHandlers struct {
	*BaseHandler
}

// NewAdminHandlers initializes a new AdminHandlers instance.
func NewAdminHandlers(base *BaseHandler) *AdminHandlers {
	return &AdminHandlers{BaseHandler: base}
}

// Admin displays the list of available administrative commands to authorized users.
func (h *AdminHandlers) Admin(ctx context.Context, message *tgbotapi.Message) {
	if !h.isAdmin(message.From) {
		h.sendMessage(message.Chat.ID, "You don't have admin permissions")
		return
	}

	text := "🔧 <b>Admin Commands:</b>\n\n" +
		"/add_artist [names] [-f|-m] - Add artists\n" +
		"/remove_artist [names] - Deactivate artists\n" +
		"/export - Export artist list\n" +
		"/config [key] [value] - Configuration\n" +
		"/parse [month] [year] - Run parser\n" +
		"/reload_playlist - Reload playlist"

	h.sendMessage(message.Chat.ID, text)
}

// AddArtist processes the command to register new artists in the system.
func (h *AdminHandlers) AddArtist(ctx context.Context, message *tgbotapi.Message) {
	if !h.isAdmin(message.From) {
		return
	}

	args := strings.Fields(message.CommandArguments())
	if len(args) < 2 {
		h.sendMessage(message.Chat.ID, "Usage: /add_artist <names> [-f|-m]")
		return
	}

	flag := args[len(args)-1]
	isFemale := flag == "-f"
	if flag != "-f" && flag != "-m" {
		h.sendMessage(message.Chat.ID, "Please specify gender flag: -f or -m")
		return
	}

	artistNamesStr := strings.Join(args[:len(args)-1], " ")
	artistNames := h.parseArtistList(artistNamesStr)

	count, err := h.Services.Artist.Add(ctx, artistNames, isFemale)
	if err != nil {
		h.handleError(message.Chat.ID, err, "Failed to add artists")
		return
	}

	h.sendMessage(message.Chat.ID, fmt.Sprintf("✅ Added artists: %d", count))
}

// RemoveArtist processes the command to deactivate artists.
func (h *AdminHandlers) RemoveArtist(ctx context.Context, message *tgbotapi.Message) {
	if !h.isAdmin(message.From) {
		return
	}

	args := strings.Fields(message.CommandArguments())
	if len(args) < 1 {
		h.sendMessage(message.Chat.ID, "Usage: /remove_artist <names>")
		return
	}

	artistNamesStr := strings.Join(args, " ")
	artistNames := h.parseArtistList(artistNamesStr)

	count, err := h.Services.Artist.Deactivate(ctx, artistNames)
	if err != nil {
		h.handleError(message.Chat.ID, err, "Failed to deactivate artists")
		return
	}

	h.sendMessage(message.Chat.ID, fmt.Sprintf("✅ Deactivated artists: %d", count))
}

// Parse triggers a manual synchronization of releases for a specific month and year.
func (h *AdminHandlers) Parse(ctx context.Context, message *tgbotapi.Message) {
	if !h.isAdmin(message.From) {
		return
	}

	args := strings.Fields(message.CommandArguments())
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

	h.sendMessage(message.Chat.ID, fmt.Sprintf("🔄 Running parser for %s %d...", month, year))

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		monthQuery := fmt.Sprintf("%s-%d", month, year)
		count, err := h.Services.Release.ParseReleasesForMonth(ctx, monthQuery)
		if err != nil {
			h.handleError(message.Chat.ID, err, "Parser error")
			return
		}
		h.sendMessage(message.Chat.ID, fmt.Sprintf("✅ Parsing complete. Found %d releases", count))
	}()
}

// Export generates and sends a formatted list of all registered artists.
func (h *AdminHandlers) Export(ctx context.Context, message *tgbotapi.Message) {
	if !h.isAdmin(message.From) {
		return
	}

	response, err := h.Services.Artist.Export(ctx)
	if err != nil {
		h.handleError(message.Chat.ID, err, "Export error")
		return
	}

	h.sendMessage(message.Chat.ID, response)
}

// parseArtistList converts a comma-separated string into a slice of cleaned artist names.
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
