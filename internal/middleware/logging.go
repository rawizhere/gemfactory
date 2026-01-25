// Package middleware provides Telegram update processing middleware.
package middleware

import (
	"fmt"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"go.uber.org/zap"
)

// RequestContext holds metadata for tracking request execution.
type RequestContext struct {
	StartTime time.Time
	RequestID string
	UserID    int64
	ChatID    int64
	Command   string
}

// LoggingMiddleware logs incoming commands and their execution duration.
func LoggingMiddleware(logger *zap.Logger) func(update tgbotapi.Update, next func(tgbotapi.Update)) {
	return func(update tgbotapi.Update, next func(tgbotapi.Update)) {
		if update.Message == nil {
			next(update)
			return
		}

		requestCtx := &RequestContext{
			StartTime: time.Now(),
			RequestID: fmt.Sprintf("%d-%d", update.UpdateID, time.Now().UnixNano()),
			UserID:    update.Message.From.ID,
			ChatID:    update.Message.Chat.ID,
			Command:   update.Message.Command(),
		}

		user := getUserIdentifier(update.Message.From)

		logger.Info("Processing command",
			zap.String("request_id", requestCtx.RequestID),
			zap.String("command", requestCtx.Command),
			zap.Int64("user_id", requestCtx.UserID),
			zap.Int64("chat_id", requestCtx.ChatID),
			zap.String("user", user),
			zap.Int("update_id", update.UpdateID))

		next(update)

		// Log completion
		duration := time.Since(requestCtx.StartTime)
		logger.Info("Command completed successfully",
			zap.String("request_id", requestCtx.RequestID),
			zap.String("command", requestCtx.Command),
			zap.Duration("duration", duration))
	}
}

// LogRequestWithError is an error-aware logging middleware.
func LogRequestWithError(logger *zap.Logger) func(update tgbotapi.Update, next func(tgbotapi.Update) error) error {
	return func(update tgbotapi.Update, next func(tgbotapi.Update) error) error {
		if update.Message == nil {
			return next(update)
		}

		requestCtx := &RequestContext{
			StartTime: time.Now(),
			RequestID: fmt.Sprintf("%d-%d", update.UpdateID, time.Now().UnixNano()),
			UserID:    update.Message.From.ID,
			ChatID:    update.Message.Chat.ID,
			Command:   update.Message.Command(),
		}

		user := getUserIdentifier(update.Message.From)

		logger.Info("Processing command",
			zap.String("request_id", requestCtx.RequestID),
			zap.String("command", requestCtx.Command),
			zap.Int64("user_id", requestCtx.UserID),
			zap.Int64("chat_id", requestCtx.ChatID),
			zap.String("user", user),
			zap.Int("update_id", update.UpdateID))

		// Call the next handler.
		err := next(update)

		// Log command completion.
		duration := time.Since(requestCtx.StartTime)
		if err != nil {
			logger.Error("Command completed with error",
				zap.String("request_id", requestCtx.RequestID),
				zap.String("command", requestCtx.Command),
				zap.Duration("duration", duration),
				zap.Error(err))
		} else {
			logger.Info("Command completed successfully",
				zap.String("request_id", requestCtx.RequestID),
				zap.String("command", requestCtx.Command),
				zap.Duration("duration", duration))
		}

		return err
	}
}

// getUserIdentifier returns a human-readable user string (username or ID).
func getUserIdentifier(user *tgbotapi.User) string {
	if user == nil {
		return "unknown"
	}

	if user.UserName != "" {
		return "@" + user.UserName
	}

	if user.FirstName != "" {
		if user.LastName != "" {
			return user.FirstName + " " + user.LastName
		}
		return user.FirstName
	}

	return fmt.Sprintf("user_%d", user.ID)
}
