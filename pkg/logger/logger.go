// Package logger provides a configured zap.Logger with file rotation and dynamic level support.
package logger

import (
	"os"
	"path/filepath"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Logger wraps zap.Logger for runtime level updates.
type Logger struct {
	*zap.Logger
	Level zap.AtomicLevel
}

// NewWithLevel initializes a logger with a configured log level.
func NewWithLevel() (*Logger, error) {
	atomicLevel := zap.NewAtomicLevelAt(getLogLevel())

	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.TimeKey = "timestamp"
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder

	var core zapcore.Core

	consoleCore := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderConfig),
		zapcore.AddSync(os.Stdout),
		atomicLevel,
	)

	logPath := getLogPath()

	// Use lumberjack for log rotation
	lumberjackLogger := &lumberjack.Logger{
		Filename:   logPath,
		MaxSize:    100, // megabytes
		MaxBackups: 3,
		MaxAge:     28, // days
		Compress:   true,
	}

	fileWriter := zapcore.AddSync(lumberjackLogger)

	fileCore := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderConfig),
		fileWriter,
		atomicLevel,
	)

	core = zapcore.NewTee(consoleCore, fileCore)

	zapLogger := zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))

	return &Logger{
		Logger: zapLogger,
		Level:  atomicLevel,
	}, nil
}

// SetLevel updates the logger's level at runtime.
func (l *Logger) SetLevel(levelStr string) {
	var level zapcore.Level
	switch levelStr {
	case "debug":
		level = zapcore.DebugLevel
	case "info":
		level = zapcore.InfoLevel
	case "warn":
		level = zapcore.WarnLevel
	case "error":
		level = zapcore.ErrorLevel
	case "fatal":
		level = zapcore.FatalLevel
	default:
		return
	}
	l.Level.SetLevel(level)
}

// getLogLevel reads the LOG_LEVEL environment variable.
func getLogLevel() zapcore.Level {
	level := os.Getenv("LOG_LEVEL")
	switch level {
	case "debug":
		return zapcore.DebugLevel
	case "info":
		return zapcore.InfoLevel
	case "warn":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	case "fatal":
		return zapcore.FatalLevel
	default:
		return zapcore.InfoLevel
	}
}

// getLogPath determines the log file location.
func getLogPath() string {
	if logPath := os.Getenv("LOG_PATH"); logPath != "" {
		dir := filepath.Dir(logPath)
		if err := os.MkdirAll(dir, 0777); err == nil {
			return logPath
		}
	}

	if dataDir := os.Getenv("APP_DATA_DIR"); dataDir != "" {
		if err := os.MkdirAll(dataDir, 0777); err == nil {
			return filepath.Join(dataDir, "app.log")
		}
	}

	if err := os.MkdirAll("logs", 0777); err == nil {
		return "logs/app.log"
	}

	return "app.log"
}
