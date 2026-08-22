package repository

import (
	"context"
	"fmt"
	"gemfactory/internal/model"
	"strings"

	"github.com/uptrace/bun"
	"go.uber.org/zap"
)

// CookieRepository manages persistent storage for yt-dlp cookies.
type CookieRepository struct {
	db     *bun.DB
	logger *zap.Logger
}

// NewCookieRepository initializes a new CookieRepository.
func NewCookieRepository(db *bun.DB, logger *zap.Logger) model.CookieRepository {
	return &CookieRepository{db: db, logger: logger}
}

// GetByDomain retrieves a cookie record by domain. Returns nil when not found.
func (r *CookieRepository) GetByDomain(ctx context.Context, domain string) (*model.Cookie, error) {
	cookie := new(model.Cookie)
	err := r.db.NewSelect().
		Model(cookie).
		Where("domain = ?", domain).
		Scan(ctx)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query cookie by domain: %w", err)
	}
	return cookie, nil
}

// GetAll retrieves all cookie domains without contents.
func (r *CookieRepository) GetAll(ctx context.Context) ([]model.CookieSummary, error) {
	var cookies []model.Cookie
	err := r.db.NewSelect().
		Model(&cookies).
		Column("cookie_id", "domain", "created_at", "updated_at").
		Order("domain ASC").
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query cookies: %w", err)
	}
	summaries := make([]model.CookieSummary, len(cookies))
	for i, c := range cookies {
		summaries[i] = model.CookieSummary{
			CookieID:  c.CookieID,
			Domain:    c.Domain,
			CreatedAt: c.CreatedAt,
			UpdatedAt: c.UpdatedAt,
		}
	}
	return summaries, nil
}

// Upsert inserts or updates cookies for the domain.
func (r *CookieRepository) Upsert(ctx context.Context, domain, content string) error {
	cookie := &model.Cookie{
		Domain:  domain,
		Content: strings.TrimSpace(content),
	}
	_, err := r.db.NewInsert().
		Model(cookie).
		On("CONFLICT (domain) DO UPDATE SET content = EXCLUDED.content, updated_at = NOW()").
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to upsert cookie for domain %q: %w", domain, err)
	}
	return nil
}

// Delete removes cookies for the domain and returns affected row count.
func (r *CookieRepository) Delete(ctx context.Context, domain string) (int, error) {
	res, err := r.db.NewDelete().
		Model((*model.Cookie)(nil)).
		Where("domain = ?", domain).
		Exec(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to delete cookie for domain %q: %w", domain, err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}
