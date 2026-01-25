// Package spotify provides a client for interacting with the Spotify Web API.
package spotify

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/sony/gobreaker"
	"github.com/zmb3/spotify/v2"
	"go.uber.org/zap"
)

// tokenTransport provides an http.RoundTripper that injects Authorization headers into requests.
type tokenTransport struct {
	base      http.RoundTripper
	token     string
	tokenType string
}

func (t *tokenTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", t.tokenType+" "+t.token)

	// Use DefaultTransport if base is nil.
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}

	return base.RoundTrip(req)
}

// Client represents a client for working with the Spotify API.
type Client struct {
	clientID     string
	clientSecret string
	cb           *gobreaker.CircuitBreaker
	logger       *zap.Logger
}

// NewClient creates a new Spotify client using the Client Credentials Flow.
func NewClient(clientID, clientSecret string, logger *zap.Logger) (*Client, error) {
	if clientID == "" || clientSecret == "" {
		return nil, fmt.Errorf("spotify client ID and secret are required")
	}

	cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name: "Spotify API",
		OnStateChange: func(name string, from, to gobreaker.State) {
			logger.Warn("Spotify Circuit Breaker state change",
				zap.String("from", from.String()),
				zap.String("to", to.String()))
		},
	})

	return &Client{
		clientID:     clientID,
		clientSecret: clientSecret,
		cb:           cb,
		logger:       logger,
	}, nil
}

// createSpotifyClient initializes an authenticated Spotify API client for a single operation.
func (c *Client) createSpotifyClient() (*spotify.Client, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create HTTP client.
	httpClient := &http.Client{}

	// Prepare data for token request according to Spotify documentation.
	data := url.Values{}
	data.Set("grant_type", "client_credentials")

	// Create request.
	req, err := http.NewRequestWithContext(ctx, "POST", "https://accounts.spotify.com/api/token", strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create token request: %w", err)
	}

	// Set headers according to documentation.
	credentials := base64.StdEncoding.EncodeToString([]byte(c.clientID + ":" + c.clientSecret))
	req.Header.Set("Authorization", "Basic "+credentials)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Execute request.
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get token: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			c.logger.Warn("Failed to close response body", zap.Error(closeErr))
		}
	}()

	// Check response status.
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token request failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Parse response.
	var tokenResponse struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tokenResponse); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}

	if tokenResponse.AccessToken == "" {
		return nil, fmt.Errorf("no access token received")
	}

	// Create HTTP client with token in headers.
	tokenClient := &http.Client{
		Transport: &tokenTransport{
			base:      http.DefaultTransport, // Use DefaultTransport instead of nil.
			token:     tokenResponse.AccessToken,
			tokenType: tokenResponse.TokenType,
		},
	}

	// Create Spotify client with HTTP client that automatically adds token.
	client := spotify.New(tokenClient)

	c.logger.Debug("Created new Spotify client for request")

	return client, nil
}

// ExtractID extracts the playlist ID from a given URL.
func (c *Client) ExtractID(playlistURL string) (string, error) {
	// Support different URL formats.
	// https://open.spotify.com/playlist/37i9dQZF1DXcBWIGoYBM5M
	// spotify:playlist:37i9dQZF1DXcBWIGoYBM5M
	if strings.HasPrefix(playlistURL, "spotify:playlist:") {
		return strings.TrimPrefix(playlistURL, "spotify:playlist:"), nil
	}

	if strings.Contains(playlistURL, "open.spotify.com/playlist/") {
		parts := strings.Split(playlistURL, "/playlist/")
		if len(parts) != 2 {
			return "", fmt.Errorf("invalid playlist URL format")
		}
		playlistID := strings.Split(parts[1], "?")[0]
		return playlistID, nil
	}

	return "", fmt.Errorf("unsupported playlist URL format")
}

// GetTracks retrieves all tracks from a public Spotify playlist.
func (c *Client) GetTracks(playlistURL string) ([]*Track, error) {
	playlistID, err := c.ExtractID(playlistURL)
	if err != nil {
		return nil, fmt.Errorf("failed to extract playlist ID: %w", err)
	}

	// Create new Spotify client for each request.
	c.logger.Debug("Creating new Spotify client for playlist tracks request")
	client, err := c.createSpotifyClient()
	if err != nil {
		c.logger.Error("Failed to create Spotify client", zap.Error(err))
		return nil, fmt.Errorf("failed to create spotify client: %w", err)
	}

	ctx := context.Background()

	var allTracks []*Track
	offset := 0
	limit := 100 // Max page size for Spotify API.

	c.logger.Debug("Starting pagination to get all playlist tracks",
		zap.String("playlist_id", playlistID))

	for {
		// Get playlist tracks page.
		c.logger.Debug("Requesting playlist items page",
			zap.String("playlist_id", playlistID),
			zap.Int("offset", offset),
			zap.Int("limit", limit))

		// Get playlist tracks page via Circuit Breaker.
		result, err := c.cb.Execute(func() (interface{}, error) {
			return client.GetPlaylistItems(ctx, spotify.ID(playlistID), spotify.Limit(limit), spotify.Offset(offset))
		})
		if err != nil {
			c.logger.Error("Spotify API request failed (Circuit Breaker)",
				zap.String("playlist_id", playlistID),
				zap.Int("offset", offset),
				zap.Error(err))
			return nil, fmt.Errorf("failed to get playlist tracks at offset %d: %w", offset, err)
		}
		tracks := result.(*spotify.PlaylistItemPage)

		c.logger.Debug("Retrieved playlist items page",
			zap.String("playlist_id", playlistID),
			zap.Int("offset", offset),
			zap.Int("items_in_page", len(tracks.Items)),
			zap.Int("total_items", int(tracks.Total)))

		// Process tracks on the current page.
		for _, item := range tracks.Items {
			// Check if it is a track, not an episode.
			if item.Track.Track == nil {
				continue
			}

			artistName := "Unknown Artist"
			if len(item.Track.Track.Artists) > 0 {
				artistName = item.Track.Track.Artists[0].Name
			}

			allTracks = append(allTracks, &Track{
				ID:     string(item.Track.Track.ID),
				Title:  item.Track.Track.Name,
				Artist: artistName,
			})
		}

		// Check if there are more pages
		if offset+len(tracks.Items) >= int(tracks.Total) {
			break
		}

		offset += len(tracks.Items)
	}

	c.logger.Info("Successfully retrieved all tracks from playlist",
		zap.String("playlist_id", playlistID),
		zap.Int("total_tracks", len(allTracks)))

	return allTracks, nil
}

// GetInfo retrieves descriptive information and track counts for a specific Spotify playlist.
func (c *Client) GetInfo(playlistURL string) (*PlaylistInfo, error) {
	playlistID, err := c.ExtractID(playlistURL)
	if err != nil {
		return nil, fmt.Errorf("failed to extract playlist ID: %w", err)
	}

	client, err := c.createSpotifyClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create spotify client: %w", err)
	}

	ctx := context.Background()

	c.logger.Debug("Requesting playlist info from Spotify API",
		zap.String("playlist_id", playlistID),
		zap.String("playlist_url", playlistURL))

	result, err := c.cb.Execute(func() (interface{}, error) {
		return client.GetPlaylist(ctx, spotify.ID(playlistID))
	})
	if err != nil {
		c.logger.Error("Failed to get playlist from Spotify API (Circuit Breaker)",
			zap.String("playlist_id", playlistID),
			zap.String("playlist_url", playlistURL),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get playlist: %w", err)
	}
	playlist := result.(*spotify.FullPlaylist)

	return &PlaylistInfo{
		SpotifyID:   string(playlist.ID),
		Name:        playlist.Name,
		Description: playlist.Description,
		Owner:       playlist.Owner.DisplayName,
		TrackCount:  int(playlist.Tracks.Total),
		LastUpdated: time.Now(),
	}, nil
}
