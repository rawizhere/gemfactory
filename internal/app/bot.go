// Package app coordinates the lifecycle and components of the Telegram bot.
package app

import (
	"context"
	"fmt"
	"gemfactory/internal/config"
	"gemfactory/internal/downloader"
	"gemfactory/internal/health"
	"gemfactory/internal/keyboard"
	"gemfactory/internal/service"
	"gemfactory/internal/settings"
	"gemfactory/internal/storage"
	"gemfactory/internal/telegram"
	"gemfactory/internal/web"
	"gemfactory/internal/worker"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// Bot coordinates the lifecycle of all services, workers, and the Telegram client.
type Bot struct {
	config         *config.Config
	logger         *zap.Logger
	db             *storage.Postgres
	telegram       *telegram.Client
	health         *health.Server
	web            *web.Server
	services       *service.Services
	keyboard       *keyboard.Manager
	router         *Router
	releaseChecker *worker.ReleaseChecker
	downloader     *downloader.Service
	wg             sync.WaitGroup
	ctx            context.Context
	cancel         context.CancelFunc
}

func NewBot(ctx context.Context, cfg *config.Config, logger *zap.Logger) (*Bot, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}

	if err := os.MkdirAll(cfg.AppDataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create app data directory: %w", err)
	}

	db, err := storage.NewPostgres(ctx, cfg.DatabaseURL, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	services := service.NewServices(db, cfg, logger)
	tgClient, err := telegram.NewClient(cfg.BotToken, logger)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to create telegram client: %w", err)
	}

	keyboardManager := keyboard.NewManager(services.Release, cfg, logger)
	keyboardManager.SetTelegramClient(tgClient)

	botCtx, botCancel := context.WithCancel(ctx)

	// yt-dlp downloader: resolve binary, start nightly update loop.
	cookieRepo := storage.NewCookieRepository(db.GetDB(), logger)
	configRepo := storage.NewConfigRepository(db.GetDB(), logger)
	downloaderSvc := downloader.NewService(cookieRepo, cfg.AppDataDir, cfg.DownloadConcurrency, logger)
	downloaderSvc.SetConfigRepo(configRepo)
	if c, err := configRepo.Get(botCtx, "DOWNLOAD_CONCURRENCY"); err == nil && c != nil {
		if n, perr := strconv.Atoi(c.Value); perr == nil && n > 0 {
			downloaderSvc.SetConcurrency(n)
		}
	}
	downloader.SetTranslationTimeoutSeconds(int64(settings.New(configRepo).Int(botCtx, "TRANSLATION_TIMEOUT", 180)))
	if err := downloader.EnsureYTDLP(botCtx); err != nil {
		logger.Warn("yt-dlp unavailable at startup; downloads disabled until next restart", zap.Error(err))
	} else {
		downloader.StartYTDLPUpdateLoop(botCtx)
	}
	downloaderSvc.StartCleanupLoop(botCtx)
	if exec.Command("deno", "--version").Run() != nil && exec.Command("node", "--version").Run() != nil {
		logger.Warn("neither deno nor node found in PATH; YouTube downloads may fail (JS runtime required by yt-dlp)")
	}
	warnIfFFmpegMissingLibass(logger)

	router := NewRouter(services, cfg, keyboardManager, logger, tgClient, downloaderSvc)

	var healthServer *health.Server
	if cfg.HealthCheckEnabled {
		healthServer = health.NewServer(cfg.HealthPort, logger, db)
	}

	var webServer *web.Server
	if cfg.WebEnabled {
		webServer = web.NewServer(cfg.WebPort, logger, web.Deps{
			AppCfg:     cfg,
			Artists:    storage.NewArtistRepository(db.GetDB(), logger),
			Releases:   storage.NewReleaseRepository(db.GetDB(), logger),
			Configs:    configRepo,
			Cookies:    cookieRepo,
			Downloads:  downloaderSvc,
			ReleaseSvc: services.Release,
		})
	}

	releaseChecker := worker.NewReleaseChecker(services.Release, logger, cfg.ReleaseCheckInterval)

	return &Bot{
		config:         cfg,
		logger:         logger,
		db:             db,
		telegram:       tgClient,
		health:         healthServer,
		web:            webServer,
		services:       services,
		keyboard:       keyboardManager,
		router:         router,
		releaseChecker: releaseChecker,
		downloader:     downloaderSvc,
		ctx:            botCtx,
		cancel:         botCancel,
	}, nil
}

func warnIfFFmpegMissingLibass(logger *zap.Logger) {
	out, err := exec.Command("ffmpeg", "-hide_banner", "-filters").Output()
	if err != nil {
		logger.Warn("ffmpeg not available; clip re-encoding will fail", zap.Error(err))
		return
	}
	if !strings.Contains(string(out), " subtitles ") {
		logger.Warn("ffmpeg build has no libass/subtitles filter; /subs commands will fail. Install ffmpeg with libass")
	}
}

// Start launches the health server, periodic cleanup, background release checker, and long-polling.
func (b *Bot) Start(ctx context.Context) error {
	b.logger.Info("Starting bot")

	if b.health != nil {
		b.wg.Add(1)
		go func() {
			defer b.wg.Done()
			if err := b.health.Start(); err != nil && err.Error() != "http: Server closed" {
				b.logger.Error("Health check server failed", zap.Error(err))
			}
		}()
	}

	if b.web != nil {
		b.wg.Add(1)
		go func() {
			defer b.wg.Done()
			if err := b.web.Start(); err != nil && err.Error() != "http: Server closed" {
				b.logger.Error("Web server failed", zap.Error(err))
			}
		}()
	}

	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		b.releaseChecker.Start(b.ctx)
	}()

	b.logger.Info("Bot started successfully")

	return b.telegram.Start(ctx, b.router)
}

func (b *Bot) Stop() error {
	b.logger.Info("Stopping bot gracefully")

	if b.cancel != nil {
		b.cancel()
	}

	if b.keyboard != nil {
		b.keyboard.Stop()
	}

	if b.health != nil {
		_ = b.health.Stop()
	}

	if b.web != nil {
		if err := b.web.Stop(); err != nil {
			b.logger.Warn("Failed to stop web server", zap.Error(err))
		}
	}

	stopped := make(chan struct{})
	go func() {
		b.wg.Wait()
		close(stopped)
	}()

	select {
	case <-stopped:
		b.logger.Info("All background workers stopped cleanly")
	case <-time.After(15 * time.Second):
		b.logger.Warn("Shutdown timed out waiting for background workers")
	}

	if b.db != nil {
		_ = b.db.Close()
	}

	b.logger.Info("Bot stopped successfully")
	return nil
}
