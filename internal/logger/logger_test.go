package logger

import (
	"testing"

	"go.uber.org/zap/zapcore"
)

func TestNewLogger(t *testing.T) {
	t.Setenv("LOG_LEVEL", "debug")
	l, err := New()
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	if l.Level.Level() != zapcore.DebugLevel {
		t.Errorf("want DebugLevel, got %v", l.Level.Level())
	}

	l.SetLevel("warn")
	if l.Level.Level() != zapcore.WarnLevel {
		t.Errorf("want WarnLevel, got %v", l.Level.Level())
	}

	l.SetLevel("invalid_level")
	if l.Level.Level() != zapcore.WarnLevel {
		t.Errorf("expected level to remain unchanged on invalid input, got %v", l.Level.Level())
	}
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
		if got := getLogLevel(); got != tt.want {
			t.Errorf("getLogLevel() for %q = %v, want %v", tt.env, got, tt.want)
		}
	}
}
