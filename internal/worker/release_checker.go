package worker

import (
	"context"
	"strconv"
	"time"

	"go.uber.org/zap"

	"gemfactory/internal/notify"
	"gemfactory/internal/service"
)

type ReleaseChecker struct {
	releaseService *service.ReleaseService
	logger         *zap.Logger
	interval       time.Duration
	notifier       *notify.AdminNotifier
	alerted        bool
}

func NewReleaseChecker(releaseService *service.ReleaseService, logger *zap.Logger, initialInterval time.Duration, notifier *notify.AdminNotifier) *ReleaseChecker {
	if initialInterval <= 0 {
		initialInterval = 24 * time.Hour
	}
	return &ReleaseChecker{
		releaseService: releaseService,
		logger:         logger,
		interval:       initialInterval,
		notifier:       notifier,
	}
}

func (w *ReleaseChecker) Start(ctx context.Context) {
	w.logger.Info("Release checker started", zap.Duration("interval", w.interval))

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	startupTimer := time.NewTimer(30 * time.Second)
	defer startupTimer.Stop()

	select {
	case <-startupTimer.C:
		w.checkReleases(ctx)
	case <-ctx.Done():
		return
	}

	for {
		select {
		case <-ticker.C:
			w.checkReleases(ctx)
		case <-ctx.Done():
			w.logger.Info("Release checker stopped")
			return
		}
	}
}

func (w *ReleaseChecker) checkReleases(ctx context.Context) {
	now := time.Now()
	currentYear := now.Format("2006")
	w.logger.Info("Checking releases for current year via REST...", zap.String("year", currentYear))

	var parseErr error
	if _, err := w.releaseService.ParseReleasesForYear(ctx, currentYear); err != nil {
		w.logger.Error("Failed to check releases for current year", zap.String("year", currentYear), zap.Error(err))
		parseErr = err
	} else {
		w.logger.Info("Current year release check completed", zap.String("year", currentYear))
	}

	// In January, check previous year to catch late backfills
	if now.Month() == time.January {
		prevYear := strconv.Itoa(now.Year() - 1)
		w.logger.Info("Checking releases for previous year backfills...", zap.String("year", prevYear))
		if _, err := w.releaseService.ParseReleasesForYear(ctx, prevYear); err != nil && parseErr == nil {
			w.logger.Warn("Failed to check releases for previous year", zap.String("year", prevYear), zap.Error(err))
			parseErr = err
		}
	}

	// In Q4 (Oct, Nov, Dec), check next year for early comebacks
	if now.Month() >= time.October {
		nextYear := strconv.Itoa(now.Year() + 1)
		w.logger.Info("Checking releases for next year...", zap.String("year", nextYear))
		if _, err := w.releaseService.ParseReleasesForYear(ctx, nextYear); err != nil && parseErr == nil {
			w.logger.Warn("Failed to check releases for next year", zap.String("year", nextYear), zap.Error(err))
			parseErr = err
		}
	}

	w.reportHealth(ctx, parseErr)
}

// reportHealth notifies the admin once when parsing breaks and once when it recovers.
func (w *ReleaseChecker) reportHealth(ctx context.Context, parseErr error) {
	if parseErr != nil {
		if w.alerted {
			w.logger.Warn("Release parsing still failing", zap.Error(parseErr))
			return
		}
		w.alerted = true
		w.notifier.Send(ctx, "Release parsing is failing.\nError: "+parseErr.Error()+"\nCheck the scraper user agents in the web admin settings.")
		return
	}
	if w.alerted {
		w.alerted = false
		w.notifier.Send(ctx, "Release parsing has recovered.")
	}
}
