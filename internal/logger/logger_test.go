package logger

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"
)

func TestNewLogger(t *testing.T) {
	t.Setenv("LOG_LEVEL", "debug")
	l, err := New()
	require.NoError(t, err, "failed to create logger")
	require.Equal(t, zapcore.DebugLevel, l.Level.Level())

	l.SetLevel("warn")
	require.Equal(t, zapcore.WarnLevel, l.Level.Level())

	l.SetLevel("invalid_level")
	require.Equal(t, zapcore.WarnLevel, l.Level.Level(), "expected level to remain unchanged on invalid input")
}

func TestGetLogLevel(t *testing.T) {
	tests := []struct {
		env  string
		want zapcore.Level
	}{
		{"debug", zapcore.DebugLevel},
		{"info", zapcore.InfoLevel},
		{"warn", zapcore.WarnLevel},
		{"error", zapcore.ErrorLevel},
		{"fatal", zapcore.FatalLevel},
		{"unknown", zapcore.InfoLevel},
	}

	for _, tt := range tests {
		t.Setenv("LOG_LEVEL", tt.env)
		require.Equal(t, tt.want, getLogLevel(), "getLogLevel() for %q", tt.env)
	}
}
