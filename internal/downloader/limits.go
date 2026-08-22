package downloader

import (
	"fmt"
	"os"
	"strconv"
)

// maxSegmentDuration returns the maximum allowed clip length in
// milliseconds for the given quality tier:
//   - HQ (2K): CLIP_MAX_DURATION_HQ_SECONDS, default 30s — heavy re-encode;
//   - normal:  CLIP_MAX_DURATION_SECONDS, default 300s (5 minutes).
func (s *Service) maxSegmentDuration(hq bool) float64 {
	if hq {
		if v := durationEnvMs("CLIP_MAX_DURATION_HQ_SECONDS"); v > 0 {
			return v
		}
		return 30 * 1000
	}
	if v := durationEnvMs("CLIP_MAX_DURATION_SECONDS"); v > 0 {
		return v
	}
	return 300 * 1000
}

func durationEnvMs(key string) float64 {
	raw := os.Getenv(key)
	if raw == "" {
		return 0
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || v <= 0 {
		return 0
	}
	return v * 1000
}

// MaxSegmentDurationSeconds exposes the configured limit for help texts.
func (s *Service) MaxSegmentDurationSeconds(hq bool) float64 {
	return s.maxSegmentDuration(hq) / 1000
}

// telegramFileLimitBytes returns the bot API upload limit, configurable via
// TG_FILE_LIMIT_MB (default 49MB to stay safely under the 50MB hard limit).
func telegramFileLimitBytes() int64 {
	const defMB = 49
	raw := os.Getenv("TG_FILE_LIMIT_MB")
	mb := defMB
	if raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			mb = v
		}
	}
	return int64(mb) * 1024 * 1024
}

// checkFileSize fails the job when the produced file exceeds the Telegram limit.
func checkFileSize(path string) error {
	st, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("output file missing: %w", err)
	}
	limit := telegramFileLimitBytes()
	if st.Size() > limit {
		return fmt.Errorf(
			"файл слишком большой: %.1f МБ (лимит Telegram для ботов — %d МБ). Попробуйте короче интервал",
			float64(st.Size())/(1024*1024), limit/(1024*1024))
	}
	return nil
}

func mustParsePair(start, end string) (float64, float64) {
	s, _ := ParseTimecode(start)
	e, _ := ParseTimecode(end)
	return s, e
}
