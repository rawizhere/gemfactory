package notify

import (
	"context"
	"testing"

	"gemfactory/internal/settings"

	"github.com/stretchr/testify/require"
	"log/slog"
)

func TestAdminNotifierSendWithoutChatID(t *testing.T) {
	n := NewAdminNotifier(settings.New(nil), nil, slog.New(slog.DiscardHandler))
	require.NotPanics(t, func() { n.Send(context.Background(), "test") })
}

func TestAdminNotifierSendNil(t *testing.T) {
	var n *AdminNotifier
	require.NotPanics(t, func() { n.Send(context.Background(), "test") })
}
