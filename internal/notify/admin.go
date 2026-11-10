// Package notify delivers admin notifications over Telegram.
package notify

import (
	"context"
	"strconv"

	"gemfactory/internal/settings"
	"gemfactory/internal/telegram"

	"log/slog"
)

// AdminNotifier sends messages to the stored admin chat.
type AdminNotifier struct {
	settings *settings.Provider
	tg       *telegram.Client
	logger   *slog.Logger
}

func NewAdminNotifier(p *settings.Provider, tg *telegram.Client, logger *slog.Logger) *AdminNotifier {
	return &AdminNotifier{
		settings: p,
		tg:       tg,
		logger:   logger,
	}
}

// Send delivers text to the admin chat; it skips until the admin writes to the bot once.
func (n *AdminNotifier) Send(ctx context.Context, text string) {
	if n == nil || n.tg == nil {
		return
	}
	value := n.settings.Value(ctx, settings.AdminChatID)
	if value == "" {
		n.logger.Debug("Admin chat id unknown, notification skipped")
		return
	}
	chatID, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		n.logger.Warn("Invalid stored admin chat id", slog.String("value", value))
		return
	}
	if err := n.tg.SendMessage(ctx, chatID, text); err != nil {
		n.logger.Warn("Failed to send admin notification", slog.Any("error", err))
	}
}
