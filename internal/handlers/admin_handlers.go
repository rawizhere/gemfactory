package handlers

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/mymmrac/telego"
)

// AdminHandlers processes privileged administrative commands.
type AdminHandlers struct {
	*BaseHandler
}

// NewAdminHandlers creates a new AdminHandlers instance.
func NewAdminHandlers(base *BaseHandler) *AdminHandlers {
	return &AdminHandlers{BaseHandler: base}
}

// Admin displays the list of available admin operations.
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
		"/parse [month] [year] - Run parser"

	_ = h.SendMessage(ctx, message.Chat.ID, text)
}

// AddArtist parses and registers new artists with their specified gender.
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

// RemoveArtist deactivates the specified artists.
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

// Config gets or sets application configuration values in the database.
func (h *AdminHandlers) Config(ctx context.Context, message *telego.Message) {
	if !h.IsAdmin(message.From) {
		return
	}

	parts := strings.Fields(message.Text)
	if len(parts) == 1 {
		res, err := h.Services.Config.GetAll(ctx)
		if err != nil {
			h.HandleError(ctx, message.Chat.ID, err, "Failed to get config")
			return
		}
		_ = h.SendMessage(ctx, message.Chat.ID, res)
		return
	}

	if len(parts) >= 3 {
		key := strings.ToUpper(parts[1])
		val := strings.Join(parts[2:], " ")
		if err := h.Services.Config.Update(ctx, key, val); err != nil {
			h.HandleError(ctx, message.Chat.ID, err, "Failed to update config")
			return
		}
		_ = h.SendMessage(ctx, message.Chat.ID, fmt.Sprintf("✅ Config updated: %s = %s", key, val))
		return
	}

	_ = h.SendMessage(ctx, message.Chat.ID, "Usage: /config OR /config <KEY> <VALUE>")
}

// Parse triggers an asynchronous release crawl for the given month and year.
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
		bgCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
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

// Export returns a comma-separated list of all tracked artist names.
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
