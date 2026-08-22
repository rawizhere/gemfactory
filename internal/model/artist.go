package model

import (
	"context"
	"time"

	"github.com/uptrace/bun"
)

type Artist struct {
	bun.BaseModel `bun:"table:gemfactory.artists"`

	ArtistID  int          `bun:"artist_id,pk,autoincrement" json:"artist_id"`
	Name      UniqueString `bun:"name,unique,notnull" json:"name"`
	Gender    Gender       `bun:"gender,notnull" json:"gender"`
	IsActive  bool         `bun:"is_active,notnull,default:true" json:"is_active"`
	CreatedAt time.Time    `bun:"created_at,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt time.Time    `bun:"updated_at,notnull,default:current_timestamp" json:"updated_at"`

	Releases []Release `bun:"rel:has-many,join:artist_id=artist_id" json:"releases,omitempty"`
}

func (a *Artist) IsFemale() bool {
	return a.Gender == GenderFemale
}

func (a *Artist) IsMale() bool {
	return a.Gender == GenderMale
}

type ArtistRepository interface {
	GetByID(ctx context.Context, id int) (*Artist, error)
	GetByName(ctx context.Context, name string) (*Artist, error)
	GetAll(ctx context.Context) ([]Artist, error)
	GetActive(ctx context.Context) ([]Artist, error)
	GetByGender(ctx context.Context, gender Gender) ([]Artist, error)
	GetByGenderAndActive(ctx context.Context, gender Gender, isActive bool) ([]Artist, error)
	Create(ctx context.Context, artist *Artist) error
	Update(ctx context.Context, artist *Artist) error
	Delete(ctx context.Context, id int) error
	Upsert(ctx context.Context, artists []Artist) error
	DeactivateByNames(ctx context.Context, names []string) (int, error)
}
