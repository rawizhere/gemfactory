package main

import (
	"context"
	"errors"
	"gemfactory/internal/app"
	"gemfactory/internal/config"
	"gemfactory/internal/logger"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	log := logger.New()

	if err := run(log); err != nil {
		log.Error("Bot stopped with error", slog.Any("error", err))
		os.Exit(1)
	}

	log.Info("Bot stopped successfully")
}

func run(log *slog.Logger) error {
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
		log.Error("Failed to stop bot gracefully", slog.Any("error", err))
	}

	return nil
}
