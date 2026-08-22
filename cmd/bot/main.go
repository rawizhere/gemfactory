package main

import (
	"context"
	"gemfactory/internal/app"
	"gemfactory/internal/config"
	"gemfactory/internal/logger"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"
)

func main() {
	l, err := logger.New()
	if err != nil {
		panic(err)
	}
	log := l.Logger

	cfg, err := config.Load()
	if err != nil {
		log.Fatal("Failed to load configuration", zap.Error(err))
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	bot, err := app.NewBot(ctx, cfg, log)
	if err != nil {
		log.Fatal("Failed to create bot", zap.Error(err))
	}

	if err := bot.Start(ctx); err != nil && err != context.Canceled {
		log.Error("Bot stopped with error", zap.Error(err))
		_ = bot.Stop()
		os.Exit(1)
	}

	if err := bot.Stop(); err != nil {
		log.Error("Failed to stop bot gracefully", zap.Error(err))
	}

	log.Info("Bot stopped successfully")
}
