// Package middleware manages request processing flows such as rate limiting and debouncing.
package middleware

import (
	"fmt"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"go.uber.org/zap"
)

// commandDebounceTimeouts maps command names to their specific debounce durations.
var commandDebounceTimeouts = map[string]time.Duration{
	"month": 5 * time.Second,
}

type DebouncerInterface interface {
	CanProcessRequest(key string) bool
	CanProcessRequestWithTimeout(key string, timeout time.Duration) bool
	Cleanup()
}

// Debouncer tracks request timing to prevent duplicate processing.
type Debouncer struct {
	requests map[string]time.Time
	mu       sync.RWMutex
	timeout  time.Duration
	logger   *zap.Logger
}

var _ DebouncerInterface = (*Debouncer)(nil)

func NewDebouncer(timeout time.Duration, logger *zap.Logger) *Debouncer {
	return &Debouncer{
		requests: make(map[string]time.Time),
		timeout:  timeout,
		logger:   logger,
	}
}

func (d *Debouncer) CanProcessRequest(key string) bool {
	return d.CanProcessRequestWithTimeout(key, d.timeout)
}

func (d *Debouncer) CanProcessRequestWithTimeout(key string, timeout time.Duration) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	lastRequest, exists := d.requests[key]

	if !exists || now.Sub(lastRequest) > timeout {
		d.requests[key] = now
		return true
	}

	return false
}

func (d *Debouncer) Cleanup() {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	for key, lastRequest := range d.requests {
		if now.Sub(lastRequest) > d.timeout {
			delete(d.requests, key)
		}
	}
}

// DebounceMiddleware prevents rapid execution of the same command from the same chat.
func DebounceMiddleware(debouncer DebouncerInterface, logger *zap.Logger) func(update tgbotapi.Update, next func(tgbotapi.Update)) {
	return func(update tgbotapi.Update, next func(tgbotapi.Update)) {
		if update.Message == nil {
			next(update)
			return
		}

		command := update.Message.Command()
		key := fmt.Sprintf("%d:%s", update.Message.Chat.ID, command)

		timeout, hasCustomTimeout := commandDebounceTimeouts[command]
		var canProcess bool

		if hasCustomTimeout {
			canProcess = debouncer.CanProcessRequestWithTimeout(key, timeout)
		} else {
			canProcess = debouncer.CanProcessRequest(key)
		}

		if !canProcess {
			user := getUserIdentifier(update.Message.From)
			logger.Info("Command debounced",
				zap.String("command", command),
				zap.Int64("chat_id", update.Message.Chat.ID),
				zap.String("user", user),
				zap.Int("update_id", update.UpdateID),
				zap.Duration("timeout", timeout))

			return
		}

		next(update)
	}
}

// DebounceMiddlewareWithError is an error-aware version of DebounceMiddleware.
func DebounceMiddlewareWithError(debouncer DebouncerInterface, logger *zap.Logger) func(update tgbotapi.Update, next func(tgbotapi.Update) error) error {
	return func(update tgbotapi.Update, next func(tgbotapi.Update) error) error {
		if update.Message == nil {
			return next(update)
		}

		command := update.Message.Command()
		key := fmt.Sprintf("%d:%s", update.Message.Chat.ID, command)

		timeout, hasCustomTimeout := commandDebounceTimeouts[command]
		var canProcess bool

		if hasCustomTimeout {
			canProcess = debouncer.CanProcessRequestWithTimeout(key, timeout)
		} else {
			canProcess = debouncer.CanProcessRequest(key)
		}

		if !canProcess {
			user := getUserIdentifier(update.Message.From)
			logger.Info("Command debounced",
				zap.String("command", command),
				zap.Int64("chat_id", update.Message.Chat.ID),
				zap.String("user", user),
				zap.Int("update_id", update.UpdateID),
				zap.Duration("timeout", timeout))

			return nil
		}

		return next(update)
	}
}

// DebounceCallbackMiddleware prevents rapid interaction with inline buttons.
func DebounceCallbackMiddleware(debouncer DebouncerInterface, logger *zap.Logger) func(update tgbotapi.Update, next func(tgbotapi.Update)) {
	return func(update tgbotapi.Update, next func(tgbotapi.Update)) {
		if update.CallbackQuery == nil {
			next(update)
			return
		}

		callbackData := update.CallbackQuery.Data
		if callbackData == "" {
			callbackData = "callback"
		}

		var timeout time.Duration
		var shouldDebounce bool

		if strings.HasPrefix(callbackData, "month_") {
			shouldDebounce = true
			timeout = commandDebounceTimeouts["month"]
		}

		key := fmt.Sprintf("%d:%s", update.CallbackQuery.Message.Chat.ID, callbackData)
		var canProcess bool

		if shouldDebounce {
			canProcess = debouncer.CanProcessRequestWithTimeout(key, timeout)
		} else {
			canProcess = true
		}

		if !canProcess {
			user := getUserIdentifier(update.CallbackQuery.From)
			logger.Info("Callback debounced",
				zap.String("callback_data", callbackData),
				zap.Int64("chat_id", update.CallbackQuery.Message.Chat.ID),
				zap.String("user", user),
				zap.Int("update_id", update.UpdateID),
				zap.Duration("timeout", timeout))

			return
		}

		next(update)
	}
}
