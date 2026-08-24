package middleware

import (
	"fmt"
	"time"

	"gemfactory/internal/telegram"

	lru "github.com/hashicorp/golang-lru/v2/expirable"
	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	"go.uber.org/zap"
)

type Debouncer struct {
	lastProcess *lru.LRU[string, time.Time]
	interval    time.Duration
	logger      *zap.Logger
}

func NewDebouncer(interval time.Duration, logger *zap.Logger) *Debouncer {
	return &Debouncer{
		lastProcess: lru.NewLRU[string, time.Time](maxTrackedUsers, nil, interval*2),
		interval:    interval,
		logger:      logger,
	}
}

func (d *Debouncer) ShouldProcess(userID int64, action string) bool {
	key := fmt.Sprintf("%d:%s", userID, action)

	if last, exists := d.lastProcess.Get(key); exists && time.Since(last) < d.interval {
		return false
	}

	d.lastProcess.Add(key, time.Now())
	return true
}

// Debounce drops duplicate commands and repeated callback queries within the debounce interval.
func Debounce(debouncer *Debouncer, logger *zap.Logger) th.Handler {
	return func(ctx *th.Context, update telego.Update) error {
		switch {
		case update.Message != nil:
			userID := update.Message.From.ID
			command := telegram.MessageCommand(update.Message)
			if command != "" && !debouncer.ShouldProcess(userID, "msg:"+command) {
				logDebounced(logger, userID, "msg:"+command, command)
				return nil
			}
		case update.CallbackQuery != nil:
			if !debouncer.ShouldProcess(update.CallbackQuery.From.ID, "cb:"+update.CallbackQuery.Data) {
				logDebounced(logger, update.CallbackQuery.From.ID, "cb:"+update.CallbackQuery.Data, update.CallbackQuery.Data)
				return nil
			}
		}
		return ctx.Next(update)
	}
}

func logDebounced(logger *zap.Logger, userID int64, key, action string) {
	logger.Debug("Update debounced",
		zap.Int64("user_id", userID),
		zap.String("key", key),
		zap.String("action", action))
}
