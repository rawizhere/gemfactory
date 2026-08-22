package worker

import (
	"context"
	"gemfactory/internal/service"
	"strconv"
	"time"

	"go.uber.org/zap"
)

// ReleaseChecker orchestrates periodic scans for new music releases.
type ReleaseChecker struct {
	releaseService *service.ReleaseService
	logger         *zap.Logger
	interval       time.Duration
}

// NewReleaseChecker initializes a new ReleaseChecker with the given interval.
func NewReleaseChecker(releaseService *service.ReleaseService, logger *zap.Logger, initialInterval time.Duration) *ReleaseChecker {
	if initialInterval <= 0 {
		initialInterval = 24 * time.Hour
	}
	return &ReleaseChecker{
		releaseService: releaseService,
		logger:         logger,
		interval:       initialInterval,
	}
}

// Start initiates the checker's periodic scanning loop until ctx is cancelled.
func (w *ReleaseChecker) Start(ctx context.Context) {
	w.logger.Info("Release checker started", zap.Duration("interval", w.interval))

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	// Initial check on startup after a brief delay
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

	count, err := w.releaseService.ParseReleasesForYear(ctx, currentYear)
	if err != nil {
		w.logger.Error("Failed to check releases for current year", zap.String("year", currentYear), zap.Error(err))
	} else {
		w.logger.Info("Current year release check completed",
			zap.String("year", currentYear),
			zap.Int("saved_releases", count))
	}

	// In January, check previous year to catch late backfills
	if now.Month() == time.January {
		prevYear := strconv.Itoa(now.Year() - 1)
		w.logger.Info("Checking releases for previous year backfills...", zap.String("year", prevYear))
		prevCount, prevErr := w.releaseService.ParseReleasesForYear(ctx, prevYear)
		if prevErr != nil {
			w.logger.Warn("Failed to check releases for previous year", zap.String("year", prevYear), zap.Error(prevErr))
		} else {
			w.logger.Info("Previous year release check completed",
				zap.String("year", prevYear),
				zap.Int("saved_releases", prevCount))
		}
	}

	// In Q4 (Oct, Nov, Dec), check next year for early comebacks
	if now.Month() >= time.October {
		nextYear := strconv.Itoa(now.Year() + 1)
		w.logger.Info("Checking releases for next year...", zap.String("year", nextYear))
		nextCount, nextErr := w.releaseService.ParseReleasesForYear(ctx, nextYear)
		if nextErr != nil {
			w.logger.Warn("Failed to check releases for next year", zap.String("year", nextYear), zap.Error(nextErr))
		} else {
			w.logger.Info("Next year release check completed",
				zap.String("year", nextYear),
				zap.Int("saved_releases", nextCount))
		}
	}
}
