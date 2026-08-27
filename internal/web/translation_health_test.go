package web

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestGetTranslationConfigBrokenKeys(t *testing.T) {
	s := &Server{logger: zap.NewNop()}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/translation", nil)
	s.getTranslationConfig(rec, req)
	require.Equal(t, 200, rec.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.NotContains(t, resp, "broken_keys")

	recHealth := httptest.NewRecorder()
	reqHealth := httptest.NewRequest("GET", "/api/translation?health=1", nil)
	s.getTranslationConfig(recHealth, reqHealth)
	require.Equal(t, 200, recHealth.Code)
	var respHealth map[string]any
	require.NoError(t, json.Unmarshal(recHealth.Body.Bytes(), &respHealth))
	_, ok := respHealth["broken_keys"]
	require.True(t, ok)
}
