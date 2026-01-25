// Package service contains business logic.
package service

import (
	"context"
	"fmt"
	"gemfactory/internal/model"
	"gemfactory/internal/storage/repository"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/uptrace/bun"
	"go.uber.org/zap"
)

// HomeworkService manages user homework assignments and tracking.
type HomeworkService struct {
	playlistRepo    model.PlaylistTracksRepository
	trackingRepo    model.HomeworkTrackingRepository
	playlistService PlaylistServiceInterface
	resetTime       string
	logger          *zap.Logger
}

func NewHomeworkService(db *bun.DB, playlistService PlaylistServiceInterface, resetTime string, logger *zap.Logger) *HomeworkService {
	if resetTime == "" {
		resetTime = "00:00"
	}
	return &HomeworkService{
		playlistRepo:    repository.NewPlaylistTracksRepository(db, logger),
		trackingRepo:    repository.NewHomeworkTrackingRepository(db, logger),
		playlistService: playlistService,
		resetTime:       resetTime,
		logger:          logger,
	}
}

// GetRandom provides a new random homework assignment for the user.
func (s *HomeworkService) GetRandom(ctx context.Context, userID int64) (*model.Homework, error) {
	if s.playlistService == nil {
		return nil, fmt.Errorf("playlist service not available")
	}

	playlistInfo, err := s.playlistService.GetInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get playlist info: %w", err)
	}

	spotifyID := playlistInfo.SpotifyID
	s.logger.Info("Using Spotify ID from playlist service", zap.String("spotify_id", spotifyID))

	canRequest, err := s.canUserRequestHomework(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to check if user can request homework: %w", err)
	}

	if !canRequest {
		return nil, fmt.Errorf("user cannot request homework yet, please wait")
	}

	issuedTrackIDs, err := s.trackingRepo.GetIssuedTrackIDs(ctx, userID, spotifyID)
	if err != nil {
		return nil, fmt.Errorf("failed to get issued track IDs: %w", err)
	}

	s.logger.Info("Getting random track", zap.String("spotify_id", spotifyID), zap.Strings("exclude_track_ids", issuedTrackIDs))
	track, err := s.playlistRepo.GetRandomTrack(ctx, spotifyID, issuedTrackIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to get random track from playlist: %w", err)
	}

	s.logger.Info("Got track from playlist", zap.Bool("track_found", track != nil))
	if track != nil {
		s.logger.Info("Track details", zap.String("track_id", track.TrackID), zap.String("artist", track.Artist), zap.String("title", track.Title))
	}

	if track == nil { // All tracks issued; return first pending one.
		pendingTrackings, err := s.trackingRepo.GetPendingByUserID(ctx, userID)
		if err != nil {
			return nil, fmt.Errorf("failed to get pending homework: %w", err)
		}

		if len(pendingTrackings) == 0 {
			return nil, fmt.Errorf("no tracks available for homework")
		}

		pending := pendingTrackings[0]
		return &model.Homework{
			UserID:    userID,
			TrackID:   pending.TrackID,
			Artist:    "",
			Title:     "",
			PlayCount: pending.PlayCount,
			Completed: false,
		}, nil
	}

	playCount := rand.Intn(6) + 1

	tracking := &model.HomeworkTracking{
		UserID:    userID,
		TrackID:   track.TrackID,
		SpotifyID: spotifyID,
		PlayCount: playCount,
		IssuedAt:  time.Now(),
	}

	err = s.trackingRepo.Create(ctx, tracking)
	if err != nil {
		return nil, fmt.Errorf("failed to create homework tracking: %w", err)
	}

	return &model.Homework{
		UserID:    userID,
		TrackID:   track.TrackID,
		Artist:    track.Artist,
		Title:     track.Title,
		PlayCount: playCount,
		Completed: false,
	}, nil
}

// MarkCompleted updates the status of a specific homework assignment to completed.
func (s *HomeworkService) MarkCompleted(ctx context.Context, userID int64, trackID string) error {
	if s.playlistService == nil {
		return fmt.Errorf("playlist service not available")
	}

	playlistInfo, err := s.playlistService.GetInfo(ctx)
	if err != nil {
		return fmt.Errorf("failed to get playlist info: %w", err)
	}

	spotifyID := playlistInfo.SpotifyID
	if spotifyID == "" {
		return fmt.Errorf("failed to extract Spotify ID from playlist URL")
	}

	err = s.trackingRepo.MarkCompleted(ctx, userID, trackID, spotifyID)
	if err != nil {
		return fmt.Errorf("failed to mark homework as completed: %w", err)
	}

	return nil
}

// GetUserHomework retrieves all homework history for a user.
func (s *HomeworkService) GetUserHomework(ctx context.Context, userID int64) ([]model.HomeworkTracking, error) {
	return s.trackingRepo.GetByUserID(ctx, userID)
}

// GetPendingHomework retrieves currently active assignments for a user.
func (s *HomeworkService) GetPendingHomework(ctx context.Context, userID int64) ([]model.HomeworkTracking, error) {
	return s.trackingRepo.GetPendingByUserID(ctx, userID)
}

// CanRequest verifies if a user is eligible for a new assignment.
func (s *HomeworkService) CanRequest(ctx context.Context, userID int64) (bool, error) {
	return s.canUserRequestHomework(ctx, userID)
}

// GetTimeUntilNext calculates the duration until the user's next assignment becomes available.
func (s *HomeworkService) GetTimeUntilNext(ctx context.Context, userID int64) time.Duration {
	resetTime, err := s.getHomeworkResetTime(ctx)
	if err != nil {
		s.logger.Error("Failed to get homework reset time", zap.Error(err))
		return 0
	}

	timeParts := strings.Split(resetTime, ":")
	if len(timeParts) != 2 {
		s.logger.Error("Invalid time format", zap.String("time", resetTime))
		return 0
	}

	hour, err := strconv.Atoi(timeParts[0])
	if err != nil {
		s.logger.Error("Invalid hour", zap.String("hour", timeParts[0]), zap.Error(err))
		return 0
	}

	minute, err := strconv.Atoi(timeParts[1])
	if err != nil {
		s.logger.Error("Invalid minute", zap.String("minute", timeParts[1]), zap.Error(err))
		return 0
	}

	now := time.Now()
	nextReset := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())

	// If reset time has passed today, next reset is tomorrow.
	if nextReset.Before(now) {
		nextReset = nextReset.AddDate(0, 0, 1)
	}

	return nextReset.Sub(now)
}

// GetActive retrieves the current active homework details for a user.
func (s *HomeworkService) GetActive(ctx context.Context, userID int64) (*model.Homework, error) {
	pendingTrackings, err := s.trackingRepo.GetPendingByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get pending homework: %w", err)
	}

	if len(pendingTrackings) == 0 {
		return nil, nil // No active homework.
	}

	latest := pendingTrackings[0]

	tracks, err := s.playlistRepo.GetBySpotifyID(ctx, latest.SpotifyID)
	if err != nil {
		return nil, fmt.Errorf("failed to get track info: %w", err)
	}

	var track *model.PlaylistTracks
	for _, t := range tracks {
		if t.TrackID == latest.TrackID {
			track = &t
			break
		}
	}

	if track == nil {
		return nil, fmt.Errorf("track not found in playlist")
	}

	return &model.Homework{
		UserID:    userID,
		TrackID:   track.TrackID,
		Artist:    track.Artist,
		Title:     track.Title,
		PlayCount: latest.PlayCount,
		Completed: false,
	}, nil
}

// ResetAllHomework forcefully completes all currently pending homework assignments.
func (s *HomeworkService) ResetAllHomework(ctx context.Context) error {
	s.logger.Info("Starting homework reset for all users")

	pendingTrackings, err := s.trackingRepo.GetAllPending(ctx)
	if err != nil {
		return fmt.Errorf("failed to get pending homework: %w", err)
	}

	if len(pendingTrackings) == 0 {
		s.logger.Info("No pending homework to reset")
		return nil
	}

	resetCount := 0
	for _, tracking := range pendingTrackings {
		err = s.trackingRepo.MarkCompleted(ctx, tracking.UserID, tracking.TrackID, tracking.SpotifyID)
		if err != nil {
			s.logger.Error("Failed to mark homework as completed during reset",
				zap.Int64("user_id", tracking.UserID),
				zap.String("track_id", tracking.TrackID),
				zap.Error(err))
			continue
		}
		resetCount++
	}

	s.logger.Info("Homework reset completed", zap.Int("reset_count", resetCount))
	return nil
}

// canUserRequestHomework checks if user can request new homework based on reset time.
func (s *HomeworkService) canUserRequestHomework(ctx context.Context, userID int64) (bool, error) {
	resetTime, err := s.getHomeworkResetTime(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to get homework reset time: %w", err)
	}

	timeParts := strings.Split(resetTime, ":")
	if len(timeParts) != 2 {
		return false, fmt.Errorf("invalid time format: %s, expected HH:MM", resetTime)
	}

	hour, err := strconv.Atoi(timeParts[0])
	if err != nil {
		return false, fmt.Errorf("invalid hour: %s", timeParts[0])
	}

	minute, err := strconv.Atoi(timeParts[1])
	if err != nil {
		return false, fmt.Errorf("invalid minute: %s", timeParts[1])
	}

	lastTime, err := s.trackingRepo.GetLastRequestTime(ctx, userID)
	if err != nil {
		return false, fmt.Errorf("failed to get last request time: %w", err)
	}

	// Allow request if no previous homework.
	if lastTime == nil {
		return true, nil
	}

	now := time.Now()
	nextReset := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())

	// If reset time has passed today, next reset is tomorrow.
	if nextReset.Before(now) {
		nextReset = nextReset.AddDate(0, 0, 1)
	}

	return lastTime.Before(nextReset.AddDate(0, 0, -1)), nil
}

func (s *HomeworkService) getHomeworkResetTime(ctx context.Context) (string, error) {
	return s.resetTime, nil
}

// HomeworkStats contains basic utilization metrics.
type HomeworkStats struct {
	TotalAssigned int
	UniqueUsers   int
}

// GetStats retrieves global homework assignment metrics.
func (s *HomeworkService) GetStats(ctx context.Context) (*HomeworkStats, error) {
	totalAssigned, err := s.trackingRepo.GetTotalAssignedCount(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get total assigned count: %w", err)
	}

	uniqueUsers, err := s.trackingRepo.GetUniqueUsersCount(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get unique users count: %w", err)
	}

	return &HomeworkStats{
		TotalAssigned: totalAssigned,
		UniqueUsers:   uniqueUsers,
	}, nil
}
