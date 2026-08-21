package worker

import (
	"context"
	"fmt"
	"gemfactory/internal/service"
	"strings"
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
	w.logger.Info("Checking for new releases (previous + current + 3 months ahead)...")

	now := time.Now()
	for i := -1; i <= 3; i++ {
		targetDate := now.AddDate(0, i, 0)
		monthName := strings.ToLower(targetDate.Format("January"))
		yearStr := targetDate.Format("2006")

		w.logger.Info("Parsing releases", zap.String("month", monthName), zap.String("year", yearStr))

		count, err := w.releaseService.ParseReleasesForMonth(ctx, fmt.Sprintf("%s-%s", monthName, yearStr))
		if err != nil {
			if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") {
				w.logger.Warn("Monthly page not found, stopping forward scan for this cycle",
					zap.String("month", monthName),
					zap.String("year", yearStr))
				break
			}

			w.logger.Error("Failed to check releases", zap.String("month", monthName), zap.Error(err))
			continue
		}

		w.logger.Info("Month check completed",
			zap.String("month", monthName),
			zap.String("year", yearStr),
			zap.Int("new_releases", count))
	}
}
