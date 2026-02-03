package middleware

import (
	"fmt"
	"gemfactory/internal/telegram"
	"sync"
	"time"

	"github.com/mymmrac/telego"
	"go.uber.org/zap"
)

// DebouncerInterface defines the methods for request debouncing.
type DebouncerInterface interface {
	ShouldProcess(userID int64, action string) bool
	Cleanup()
}

// Debouncer prevents rapid repetition of the same action.
type Debouncer struct {
	lastProcess map[string]time.Time
	mu          sync.RWMutex
	interval    time.Duration
	logger      *zap.Logger
}

var _ DebouncerInterface = (*Debouncer)(nil)

func NewDebouncer(interval time.Duration, logger *zap.Logger) *Debouncer {
	return &Debouncer{
		lastProcess: make(map[string]time.Time),
		interval:    interval,
		logger:      logger,
	}
}

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

// Cleanup removes expired records.
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

// DebounceMiddleware prevents duplicate processing of commands.
func DebounceMiddleware(debouncer DebouncerInterface, logger *zap.Logger) func(update telego.Update, next func(telego.Update)) {
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

// DebounceMiddlewareWithError is an error-aware version of DebounceMiddleware.
func DebounceMiddlewareWithError(debouncer DebouncerInterface, logger *zap.Logger) func(update telego.Update, next func(telego.Update) error) error {
	return func(update telego.Update, next func(telego.Update) error) error {
		if update.Message == nil {
			return next(update)
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
			return nil
		}

		return next(update)
	}
}

// DebounceCallbackMiddleware prevents duplicate processing of callback interactions.
func DebounceCallbackMiddleware(debouncer DebouncerInterface, logger *zap.Logger) func(update telego.Update, next func(telego.Update)) {
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
