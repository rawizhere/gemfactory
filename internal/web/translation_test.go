package web

import (
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"gemfactory/internal/translate"
)

func TestRestrictTestToModel(t *testing.T) {
	base := []string{translate.ProviderGoogle, translate.ProviderGemini, translate.ProviderOpencode, translate.ProviderOpenRouter}

	cases := []struct {
		name      string
		tc        translate.Config
		chain     []string
		model     string
		wantChain []string
		wantModel map[string][]string // provider -> expected model list after restriction
	}{
		{
			name:      "empty model keeps full chain",
			tc:        translate.Config{GeminiModels: []string{"a", "b"}, OpencodeModels: []string{"c"}},
			chain:     base,
			model:     "",
			wantChain: base,
			wantModel: map[string][]string{translate.ProviderGemini: {"a", "b"}, translate.ProviderOpencode: {"c"}},
		},
		{
			name:      "malformed model keeps full chain",
			tc:        translate.Config{OpencodeModels: []string{"c"}},
			chain:     base,
			model:     "opencode-no-slash",
			wantChain: base,
			wantModel: map[string][]string{translate.ProviderOpencode: {"c"}},
		},
		{
			name:      "unknown provider keeps full chain",
			tc:        translate.Config{},
			chain:     base,
			model:     "bogus/nope",
			wantChain: base,
		},
		{
			name:      "restricts to a present provider and single model",
			tc:        translate.Config{OpencodeModels: []string{"old"}},
			chain:     base,
			model:     "opencode/nemotron-3.5-lightning-free",
			wantChain: []string{translate.ProviderOpencode},
			wantModel: map[string][]string{translate.ProviderOpencode: {"nemotron-3.5-lightning-free"}},
		},
		{
			name:      "provider missing from chain is still probed",
			tc:        translate.Config{GroqModels: []string{"x"}},
			chain:     []string{translate.ProviderGemini},
			model:     "groq/llama",
			wantChain: []string{translate.ProviderGroq},
			wantModel: map[string][]string{translate.ProviderGroq: {"llama"}},
		},
		{
			name:      "google has no model list but chain narrows",
			tc:        translate.Config{},
			chain:     base,
			model:     "google/web",
			wantChain: []string{translate.ProviderGoogle},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tc := c.tc
			got := restrictTestToModel(&tc, c.chain, c.model)
			if !reflect.DeepEqual(got, c.wantChain) {
				t.Fatalf("chain = %v, want %v", got, c.wantChain)
			}
			for p, want := range c.wantModel {
				var gotModels []string
				switch p {
				case translate.ProviderGemini:
					gotModels = tc.GeminiModels
				case translate.ProviderGroq:
					gotModels = tc.GroqModels
				case translate.ProviderOpencode:
					gotModels = tc.OpencodeModels
				case translate.ProviderNvidia:
					gotModels = tc.NvidiaModels
				case translate.ProviderOpenRouter:
					gotModels = tc.OpenRouterModels
				}
				if !reflect.DeepEqual(gotModels, want) {
					t.Fatalf("provider %s models = %v, want %v", p, gotModels, want)
				}
			}
		})
	}
}

func TestTestTranslation(t *testing.T) {
	s := &Server{logger: zap.NewNop()}

	t.Run("invalid body", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/api/translation/test", strings.NewReader("bad-json"))
		s.testTranslation(rec, req)
		require.Equal(t, 400, rec.Code)
	})

	t.Run("parallel chain stream", func(t *testing.T) {
		body := `{"text":"test","target_lang":"ru","fallback_order":"gemini,groq","gemini_models":"gemini-test","groq_models":"groq-test"}`
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/api/translation/test", strings.NewReader(body))
		s.testTranslation(rec, req)
		require.Equal(t, 200, rec.Code)
		require.Equal(t, "application/x-ndjson", rec.Header().Get("Content-Type"))

		lines := strings.Split(strings.TrimSpace(rec.Body.String()), "\n")
		require.GreaterOrEqual(t, len(lines), 3)

		var initEv struct {
			Type  string   `json:"type"`
			Chain []string `json:"chain"`
			Items []struct {
				Provider string `json:"provider"`
				Model    string `json:"model"`
			} `json:"items"`
		}
		require.NoError(t, json.Unmarshal([]byte(lines[0]), &initEv))
		require.Equal(t, "init", initEv.Type)
		require.Equal(t, []string{"gemini", "groq"}, initEv.Chain)
		require.Len(t, initEv.Items, 2)

		var doneEv struct {
			Type    string `json:"type"`
			Success bool   `json:"success"`
		}
		require.NoError(t, json.Unmarshal([]byte(lines[len(lines)-1]), &doneEv))
		require.Equal(t, "done", doneEv.Type)
		require.False(t, doneEv.Success)
	})
}
