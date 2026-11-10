package keyboard

import (
	"testing"

	"log/slog"

	"github.com/stretchr/testify/require"

	"gemfactory/internal/config"
)

func TestKeyboardManager(t *testing.T) {
	cfg := &config.Config{Timezone: "UTC"}
	mgr := NewManager(nil, nil, cfg, slog.New(slog.DiscardHandler))

	mainKb := mgr.GetMainKeyboard()
	require.NotNil(t, mainKb)
	require.NotEmpty(t, mainKb.InlineKeyboard, "expected non-empty main keyboard")

	allKb := mgr.GetAllMonthsKeyboard()
	require.NotNil(t, allKb)
	require.NotEmpty(t, allKb.InlineKeyboard, "expected non-empty all months keyboard")
}
