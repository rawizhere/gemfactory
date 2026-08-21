package middleware

import (
	"fmt"
	"gemfactory/internal/telegram"
	"sync"
	"time"

	"github.com/mymmrac/telego"
	"go.uber.org/zap"
)

// Debouncer tracks per-user action timestamps to suppress duplicate rapid-fire inputs.
type Debouncer struct {
	lastProcess map[string]time.Time
	mu          sync.RWMutex
	interval    time.Duration
	logger      *zap.Logger
}

// NewDebouncer creates a new Debouncer with the given cooldown interval.
func NewDebouncer(interval time.Duration, logger *zap.Logger) *Debouncer {
	return &Debouncer{
		lastProcess: make(map[string]time.Time),
		interval:    interval,
		logger:      logger,
	}
}

// ShouldProcess checks if an action for a user should proceed based on cooldown.
func (d *Debouncer) ShouldProcess(userID int64, action string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	key := fmt.Sprintf("%d:%s", userID, action)
	last, exists := d.lastProcess[key]

	if exists && time.Since(last) < d.interval {
		return false
	}

	d.lastProcess[key] = time.Now()
	return true
}

// Cleanup purges stale cooldown entries.
func (d *Debouncer) Cleanup() {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	for key, last := range d.lastProcess {
		if now.Sub(last) > d.interval*2 {
			delete(d.lastProcess, key)
		}
	}
}

// Debounce drops repeated rapid command messages from the same user.
func Debounce(debouncer *Debouncer, logger *zap.Logger) func(update telego.Update, next func(telego.Update)) {
	return func(update telego.Update, next func(telego.Update)) {
		if update.Message == nil {
			next(update)
			return
		}

		userID := update.Message.From.ID
		command := ""
		for _, entity := range update.Message.Entities {
			if entity.Type == telego.EntityTypeBotCommand && entity.Offset == 0 {
				command = update.Message.Text[1:entity.Length]
				break
			}
		}

		if command != "" && !debouncer.ShouldProcess(userID, "msg:"+command) {
			user := telegram.GetUserIdentifier(update.Message.From)
			logger.Debug("Message debounced",
				zap.Int64("user_id", userID),
				zap.String("user", user),
				zap.String("command", command))
			return
		}

		next(update)
	}
}

// DebounceCallback drops duplicate button clicks in rapid succession.
func DebounceCallback(debouncer *Debouncer, logger *zap.Logger) func(update telego.Update, next func(telego.Update)) {
	return func(update telego.Update, next func(telego.Update)) {
		if update.CallbackQuery == nil {
			next(update)
			return
		}

		userID := update.CallbackQuery.From.ID
		data := update.CallbackQuery.Data

		if !debouncer.ShouldProcess(userID, "cb:"+data) {
			user := telegram.GetUserIdentifier(&update.CallbackQuery.From)
			logger.Debug("Callback debounced",
				zap.Int64("user_id", userID),
				zap.String("user", user),
				zap.String("data", data))
			return
		}

		next(update)
	}
}
