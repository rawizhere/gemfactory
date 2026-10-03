package scraper

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestHTTPClientFailsOverOn403(t *testing.T) {
	var mu sync.Mutex
	calls := make(map[string]int)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls[r.Header.Get("User-Agent")]++
		mu.Unlock()
		if r.Header.Get("User-Agent") == "ua-bad" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := NewHTTPClient([]string{"ua-bad", "ua-good"}, zap.NewNop())
	resp, err := c.doRequest(context.Background(), srv.URL)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()

	resp, err = c.doRequest(context.Background(), srv.URL)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, 1, calls["ua-bad"], "blocked agent must not be retried within cooldown")
	require.GreaterOrEqual(t, calls["ua-good"], 1)
}

func TestHTTPClientSetUserAgents(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := NewHTTPClient([]string{"ua-old"}, zap.NewNop())
	c.SetUserAgents([]string{"ua-new", "ua-two"})
	resp, err := c.doRequest(context.Background(), srv.URL)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()
	require.Equal(t, "ua-new", resp.Request.Header.Get("User-Agent"))
}
