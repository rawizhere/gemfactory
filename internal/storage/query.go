package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/uptrace/bun"
)

// one runs q into dst and returns it, or nil when no row matches.
func one[T any](ctx context.Context, q *bun.SelectQuery, dst *T, what string) (*T, error) {
	if err := q.Model(dst).Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query %s: %w", what, err)
	}
	return dst, nil
}
