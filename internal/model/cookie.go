package model

import (
	"context"
	"time"

	"github.com/uptrace/bun"
)

type Cookie struct {
	bun.BaseModel `bun:"table:gemfactory.cookies,alias:c"`

	CookieID  int       `bun:"cookie_id,pk,autoincrement" json:"id"`
	Domain    string    `bun:"domain,notnull,unique" json:"domain"`
	Content   string    `bun:"content,notnull" json:"content"`
	CreatedAt time.Time `bun:"created_at,nullzero,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt time.Time `bun:"updated_at,nullzero,notnull,default:current_timestamp" json:"updated_at"`
}

type CookieSummary struct {
	CookieID  int       `json:"id"`
	Domain    string    `json:"domain"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CookieRepository interface {
	GetByDomain(ctx context.Context, domain string) (*Cookie, error)
	GetAll(ctx context.Context) ([]CookieSummary, error)
	Upsert(ctx context.Context, domain, content string) error
	Delete(ctx context.Context, domain string) (int, error)
}
