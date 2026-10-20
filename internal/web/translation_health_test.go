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

	recHealth := httptest.NewRecorder()
	reqHealth := httptest.NewRequest("GET", "/api/translation?health=1", nil)
	s.getTranslationConfig(recHealth, reqHealth)
	require.Equal(t, 200, recHealth.Code)
	var respHealth map[string]any
	require.NoError(t, json.Unmarshal(recHealth.Body.Bytes(), &respHealth))
	require.NotNil(t, respHealth["broken_keys"])
}
