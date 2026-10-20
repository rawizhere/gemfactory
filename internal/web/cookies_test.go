package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"gemfactory/internal/model"
)

type stubCookies struct {
	domains map[string]string
}

func (s *stubCookies) GetByDomain(ctx context.Context, domain string) (*model.Cookie, error) {
	if c, ok := s.domains[domain]; ok {
		return &model.Cookie{Domain: domain, Content: c}, nil
	}
	return nil, nil
}
func (s *stubCookies) GetAll(ctx context.Context) ([]model.CookieSummary, error) { return nil, nil }
func (s *stubCookies) Upsert(ctx context.Context, domain, content string) error {
	s.domains[domain] = content
	return nil
}
func (s *stubCookies) Delete(ctx context.Context, domain string) (int, error) {
	delete(s.domains, domain)
	return 1, nil
}

func newTestServer(t *testing.T) *Server {
	t.Helper()
	return &Server{
		logger: zap.NewNop(),
		Deps:   Deps{Cookies: &stubCookies{domains: map[string]string{}}},
	}
}

func TestUpsertCookieValidation(t *testing.T) {
	s := newTestServer(t)

	cases := []struct {
		name   string
		domain string
		body   map[string]string
		want   int
	}{
		{"netscape header ok", "youtube.com", map[string]string{"content": "# Netscape HTTP Cookie File\n.youtube.com\tTRUE\t/\tTRUE\t0\tA\tB"}, 204},
		{"tab lines ok", "tiktok.com", map[string]string{"content": ".tiktok.com\tTRUE\t/\tTRUE\t0\tA\tB"}, 204},
		{"empty content", "_", map[string]string{"content": ""}, 400},
		{"garbage format", "_", map[string]string{"content": "just some random text"}, 400},
		{"bad explicit domain", "not a domain!", map[string]string{"content": "a\tb"}, 400},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			payload, _ := json.Marshal(c.body)
			req := httptest.NewRequest("PUT", "/", bytes.NewReader(payload))
			req.SetPathValue("domain", c.domain)
			rec := httptest.NewRecorder()

			s.upsertCookie(rec, req)

			require.Equal(t, c.want, rec.Code, "body: %s", rec.Body.String())
		})
	}
}

func TestUpsertCookieAutoDetect(t *testing.T) {
	stub := &stubCookies{domains: map[string]string{}}
	s := newTestServer(t)
	s.Cookies = stub

	content := "# Netscape HTTP Cookie File\n" +
		".youtube.com\tTRUE\t/\tTRUE\t0\tA\t1\n" +
		".google.com\tTRUE\t/\tTRUE\t0\tB\t2\n" +
		"www.youtube.com\tFALSE\t/\tTRUE\t0\tC\t3\n"

	payload, _ := json.Marshal(map[string]string{"content": content})
	req := httptest.NewRequest("PUT", "/", bytes.NewReader(payload))
	req.SetPathValue("domain", "_")
	rec := httptest.NewRecorder()

	s.upsertCookie(rec, req)

	require.Equal(t, 200, rec.Code, "body: %s", rec.Body.String())
	require.Contains(t, stub.domains, "youtube.com", "have %v", keys(stub.domains))
	require.Contains(t, stub.domains, "google.com", "have %v", keys(stub.domains))
	require.Len(t, stub.domains, 2, "expected dedup to 2 domains, got %v", keys(stub.domains))
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestCookieHealth(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	line := func(exp int64) string { return fmt.Sprintf(".youtube.com\tTRUE\t/\tTRUE\t%d\tSID\tx\n", exp) }
	day := int64(24 * 3600)

	_, health, _ := cookieHealth(line(now.Unix()-day), now)
	require.Equal(t, "expired", health)

	_, health, days := cookieHealth(line(now.Unix()+3*day+60)+line(now.Unix()+90*day), now)
	require.Equal(t, "expiring_soon", health)
	require.Equal(t, 3, days)

	count, health, days := cookieHealth(line(now.Unix()+30*day+60), now)
	require.Equal(t, 1, count)
	require.Equal(t, "valid", health)
	require.Equal(t, 30, days)

	_, health, _ = cookieHealth(line(0), now)
	require.Equal(t, "session", health)
	_, health, _ = cookieHealth("", now)
	require.Equal(t, "empty", health)
}
