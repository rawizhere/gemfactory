// Package repository provides database-specific implementations of domain repositories.
package repository

import (
	"context"
	"fmt"
	"time"

	"gemfactory/internal/model"

	"github.com/uptrace/bun"
	"go.uber.org/zap"
)

// HomeworkTrackingRepository manages persistent storage for tracking user homework progress.
type HomeworkTrackingRepository struct {
	db     *bun.DB
	logger *zap.Logger
}

// NewHomeworkTrackingRepository initializes a new HomeworkTrackingRepository.
func NewHomeworkTrackingRepository(db *bun.DB, logger *zap.Logger) *HomeworkTrackingRepository {
	return &HomeworkTrackingRepository{
		db:     db,
		logger: logger,
	}
}

// GetByUserID retrieves all tracking records for a specific user, ordered by issue date descending.
func (r *HomeworkTrackingRepository) GetByUserID(ctx context.Context, userID int64) ([]model.HomeworkTracking, error) {
	var trackings []model.HomeworkTracking

	err := r.db.NewSelect().
		Model(&trackings).
		Where("user_id = ?", userID).
		Order("issued_at DESC").
		Scan(ctx)

	if err != nil {
		return nil, fmt.Errorf("failed to get homework tracking by user_id: %w", err)
	}

	return trackings, nil
}

// GetCompletedByUserID retrieves all completed tracking records for a specific user.
func (r *HomeworkTrackingRepository) GetCompletedByUserID(ctx context.Context, userID int64) ([]model.HomeworkTracking, error) {
	var trackings []model.HomeworkTracking

	err := r.db.NewSelect().
		Model(&trackings).
		Where("user_id = ? AND is_completed = true", userID).
		Order("completed_at DESC").
		Scan(ctx)

	if err != nil {
		return nil, fmt.Errorf("failed to get completed homework tracking by user_id: %w", err)
	}

	return trackings, nil
}

// GetPendingByUserID retrieves all pending (incomplete) tracking records for a specific user.
func (r *HomeworkTrackingRepository) GetPendingByUserID(ctx context.Context, userID int64) ([]model.HomeworkTracking, error) {
	var trackings []model.HomeworkTracking

	err := r.db.NewSelect().
		Model(&trackings).
		Where("user_id = ? AND is_completed = false", userID).
		Order("issued_at ASC").
		Scan(ctx)

	if err != nil {
		return nil, fmt.Errorf("failed to get pending homework tracking by user_id: %w", err)
	}

	return trackings, nil
}

// Create inserts a new tracking record, updating the issue date on conflict.
func (r *HomeworkTrackingRepository) Create(ctx context.Context, tracking *model.HomeworkTracking) error {

	_, err := r.db.NewInsert().
		Model(tracking).
		On("CONFLICT (user_id, track_id, spotify_id) DO UPDATE").
		Set("issued_at = EXCLUDED.issued_at").
		Set("updated_at = CURRENT_TIMESTAMP").
		Exec(ctx)

	if err != nil {
		return fmt.Errorf("failed to create homework tracking: %w", err)
	}

	return nil
}

// Update modifies an existing tracking record.
func (r *HomeworkTrackingRepository) Update(ctx context.Context, tracking *model.HomeworkTracking) error {

	_, err := r.db.NewUpdate().
		Model(tracking).
		Where("id = ?", tracking.ID).
		Exec(ctx)

	if err != nil {
		return fmt.Errorf("failed to update homework tracking: %w", err)
	}

	return nil
}

// MarkCompleted flags a specific track as finished for a user.
func (r *HomeworkTrackingRepository) MarkCompleted(ctx context.Context, userID int64, trackID string, spotifyID string) error {
	now := time.Now()

	_, err := r.db.NewUpdate().
		Model((*model.HomeworkTracking)(nil)).
		Set("is_completed = true").
		Set("completed_at = ?", now).
		Set("updated_at = ?", now).
		Where("user_id = ? AND track_id = ? AND spotify_id = ?", userID, trackID, spotifyID).
		Exec(ctx)

	if err != nil {
		return fmt.Errorf("failed to mark homework as completed: %w", err)
	}

	return nil
}

// GetIssuedTrackIDs retrieves a list of track IDs already assigned to a user for a specific playlist.
func (r *HomeworkTrackingRepository) GetIssuedTrackIDs(ctx context.Context, userID int64, spotifyID string) ([]string, error) {
	var trackIDs []string

	err := r.db.NewSelect().
		Model((*model.HomeworkTracking)(nil)).
		Column("track_id").
		Where("user_id = ? AND spotify_id = ?", userID, spotifyID).
		Scan(ctx, &trackIDs)

	if err != nil {
		return nil, fmt.Errorf("failed to get issued track IDs: %w", err)
	}

	return trackIDs, nil
}

// GetLastRequestTime retrieves the timestamp of the user's most recent homework assignment.
func (r *HomeworkTrackingRepository) GetLastRequestTime(ctx context.Context, userID int64) (*time.Time, error) {
	var lastTime time.Time

	err := r.db.NewSelect().
		Model((*model.HomeworkTracking)(nil)).
		Column("issued_at").
		Where("user_id = ?", userID).
		Order("issued_at DESC").
		Limit(1).
		Scan(ctx, &lastTime)

	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get last request time: %w", err)
	}

	return &lastTime, nil
}

// GetAllPending retrieves all incomplete tracking records across all users.
func (r *HomeworkTrackingRepository) GetAllPending(ctx context.Context) ([]model.HomeworkTracking, error) {

	var trackings []model.HomeworkTracking
	err := r.db.NewSelect().
		Model(&trackings).
		Where("is_completed = ?", false).
		Order("issued_at ASC").
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get all pending homework: %w", err)
	}
	return trackings, nil
}

// GetTotalAssignedCount returns the total number of homework assignments ever issued.
func (r *HomeworkTrackingRepository) GetTotalAssignedCount(ctx context.Context) (int, error) {

	count, err := r.db.NewSelect().
		Model((*model.HomeworkTracking)(nil)).
		Count(ctx)

	if err != nil {
		return 0, fmt.Errorf("failed to count homework assignments: %w", err)
	}

	return count, nil
}

// GetUniqueUsersCount returns the total number of distinct users who have received homework.
func (r *HomeworkTrackingRepository) GetUniqueUsersCount(ctx context.Context) (int, error) {

	var count int
	err := r.db.NewSelect().
		Model((*model.HomeworkTracking)(nil)).
		ColumnExpr("COUNT(DISTINCT user_id)").
		Scan(ctx, &count)

	if err != nil {
		return 0, fmt.Errorf("failed to count unique users: %w", err)
	}

	return count, nil
}
