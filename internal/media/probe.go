// Package media inspects media files with ffprobe.
package media

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// Info is what one ffprobe run reports about a file. Codec names are lower-case;
// durations are seconds, stream durations fall back to the container duration.
type Info struct {
	VideoCodec     string
	PixFmt         string
	AudioCodec     string
	Width          int
	Height         int
	VideoDuration  float64
	AudioDuration  float64
	FormatDuration float64
}

type probeOutput struct {
	Streams []struct {
		CodecType string `json:"codec_type"`
		CodecName string `json:"codec_name"`
		PixFmt    string `json:"pix_fmt"`
		Width     int    `json:"width"`
		Height    int    `json:"height"`
		Duration  string `json:"duration"`
	} `json:"streams"`
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
}

// Probe runs ffprobe once and returns the first video and audio stream details.
func Probe(path string) (*Info, error) {
	out, err := exec.Command("ffprobe", "-v", "error", "-of", "json", "-show_streams", "-show_format", path).Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe %s: %w", path, err)
	}
	return parseProbe(out)
}

func parseProbe(data []byte) (*Info, error) {
	var p probeOutput
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parse ffprobe output: %w", err)
	}
	info := &Info{FormatDuration: seconds(p.Format.Duration)}
	var videoSeen, audioSeen bool
	for _, s := range p.Streams {
		d := seconds(s.Duration)
		switch s.CodecType {
		case "video":
			if !videoSeen {
				videoSeen = true
				info.VideoCodec = strings.ToLower(s.CodecName)
				info.PixFmt = strings.ToLower(s.PixFmt)
				info.Width, info.Height = s.Width, s.Height
			}
			if info.VideoDuration == 0 {
				info.VideoDuration = d
			}
		case "audio":
			if !audioSeen {
				audioSeen = true
				info.AudioCodec = strings.ToLower(s.CodecName)
			}
			if info.AudioDuration == 0 {
				info.AudioDuration = d
			}
		}
	}
	if info.VideoDuration == 0 {
		info.VideoDuration = info.FormatDuration
	}
	if info.AudioDuration == 0 {
		info.AudioDuration = info.FormatDuration
	}
	return info, nil
}

// seconds parses an ffprobe duration ("12.345000", "N/A", ""); unknown or non-positive values are 0.
func seconds(s string) float64 {
	d, err := strconv.ParseFloat(s, 64)
	if err != nil || d <= 0 {
		return 0
	}
	return d
}
