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

// Parse triggers an asynchronous release crawl for the given month or year.
// Accepted forms: /parse, /parse <month>, /parse <month> <year>,
// /parse <year> (whole year), /parse <month>-<year>.
func (h *AdminHandlers) Parse(ctx context.Context, message *telego.Message) {
	if !h.IsAdmin(message.From) {
		return
	}

	args := strings.Fields(message.Text)[1:]

	if len(args) == 1 {
		if y, err := strconv.Atoi(args[0]); err == nil && len(args[0]) == 4 && y >= 2000 {
			yearStr := strconv.Itoa(y)
			_ = h.SendMessage(ctx, message.Chat.ID, fmt.Sprintf("🔄 Running parser for entire year %s...", yearStr))
			go func() {
				bgCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
				defer cancel()

				count, err := h.Services.Release.ParseReleasesForYear(bgCtx, yearStr)
				if err != nil {
					h.HandleError(bgCtx, message.Chat.ID, fmt.Errorf("year %s: %w", yearStr, err), "Parser error")
					return
				}
				_ = h.SendMessage(bgCtx, message.Chat.ID, fmt.Sprintf("✅ Parsing complete for %s. Found %d releases", yearStr, count))
			}()
			return
		}
	}

	var monthQueries []string
	switch {
	case len(args) >= 2:
		if _, err := strconv.Atoi(args[1]); err != nil {
			_ = h.SendMessage(ctx, message.Chat.ID, "Usage: /parse [<month>|<year>] [<year>]")
			return
		}
		monthQueries = []string{strings.ToLower(args[0]) + "-" + args[1]}
	case len(args) == 1:
		monthQueries = []string{strings.ToLower(args[0])}
	default:
		monthQueries = []string{strings.ToLower(time.Now().Format("January")) + "-" + strconv.Itoa(time.Now().Year())}
	}

	_ = h.SendMessage(ctx, message.Chat.ID, fmt.Sprintf("🔄 Running parser for %s...", strings.Join(monthQueries, ", ")))

	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), time.Duration(len(monthQueries))*5*time.Minute)
		defer cancel()

		total := 0
		for _, q := range monthQueries {
			count, err := h.Services.Release.ParseReleasesForMonth(bgCtx, q)
			if err != nil {
				h.HandleError(bgCtx, message.Chat.ID, fmt.Errorf("%s: %w", q, err), "Parser error")
				continue
			}
			total += count
		}
		_ = h.SendMessage(bgCtx, message.Chat.ID, fmt.Sprintf("✅ Parsing complete. Found %d releases", total))
	}()
}

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

var monthNames = []string{
	"january", "february", "march", "april", "may", "june",
	"july", "august", "september", "october", "november", "december",
}
