// Package main serves as the entry point for the GemFactory bot application.
package main

import (
	"context"
	"gemfactory/internal/app"
	"gemfactory/internal/config"
	"gemfactory/pkg/logger"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"
)

func main() {
	// Initialize logger.
	l, err := logger.NewWithLevel()
	if err != nil {
		panic(err)
	}
	log := l.Logger

	// Load configuration.
	cfg, err := config.Load()
	if err != nil {
		log.Fatal("Failed to load configuration", zap.Error(err))
	}

	// Create context and setup signal handling.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Info("Shutdown signal received")
		cancel()
	}()

	// Initialize bot via factory.
	bot, err := app.NewBotWithFactory(ctx, cfg, l)
	if err != nil {
		log.Fatal("Failed to create bot", zap.Error(err))
	}

	// Start bot execution.
	if err := bot.Start(ctx); err != nil {
		log.Error("Bot stopped with error", zap.Error(err))
		if err := bot.Stop(); err != nil {
			log.Error("Failed to stop bot after start error", zap.Error(err))
		}
		os.Exit(1)
	}

	// Graceful shutdown.
	if err := bot.Stop(); err != nil {
		log.Error("Failed to stop bot gracefully", zap.Error(err))
	}

	log.Info("Bot stopped successfully")
}
