// Package model defines core domain entities and data transfer objects.
package model

import (
	"context"
	"time"

	"github.com/uptrace/bun"
)

// Artist represents a music performer or group within the system.
type Artist struct {
	bun.BaseModel `bun:"table:gemfactory.artists"`

	ArtistID  int       `bun:"artist_id,pk,autoincrement" json:"artist_id"`
	Name      string    `bun:"name,unique,notnull" json:"name"`
	Gender    Gender    `bun:"gender,notnull,default:'male'" json:"gender"`
	IsActive  bool      `bun:"is_active,notnull,default:true" json:"is_active"`
	CreatedAt time.Time `bun:"created_at,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt time.Time `bun:"updated_at,notnull,default:current_timestamp" json:"updated_at"`
}

// IsFemale checks if the artist is female.
func (a *Artist) IsFemale() bool {
	return a.Gender == GenderFemale
}

// SetGender sets the artist's gender.
func (a *Artist) SetGender(isFemale bool) {
	if isFemale {
		a.Gender = GenderFemale
	} else {
		a.Gender = GenderMale
	}
}

// GetDisplayName returns the cleaned display name of the artist.
func (a *Artist) GetDisplayName() string {
	return GetUtils().CleanText(a.Name)
}

// ArtistRepository defines the interface for artist operations.
type ArtistRepository interface {
	Repository[Artist]
	GetByGender(ctx context.Context, gender Gender) ([]Artist, error)
	GetByName(ctx context.Context, name string) (*Artist, error)
	GetActive(ctx context.Context) ([]Artist, error)
	GetByGenderAndActive(ctx context.Context, gender Gender, active bool) ([]Artist, error)
	Upsert(ctx context.Context, artists []Artist) error
}
