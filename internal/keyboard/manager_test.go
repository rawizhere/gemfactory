package keyboard

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"gemfactory/internal/config"
)

func TestKeyboardManager(t *testing.T) {
	cfg := &config.Config{Timezone: "UTC"}
	mgr := NewManager(nil, cfg, zap.NewNop())
	defer mgr.Stop()

	mainKb := mgr.GetMainKeyboard()
	require.NotNil(t, mainKb)
	require.NotEmpty(t, mainKb.InlineKeyboard, "expected non-empty main keyboard")

	allKb := mgr.GetAllMonthsKeyboard()
	require.NotNil(t, allKb)
	require.NotEmpty(t, allKb.InlineKeyboard, "expected non-empty all months keyboard")
}
