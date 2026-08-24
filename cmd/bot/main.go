package main

import (
	"context"
	"errors"
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

	if err := run(log); err != nil {
		log.Error("Bot stopped with error", zap.Error(err))
		os.Exit(1)
	}

	log.Info("Bot stopped successfully")
}

func run(log *zap.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	bot, err := app.NewBot(ctx, cfg, log)
	if err != nil {
		return err
	}

	if err := bot.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
		_ = bot.Stop()
		return err
	}

	if err := bot.Stop(); err != nil {
		log.Error("Failed to stop bot gracefully", zap.Error(err))
	}

	return nil
}
