package web

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestTestTranslation(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "")
	s := &Server{logger: zap.NewNop()}

	t.Run("invalid body", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/api/translation/test", strings.NewReader("bad-json"))
		s.testTranslation(rec, req)
		require.Equal(t, 400, rec.Code)
	})

	t.Run("missing key reports an error", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/api/translation/test", strings.NewReader(`{"text":"test","model":"gemini-test"}`))
		s.testTranslation(rec, req)
		require.Equal(t, 200, rec.Code)

		var resp struct {
			OK    bool   `json:"ok"`
			Model string `json:"model"`
			Error string `json:"error"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		require.False(t, resp.OK)
		require.Equal(t, "gemini-test", resp.Model)
		require.Contains(t, resp.Error, "GEMINI_API_KEY")
	})
}
