package translate

import (
	"context"
	"os"
	"testing"
	"time"

	"google.golang.org/genai"
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

	// Called directly so a silently dropped search tool shows up as missing grounding metadata.
	client, err := newClient(ctx, key, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	gc := baseConfig(DefaultGrokPrompt, 0.3)
	gc.Tools = []*genai.Tool{{GoogleSearch: &genai.GoogleSearch{}}}
	start := time.Now()
	resp, err := client.Models.GenerateContent(ctx, model, genai.Text(msg), gc)
	if err != nil {
		t.Fatalf("%s with search (thinking off): %v", model, err)
	}
	t.Logf("%s, %s\nanswer: %s", model, time.Since(start).Round(time.Millisecond), resp.Text())
	if len(resp.Candidates) == 0 || resp.Candidates[0].GroundingMetadata == nil {
		t.Fatal("no grounding metadata: search was not used")
	}
	gm := resp.Candidates[0].GroundingMetadata
	t.Logf("queries: %v, sources: %d", gm.WebSearchQueries, len(gm.GroundingChunks))

	out, err := Complete(ctx, Config{Key: key, Model: model}, DefaultGrokPrompt, msg, 0.3, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("without search: %s", out)
}
