// Package notify delivers admin notifications over Telegram.
package notify

import (
	"context"
	"strconv"

	"gemfactory/internal/model"
	"gemfactory/internal/telegram"

	"go.uber.org/zap"
)

// AdminChatIDKey stores the admin chat id learned from admin messages.
const AdminChatIDKey = "ADMIN_CHAT_ID"

// AdminNotifier sends messages to the stored admin chat.
type AdminNotifier struct {
	configRepo model.ConfigRepository
	tg         *telegram.Client
	logger     *zap.Logger
}

func NewAdminNotifier(configRepo model.ConfigRepository, tg *telegram.Client, logger *zap.Logger) *AdminNotifier {
	return &AdminNotifier{
		configRepo: configRepo,
		tg:         tg,
		logger:     logger,
	}
}

// Send delivers text to the admin chat; it skips until the admin writes to the bot once.
func (n *AdminNotifier) Send(ctx context.Context, text string) {
	if n == nil || n.tg == nil {
		return
	}
	c, err := n.configRepo.Get(ctx, AdminChatIDKey)
	if err != nil || c == nil || c.Value == "" {
		n.logger.Debug("Admin chat id unknown, notification skipped")
		return
	}
	chatID, err := strconv.ParseInt(c.Value, 10, 64)
	if err != nil {
		n.logger.Warn("Invalid stored admin chat id", zap.String("value", c.Value))
		return
	}
	if err := n.tg.SendMessage(ctx, chatID, text); err != nil {
		n.logger.Warn("Failed to send admin notification", zap.Error(err))
	}
}
