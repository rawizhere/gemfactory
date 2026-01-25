package worker

import (
	"context"
	"gemfactory/internal/service"
	"sync"
	"time"

	"go.uber.org/zap"
)

// HomeworkResetWorker periodically clears homework assignments to allow daily requests.
type HomeworkResetWorker struct {
	homeworkService service.HomeworkServiceInterface
	logger          *zap.Logger
}

// NewHomeworkResetWorker creates a new homework reset worker.
func NewHomeworkResetWorker(homeworkService service.HomeworkServiceInterface, logger *zap.Logger) *HomeworkResetWorker {
	return &HomeworkResetWorker{
		homeworkService: homeworkService,
		logger:          logger,
	}
}

// Start initiates the background loop that triggers a homework reset at midnight.
func (w *HomeworkResetWorker) Start(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	w.logger.Info("Homework reset worker started")

	for {
		// Calculate time until midnight
		now := time.Now()
		nextReset := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		if nextReset.Before(now) {
			nextReset = nextReset.AddDate(0, 0, 1)
		}

		duration := time.Until(nextReset)
		w.logger.Info("Next homework reset scheduled", zap.Duration("until", duration))

		select {
		case <-time.After(duration):
			w.logger.Info("Executing scheduled homework reset")
			if err := w.homeworkService.ResetAllHomework(ctx); err != nil {
				w.logger.Error("Failed to reset homework", zap.Error(err))
			}
		case <-ctx.Done():
			w.logger.Info("Homework reset worker stopped")
			return
		}
	}
}
