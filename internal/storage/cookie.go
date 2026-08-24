package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"gemfactory/internal/model"
	"strings"

	"github.com/uptrace/bun"
	"go.uber.org/zap"
)

type CookieRepository struct {
	db     *bun.DB
	logger *zap.Logger
}

func NewCookieRepository(db *bun.DB, logger *zap.Logger) model.CookieRepository {
	return &CookieRepository{db: db, logger: logger}
}

func (r *CookieRepository) GetByDomain(ctx context.Context, domain string) (*model.Cookie, error) {
	cookie := new(model.Cookie)
	err := r.db.NewSelect().
		Model(cookie).
		Where("domain = ?", domain).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query cookie by domain: %w", err)
	}
	return cookie, nil
}

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
