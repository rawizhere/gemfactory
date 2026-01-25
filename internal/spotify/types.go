// Package spotify defines data structures for handling Spotify-sourced music information.
package spotify

import "time"

// Track represents basic metadata for a single item within a Spotify playlist.
type Track struct {
	ID     string // Spotify track ID.
	Title  string // Track title.
	Artist string // Artist name.
}

// TrackInfo represents detailed track information.
type TrackInfo struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Artists  []string `json:"artists"`
	Duration int      `json:"duration"`
}

// PlaylistInfo represents detailed playlist information.
type PlaylistInfo struct {
	SpotifyID   string    `json:"spotify_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Owner       string    `json:"owner"`
	TrackCount  int       `json:"track_count"`
	LastUpdated time.Time `json:"last_updated"`
}
