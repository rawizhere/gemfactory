package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

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
		logger:  zap.NewNop(),
		cookies: &stubCookies{domains: map[string]string{}},
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

			if rec.Code != c.want {
				t.Errorf("got %d, want %d (body: %s)", rec.Code, c.want, rec.Body.String())
			}
		})
	}
}

func TestUpsertCookieAutoDetect(t *testing.T) {
	stub := &stubCookies{domains: map[string]string{}}
	s := newTestServer(t)
	s.cookies = stub

	content := "# Netscape HTTP Cookie File\n" +
		".youtube.com\tTRUE\t/\tTRUE\t0\tA\t1\n" +
		".google.com\tTRUE\t/\tTRUE\t0\tB\t2\n" +
		"www.youtube.com\tFALSE\t/\tTRUE\t0\tC\t3\n"

	payload, _ := json.Marshal(map[string]string{"content": content})
	req := httptest.NewRequest("PUT", "/", bytes.NewReader(payload))
	req.SetPathValue("domain", "_")
	rec := httptest.NewRecorder()

	s.upsertCookie(rec, req)

	if rec.Code != 200 {
		t.Fatalf("got %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if _, ok := stub.domains["youtube.com"]; !ok {
		t.Errorf("youtube.com not stored; have %v", keys(stub.domains))
	}
	if _, ok := stub.domains["google.com"]; !ok {
		t.Errorf("google.com not stored; have %v", keys(stub.domains))
	}
	if len(stub.domains) != 2 {
		t.Errorf("expected dedup to 2 domains, got %v", keys(stub.domains))
	}
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
