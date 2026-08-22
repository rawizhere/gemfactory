package downloader

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"time"

	ytdlp "github.com/lrstanley/go-ytdlp"
)

var (
	resolveMu   sync.Mutex
	binaryPath  string
	binaryReady bool
)

// EnsureYTDLP resolves the yt-dlp binary from PATH, falling back to a
// go-ytdlp auto-install when missing. Safe to call once at startup.
func EnsureYTDLP(ctx context.Context) error {
	resolveMu.Lock()
	defer resolveMu.Unlock()

	if binaryReady {
		return nil
	}

	if path, err := exec.LookPath("yt-dlp"); err == nil {
		binaryPath = path
		binaryReady = true
		slog.Info("yt-dlp binary resolved from PATH", "path", path)
		return nil
	}

	slog.Info("yt-dlp not found in PATH, attempting auto-install")
	resolved, err := ytdlp.Install(ctx, &ytdlp.InstallOptions{AllowVersionMismatch: true})
	if err != nil {
		return fmt.Errorf("yt-dlp auto-install failed: %w", err)
	}
	binaryPath = resolved.Executable
	binaryReady = true
	slog.Info("yt-dlp binary auto-installed", "path", binaryPath)
	return nil
}

// YTDLPBinary returns the resolved yt-dlp executable path.
func YTDLPBinary() (string, error) {
	resolveMu.Lock()
	defer resolveMu.Unlock()
	if !binaryReady || binaryPath == "" {
		return "", fmt.Errorf("yt-dlp is not available; install yt-dlp or restart the server")
	}
	return binaryPath, nil
}

// StartYTDLPUpdateLoop updates yt-dlp to the nightly channel at startup and
// then once per day until ctx is cancelled.
func StartYTDLPUpdateLoop(ctx context.Context) {
	go func() {
		updateToNightly(ctx)
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				updateToNightly(ctx)
			}
		}
	}()
}

func updateToNightly(parent context.Context) {
	path, err := YTDLPBinary()
	if err != nil {
		slog.Warn("yt-dlp nightly update skipped", "error", err)
		return
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "--update-to", "nightly").CombinedOutput()
	if err != nil {
		slog.Warn("yt-dlp self-update failed", "error", err, "output", strings.TrimSpace(string(out)))
		return
	}
	slog.Info("yt-dlp self-update done", "output", strings.TrimSpace(string(out)))
}
