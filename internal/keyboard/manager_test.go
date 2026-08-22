package keyboard

import (
	"testing"

	"go.uber.org/zap"

	"gemfactory/internal/config"
)

func TestKeyboardManager(t *testing.T) {
	cfg := &config.Config{Timezone: "UTC"}
	mgr := NewManager(nil, cfg, zap.NewNop())
	defer mgr.Stop()

	mainKb := mgr.GetMainKeyboard()
	if mainKb == nil || len(mainKb.InlineKeyboard) == 0 {
		t.Error("expected non-empty main keyboard")
	}

	allKb := mgr.GetAllMonthsKeyboard()
	if allKb == nil || len(allKb.InlineKeyboard) == 0 {
		t.Error("expected non-empty all months keyboard")
	}
}
