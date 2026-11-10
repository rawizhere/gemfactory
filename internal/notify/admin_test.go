package notify

import (
	"context"
	"testing"

	"gemfactory/internal/settings"

	"log/slog"

	"github.com/stretchr/testify/require"
)

func TestAdminNotifierSendWithoutChatID(t *testing.T) {
	n := NewAdminNotifier(settings.New(nil), nil, slog.New(slog.DiscardHandler))
	require.NotPanics(t, func() { n.Send(context.Background(), "test") })
}

func TestAdminNotifierSendNil(t *testing.T) {
	var n *AdminNotifier
	require.NotPanics(t, func() { n.Send(context.Background(), "test") })
}
