package web

import (
	"reflect"
	"testing"

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
