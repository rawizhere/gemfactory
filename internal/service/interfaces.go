package service

import (
	"context"
	"gemfactory/internal/model"
	"gemfactory/internal/spotify"
	"time"
)

// ArtistServiceInterface defines the core operations for artist management.
type ArtistServiceInterface interface {
	Add(ctx context.Context, artists []string, isFemale bool) (int, error)
	Remove(ctx context.Context, artists []string) (int, error)
	Deactivate(ctx context.Context, artists []string) (int, error)
	GetFemaleArtists(ctx context.Context) ([]string, error)
	GetMaleArtists(ctx context.Context) ([]string, error)
	GetAll(ctx context.Context) ([]model.Artist, error)
	GetAllActive(ctx context.Context) ([]model.Artist, error)
	Export(ctx context.Context) (string, error)
	FormatList(ctx context.Context) (string, error)
	GetCounts(ctx context.Context) (femaleCount, maleCount, totalCount int, err error)
}

// ReleaseServiceInterface defines the core operations for release management and parsing.
type ReleaseServiceInterface interface {
	GetReleasesForMonth(ctx context.Context, month string, femaleOnly, maleOnly bool) (string, error)
	Upsert(ctx context.Context, release *model.Release) error
	Update(ctx context.Context, release *model.Release) error
	FormatReleaseForDisplay(release *model.Release) string
	FormatDate(dateStr string) (string, error)
	FormatTimeKST(timeStr string) (string, error)
	ConvertKSTToMSK(kstTimeStr string) (string, error)
	CleanLink(link string) string
	FormatReleaseForTelegram(release *model.Release) string
	Create(ctx context.Context, release *model.Release) error
	Delete(ctx context.Context, id int) error
	GetByArtistID(ctx context.Context, artistID int) ([]model.Release, error)
	GetReleasesByGender(ctx context.Context, gender string) ([]model.Release, error)
	GetAllReleases(ctx context.Context) ([]model.Release, error)
	ParseReleasesForMonth(ctx context.Context, month string) (int, error)
	GetByArtist(ctx context.Context, artistName string) (string, error)
	GetTotalReleaseCount(ctx context.Context) (int, error)
}

// HomeworkServiceInterface defines the core operations for user homework assignments.
type HomeworkServiceInterface interface {
	GetRandom(ctx context.Context, userID int64) (*model.Homework, error)
	MarkCompleted(ctx context.Context, userID int64, trackID string) error
	GetUserHomework(ctx context.Context, userID int64) ([]model.HomeworkTracking, error)
	GetPendingHomework(ctx context.Context, userID int64) ([]model.HomeworkTracking, error)
	CanRequest(ctx context.Context, userID int64) (bool, error)
	GetTimeUntilNext(ctx context.Context, userID int64) time.Duration
	GetActive(ctx context.Context, userID int64) (*model.Homework, error)
	ResetAllHomework(ctx context.Context) error
	GetStats(ctx context.Context) (*HomeworkStats, error)
}

// PlaylistServiceInterface defines the core operations for Spotify playlist synchronization.
type PlaylistServiceInterface interface {
	Reload(ctx context.Context) error
	Update(ctx context.Context) error
	GetTracks(ctx context.Context) ([]model.PlaylistTracks, error)
	GetInfo(ctx context.Context) (*spotify.PlaylistInfo, error)
}

// ConfigServiceInterface provides access to the application's persistent configuration.
type ConfigServiceInterface interface {
	Get(ctx context.Context, key string) (string, error)
	GetAll(ctx context.Context) (string, error)
	Update(ctx context.Context, key, value string) error
	GetAllRaw(ctx context.Context) ([]model.Config, error)
}

// Configurable describes components that can react to dynamic configuration changes.
type Configurable interface {
	ApplyConfig(ctx context.Context, configs []model.Config) error
}

// ConfigWatcherInterface defines the behavior for monitoring configuration changes.
type ConfigWatcherInterface interface {
	Start(ctx context.Context)
	Stop()
}
