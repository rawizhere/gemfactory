package translate

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestLiveGrokSearch calls Gemini for real; run with GEMINI_API_KEY (and optionally GEMINI_MODEL) set.
func TestLiveGrokSearch(t *testing.T) {
	key := os.Getenv("GEMINI_API_KEY")
	if key == "" {
		t.Skip("GEMINI_API_KEY not set")
	}
	model := os.Getenv("GEMINI_MODEL")
	if model == "" {
		model = DefaultModel
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	msg := "Текст для проверки:\n\"IVE выпустили новый альбом в этом месяце\"\n\nВопрос пользователя: какой последний релиз у IVE и когда он вышел?"

	out, err := Complete(ctx, Config{Key: key, Model: model}, DefaultGrokPrompt, msg, 0.3)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("without search: %s", out)
}
