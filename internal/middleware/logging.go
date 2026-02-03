package middleware

import (
	"fmt"
	"gemfactory/internal/telegram"
	"time"

	"github.com/mymmrac/telego"
	"go.uber.org/zap"
)

// RequestContext holds metadata for tracking requests.
type RequestContext struct {
	StartTime time.Time
	RequestID string
	UserID    int64
	ChatID    int64
	Command   string
}

// LoggingMiddleware logs commands and their duration.
func LoggingMiddleware(logger *zap.Logger) func(update telego.Update, next func(telego.Update)) {
	return func(update telego.Update, next func(telego.Update)) {
		if update.Message == nil {
			next(update)
			return
		}

		command := ""
		for _, entity := range update.Message.Entities {
			if entity.Type == telego.EntityTypeBotCommand && entity.Offset == 0 {
				command = update.Message.Text[1:entity.Length]
				break
			}
		}

		requestCtx := &RequestContext{
			StartTime: time.Now(),
			RequestID: fmt.Sprintf("%d-%d", update.UpdateID, time.Now().UnixNano()),
			UserID:    update.Message.From.ID,
			ChatID:    update.Message.Chat.ID,
			Command:   command,
		}

		user := telegram.GetUserIdentifier(update.Message.From)

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
func LogRequestWithError(logger *zap.Logger) func(update telego.Update, next func(telego.Update) error) error {
	return func(update telego.Update, next func(telego.Update) error) error {
		if update.Message == nil {
			return next(update)
		}

		command := ""
		for _, entity := range update.Message.Entities {
			if entity.Type == telego.EntityTypeBotCommand && entity.Offset == 0 {
				command = update.Message.Text[1:entity.Length]
				break
			}
		}

		requestCtx := &RequestContext{
			StartTime: time.Now(),
			RequestID: fmt.Sprintf("%d-%d", update.UpdateID, time.Now().UnixNano()),
			UserID:    update.Message.From.ID,
			ChatID:    update.Message.Chat.ID,
			Command:   command,
		}

		user := telegram.GetUserIdentifier(update.Message.From)

		logger.Info("Processing command",
			zap.String("request_id", requestCtx.RequestID),
			zap.String("command", requestCtx.Command),
			zap.Int64("user_id", requestCtx.UserID),
			zap.Int64("chat_id", requestCtx.ChatID),
			zap.String("user", user),
			zap.Int("update_id", update.UpdateID))

		// Execute next handler
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
