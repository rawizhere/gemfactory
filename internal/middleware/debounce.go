package middleware

import (
	"fmt"
	"maps"
	"sync"
	"time"

	"gemfactory/internal/telegram"

	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	"go.uber.org/zap"
)

type Debouncer struct {
	mu          sync.Mutex
	lastProcess map[string]time.Time
	interval    time.Duration
	logger      *zap.Logger
}

func NewDebouncer(interval time.Duration, logger *zap.Logger) *Debouncer {
	return &Debouncer{
		lastProcess: map[string]time.Time{},
		interval:    interval,
		logger:      logger,
	}
}

func (d *Debouncer) ShouldProcess(userID int64, action string) bool {
	key := fmt.Sprintf("%d:%s", userID, action)
	now := time.Now()

	d.mu.Lock()
	defer d.mu.Unlock()
	// Entries past the interval can never debounce again, so they are dropped on every call.
	maps.DeleteFunc(d.lastProcess, func(_ string, t time.Time) bool { return now.Sub(t) >= d.interval })
	if _, exists := d.lastProcess[key]; exists {
		return false
	}
	d.lastProcess[key] = now
	return true
}

// Debounce drops duplicate commands and repeated callback queries within the debounce interval.
func Debounce(debouncer *Debouncer, logger *zap.Logger) th.Handler {
	return func(ctx *th.Context, update telego.Update) error {
		switch {
		case update.Message != nil && update.Message.From != nil:
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
