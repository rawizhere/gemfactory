// Package repository contains database implementation.
package repository

import (
	"context"
	"fmt"
	"gemfactory/internal/model"
	"time"

	"github.com/uptrace/bun"
	"go.uber.org/zap"
)

// HomeworkRepository provides persistent storage for user homework assignments.
type HomeworkRepository struct {
	db     *bun.DB
	logger *zap.Logger
}

// NewHomeworkRepository initializes a new HomeworkRepository.
func NewHomeworkRepository(db *bun.DB, logger *zap.Logger) *HomeworkRepository {
	return &HomeworkRepository{
		db:     db,
		logger: logger,
	}
}

// GetByID retrieves a single homework record by its unique ID.
func (r *HomeworkRepository) GetByID(ctx context.Context, id int) (*model.Homework, error) {
	homework := new(model.Homework)

	err := r.db.NewSelect().
		Model(homework).
		Where("homework_id = ?", id).
		Scan(ctx)

	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query homework by ID: %w", err)
	}

	return homework, nil
}

// GetAll retrieves all homework records, ordered by creation date descending.
func (r *HomeworkRepository) GetAll(ctx context.Context) ([]model.Homework, error) {
	var homework []model.Homework

	err := r.db.NewSelect().
		Model(&homework).
		Order("created_at DESC").
		Scan(ctx)

	if err != nil {
		return nil, fmt.Errorf("failed to query homework: %w", err)
	}

	return homework, nil
}

// GetByUserID retrieves all homework records associated with a specific user.
func (r *HomeworkRepository) GetByUserID(ctx context.Context, userID int64) ([]model.Homework, error) {
	var homework []model.Homework

	err := r.db.NewSelect().
		Model(&homework).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Scan(ctx)

	if err != nil {
		return nil, fmt.Errorf("failed to query homework: %w", err)
	}

	return homework, nil
}

// GetActiveByUserID retrieves the most recent pending homework record for a user.
func (r *HomeworkRepository) GetActiveByUserID(ctx context.Context, userID int64) (*model.Homework, error) {
	homework := new(model.Homework)

	err := r.db.NewSelect().
		Model(homework).
		Where("user_id = ? AND completed = false", userID).
		Order("created_at DESC").
		Limit(1).
		Scan(ctx)

	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to scan homework: %w", err)
	}

	return homework, nil
}

// Create inserts a new homework record into the database.
func (r *HomeworkRepository) Create(ctx context.Context, homework *model.Homework) error {

	_, err := r.db.NewInsert().
		Model(homework).
		Exec(ctx)

	if err != nil {
		return fmt.Errorf("failed to create homework: %w", err)
	}

	return nil
}

// Update modifies an existing homework record.
func (r *HomeworkRepository) Update(ctx context.Context, homework *model.Homework) error {

	_, err := r.db.NewUpdate().
		Model(homework).
		WherePK().
		Exec(ctx)

	if err != nil {
		return fmt.Errorf("failed to update homework: %w", err)
	}

	return nil
}

// Delete removes a homework record by its ID.
func (r *HomeworkRepository) Delete(ctx context.Context, id int) error {

	_, err := r.db.NewDelete().
		Model((*model.Homework)(nil)).
		Where("homework_id = ?", id).
		Exec(ctx)

	if err != nil {
		return fmt.Errorf("failed to delete homework: %w", err)
	}

	return nil
}

// MarkCompleted flags a specific homework record as finished.
func (r *HomeworkRepository) MarkCompleted(ctx context.Context, id int) error {

	_, err := r.db.NewUpdate().
		Model((*model.Homework)(nil)).
		Set("completed = true").
		Set("updated_at = NOW()").
		Where("homework_id = ?", id).
		Exec(ctx)

	if err != nil {
		return fmt.Errorf("failed to mark homework as completed: %w", err)
	}

	return nil
}

// GetRandomTrack retrieves a single random homework record from the database.
func (r *HomeworkRepository) GetRandomTrack(ctx context.Context) (*model.Homework, error) {
	homework := new(model.Homework)

	err := r.db.NewSelect().
		Model(homework).
		OrderExpr("RANDOM()").
		Limit(1).
		Scan(ctx)

	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to scan homework: %w", err)
	}

	return homework, nil
}

// CanRequestHomework verifies if a user is eligible to request new homework based on their pending assignments.
func (r *HomeworkRepository) CanRequestHomework(ctx context.Context, userID int64) (bool, error) {

	count, err := r.db.NewSelect().
		Model((*model.Homework)(nil)).
		Where("user_id = ? AND completed = false", userID).
		Count(ctx)

	if err != nil {
		return false, fmt.Errorf("failed to check homework count: %w", err)
	}

	return count == 0, nil
}

// GetLastRequestTime retrieves the timestamp of the user's most recent homework request.
func (r *HomeworkRepository) GetLastRequestTime(ctx context.Context, userID int64) (*time.Time, error) {
	var lastRequest time.Time

	err := r.db.NewSelect().
		Model((*model.Homework)(nil)).
		Column("created_at").
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Limit(1).
		Scan(ctx, &lastRequest)

	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to scan last request time: %w", err)
	}

	return &lastRequest, nil
}
