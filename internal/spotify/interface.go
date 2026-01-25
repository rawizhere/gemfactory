// Package spotify defines the contract for interacting with Spotify's external services.
package spotify

// Interface defines the Spotify API contract.
type Interface interface {
	// ExtractPlaylistID extracts the playlist ID from a URL.
	ExtractPlaylistID(playlistURL string) (string, error)

	// GetPlaylistTracks retrieves tracks from a public playlist.
	GetPlaylistTracks(playlistURL string) ([]*Track, error)

	// GetPlaylistInfo gets information about a playlist.
	GetPlaylistInfo(playlistURL string) (*PlaylistInfo, error)
}
