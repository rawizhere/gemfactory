package scraper

import (
	"time"
)

type Release struct {
	Date       time.Time
	Artist     string
	Title      string
	AlbumName  string
	TitleTrack string
	MV         string
	Spotify    string
	SourceURL  string
}
