package web

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"gemfactory/internal/model"
)

type stubConfigs struct {
	values map[string]string
}

func (s *stubConfigs) Get(_ context.Context, key string) (*model.Config, error) {
	if v, ok := s.values[key]; ok {
		return &model.Config{Key: key, Value: v}, nil
	}
	return nil, nil
}

func (s *stubConfigs) GetAll(_ context.Context) ([]model.Config, error) {
	out := make([]model.Config, 0, len(s.values))
	for k, v := range s.values {
		out = append(out, model.Config{Key: k, Value: v})
	}
	return out, nil
}

func (s *stubConfigs) Set(_ context.Context, key, value string) error {
	s.values[key] = value
	return nil
}

func (s *stubConfigs) Reset(context.Context) error { return nil }

func newConfigServer(values map[string]string) (*Server, *stubConfigs) {
	cfg := &stubConfigs{values: values}
	return &Server{logger: zap.NewNop(), Deps: Deps{Configs: cfg}}, cfg
}

func TestListConfigHidesRegisteredSettings(t *testing.T) {
	s, _ := newConfigServer(map[string]string{
		"OPENROUTER_API_KEY":  "sk-or-v1-secret",
		"SCRAPER_USER_AGENTS": "UA",
		"CUSTOM_KEY":          "x",
	})

	rec := httptest.NewRecorder()
	s.listConfig(rec, httptest.NewRequest("GET", "/api/config", nil))
	require.Equal(t, 200, rec.Code)

	var rows []struct{ Key string }
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &rows))
	var keys []string
	for _, r := range rows {
		keys = append(keys, r.Key)
	}
	require.Equal(t, []string{"CUSTOM_KEY"}, keys)
}

func TestUpdateConfigRejectsRegisteredSettings(t *testing.T) {
	s, cfg := newConfigServer(map[string]string{})

	req := httptest.NewRequest("PUT", "/api/config/SCRAPER_USER_AGENTS", strings.NewReader(`{"value":""}`))
	req.SetPathValue("key", "SCRAPER_USER_AGENTS")
	rec := httptest.NewRecorder()
	s.updateConfig(rec, req)

	require.Equal(t, 400, rec.Code)
	require.Empty(t, cfg.values)
}
