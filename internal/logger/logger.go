package logger

import (
	"log/slog"
	"os"
)

// New builds the JSON stdout logger at LOG_LEVEL (info when unset or invalid).
func New() *slog.Logger {
	var level slog.Level
	if err := level.UnmarshalText([]byte(os.Getenv("LOG_LEVEL"))); err != nil {
		level = slog.LevelInfo
	}

	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		AddSource: true,
		Level:     level,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if len(groups) == 0 && a.Key == slog.TimeKey {
				a.Key = "timestamp"
			}
			return a
		},
	}))
}
