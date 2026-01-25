package worker

import (
	"context"
	"fmt"
	"gemfactory/internal/model"
	"gemfactory/internal/service"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// ReleaseCheckerWorker orchestrates periodic scans for new vulnerability music releases.
type ReleaseCheckerWorker struct {
	releaseService service.ReleaseServiceInterface
	logger         *zap.Logger
	interval       time.Duration
	intervalUpdate chan time.Duration
}

// NewReleaseCheckerWorker initializes a new ReleaseCheckerWorker with a default interval.
func NewReleaseCheckerWorker(releaseService service.ReleaseServiceInterface, logger *zap.Logger, initialInterval time.Duration) *ReleaseCheckerWorker {
	if initialInterval <= 0 {
		initialInterval = 24 * time.Hour
	}
	return &ReleaseCheckerWorker{
		releaseService: releaseService,
		logger:         logger,
		interval:       initialInterval,
		intervalUpdate: make(chan time.Duration, 1),
	}
}

// Start initiates the background loop for periodic release checks and interval updates.
func (w *ReleaseCheckerWorker) Start(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	w.logger.Info("Release checker worker started", zap.Duration("interval", w.interval))

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	// Initial check on startup
	w.checkReleases(ctx)

	for {
		select {
		case <-ticker.C:
			w.checkReleases(ctx)
		case newInterval := <-w.intervalUpdate:
			w.logger.Info("Updating release checker interval", zap.Duration("new", newInterval))
			ticker.Stop()
			ticker = time.NewTicker(newInterval)
			w.interval = newInterval
		case <-ctx.Done():
			w.logger.Info("Release checker worker stopped")
			return
		}
	}
}

// ApplyConfig dynamically updates the worker's polling interval based on configuration changes.
func (w *ReleaseCheckerWorker) ApplyConfig(ctx context.Context, configs []model.Config) error {
	for _, c := range configs {
		if c.Key == "RELEASE_CHECK_INTERVAL" {
			d, err := time.ParseDuration(c.Value)
			if err != nil {
				return fmt.Errorf("invalid RELEASE_CHECK_INTERVAL from DB: %w", err)
			}
			w.intervalUpdate <- d
		}
	}
	return nil
}

func (w *ReleaseCheckerWorker) checkReleases(ctx context.Context) {
	w.logger.Info("Checking for new releases (current + 3 months ahead)...")

	now := time.Now()
	// Scan current and next 3 months
	for i := 0; i <= 3; i++ {
		targetDate := now.AddDate(0, i, 0)
		monthName := strings.ToLower(targetDate.Format("January"))
		yearStr := targetDate.Format("2006")

		w.logger.Info("Parsing releases", zap.String("month", monthName), zap.String("year", yearStr))

		count, err := w.releaseService.ParseReleasesForMonth(ctx, monthName)
		if err != nil {
			// Stop forward scan if monthly page is missing (404)
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
