package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"log/slog"

	"github.com/stretchr/testify/require"
)

func TestHealth(t *testing.T) {
	s := &Server{logger: slog.New(slog.DiscardHandler)}
	rec := httptest.NewRecorder()
	s.health(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"status":"healthy"}`, rec.Body.String())
}
