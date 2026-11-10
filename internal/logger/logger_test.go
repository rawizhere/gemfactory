package logger

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewLogger(t *testing.T) {
	tests := []struct {
		env  string
		want slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"error", slog.LevelError},
		{"fatal", slog.LevelInfo},
		{"unknown", slog.LevelInfo},
		{"", slog.LevelInfo},
	}

	for _, tt := range tests {
		t.Setenv("LOG_LEVEL", tt.env)
		l := New()
		ctx := context.Background()
		require.Equal(t, tt.want <= slog.LevelDebug, l.Enabled(ctx, slog.LevelDebug), "debug for %q", tt.env)
		require.Equal(t, tt.want <= slog.LevelInfo, l.Enabled(ctx, slog.LevelInfo), "info for %q", tt.env)
		require.Equal(t, tt.want <= slog.LevelWarn, l.Enabled(ctx, slog.LevelWarn), "warn for %q", tt.env)
		require.Equal(t, tt.want <= slog.LevelError, l.Enabled(ctx, slog.LevelError), "error for %q", tt.env)
	}
}
