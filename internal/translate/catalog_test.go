package translate

import (
	"testing"
)

func TestOpenRouterIsFree(t *testing.T) {
	cases := []struct {
		name string
		m    openRouterModel
		want bool
	}{
		{"both zero", openRouterModel{Pricing: struct {
			Prompt     string `json:"prompt"`
			Completion string `json:"completion"`
		}{Prompt: "0", Completion: "0"}}, true},
		{"prompt nonzero", openRouterModel{Pricing: struct {
			Prompt     string `json:"prompt"`
			Completion string `json:"completion"`
		}{Prompt: "0.0001", Completion: "0"}}, false},
		{"completion nonzero", openRouterModel{Pricing: struct {
			Prompt     string `json:"prompt"`
			Completion string `json:"completion"`
		}{Prompt: "0", Completion: "0.0002"}}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := OpenRouterIsFree(c.m); got != c.want {
				t.Errorf("OpenRouterIsFree() = %v, want %v", got, c.want)
			}
		})
	}
}

func TestOpencodeIsFree(t *testing.T) {
	cases := []struct {
		id   string
		want bool
	}{
		{"x-preview-f-free", true},
		{"nemotron-3-ultra-free", true},
		{"claude-fable-5", false},
		{"minimax-m3", false},
	}
	for _, c := range cases {
		if got := OpencodeIsFree(c.id); got != c.want {
			t.Errorf("OpencodeIsFree(%q) = %v, want %v", c.id, got, c.want)
		}
	}
}

func TestParseOpenRouterModels(t *testing.T) {
	body := `{"data":[
		{"id":"a/free-model:free","context_length":1000,"reasoning":{"default_enabled":true},"pricing":{"prompt":"0","completion":"0"}},
		{"id":"a/paid","context_length":2000,"reasoning":{"default_enabled":false},"pricing":{"prompt":"0.1","completion":"0"}},
		{"id":"b/free-noreason","context_length":3000,"pricing":{"prompt":"0","completion":"0"}}
	]}`
	models, err := ParseOpenRouterModels([]byte(body))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(models) != 3 {
		t.Fatalf("expected 3 models, got %d", len(models))
	}
	if !models[0].Free || models[0].Reasoning != true || models[0].ContextLength != 1000 {
		t.Errorf("model0 wrong: %+v", models[0])
	}
	if models[1].Free {
		t.Errorf("model1 should not be free: %+v", models[1])
	}
	if !models[2].Free || models[2].Reasoning {
		t.Errorf("model2 wrong: %+v", models[2])
	}
}

func TestParseOpenAIModelIDs(t *testing.T) {
	body := `{"object":"list","data":[{"id":"llama-3.1-8b","object":"model"},{"id":"mixtral-8x7b"},{"id":""}]}`
	ids, err := ParseOpenAIModelIDs([]byte(body))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 ids, got %d (%v)", len(ids), ids)
	}
	if ids[0] != "llama-3.1-8b" || ids[1] != "mixtral-8x7b" {
		t.Errorf("unexpected ids: %v", ids)
	}
}

func TestBuildCatalogFreeFilter(t *testing.T) {
	ids := []string{"x-free", "y", "z-free"}
	got := buildCatalog(ProviderOpencode, ids, OpencodeIsFree)
	if len(got) != 3 {
		t.Fatalf("expected 3, got %d", len(got))
	}
	if !got[0].Free || got[1].Free || !got[2].Free {
		t.Errorf("free flags wrong: %+v", got)
	}
	if got[0].Provider != ProviderOpencode {
		t.Errorf("provider wrong: %s", got[0].Provider)
	}
}

func TestParseOpenAIModelIDsInvalid(t *testing.T) {
	if _, err := ParseOpenAIModelIDs([]byte("not json")); err == nil {
		t.Error("expected error on invalid json")
	}
}

func TestClassifyOpenAIStatus(t *testing.T) {
	cases := []struct {
		code int
		body string
		want string
	}{
		{200, "", StatusOK},
		{400, "", StatusDead},
		{404, "", StatusDead},
		{429, "rate limit", StatusRateLimited},
		{429, "out of credits", StatusQuota},
		{403, "", StatusNotChat},
		{502, "", StatusNotChat},
		{500, "boom", StatusError},
	}
	for _, c := range cases {
		if got := classifyOpenAIStatus(c.code, c.body); got != c.want {
			t.Errorf("classifyOpenAIStatus(%d,%q) = %s, want %s", c.code, c.body, got, c.want)
		}
	}
}

func TestCatalogCacheTTL(t *testing.T) {
	// populate via store + cached within TTL
	storeCatalog(ProviderOpenRouter, []CatalogModel{{Provider: ProviderOpenRouter, ID: "m1"}})
	if _, ok := cachedCatalog(ProviderOpenRouter); !ok {
		t.Fatal("expected cached entry")
	}
	storeStatus(ProviderOpenRouter, "m1", StatusOK)
	if st, ok := cachedStatus(ProviderOpenRouter, "m1"); !ok || st != StatusOK {
		t.Fatalf("expected cached status ok, got %s ok=%v", st, ok)
	}
}

func TestIsSupportedProvider(t *testing.T) {
	for _, p := range []string{ProviderOpenRouter, ProviderOpencode, ProviderNvidia, ProviderGroq, ProviderGemini} {
		if !isSupportedProvider(p) {
			t.Errorf("%s should be supported", p)
		}
	}
	if isSupportedProvider("bogus") {
		t.Error("bogus should not be supported")
	}
}
