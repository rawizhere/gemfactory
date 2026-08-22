package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var domainRe = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?)+$`)

// listCookies GET /api/cookies — all stored domains with cookie health and count.
func (s *Server) listCookies(w http.ResponseWriter, r *http.Request) {
	cookies, err := s.cookies.GetAll(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}

	type cookieItem struct {
		Domain        string `json:"domain"`
		UpdatedAt     string `json:"updated_at"`
		CookieCount   int    `json:"cookie_count"`
		Health        string `json:"health"`
		ExpiresInDays int    `json:"expires_in_days"`
	}

	now := time.Now().Unix()
	out := make([]cookieItem, 0, len(cookies))

	for _, c := range cookies {
		rec, err := s.cookies.GetByDomain(r.Context(), c.Domain)
		if err != nil || rec == nil {
			out = append(out, cookieItem{
				Domain:    c.Domain,
				UpdatedAt: c.UpdatedAt.Format("2006-01-02 15:04:05"),
				Health:    "unknown",
			})
			continue
		}

		count, minExp, maxExp := parseNetscapeMetadata(rec.Content)
		health := "valid"
		days := 0

		if maxExp > 0 {
			if maxExp < now {
				health = "expired"
				days = int((maxExp - now) / 86400)
			} else if minExp > 0 && minExp < now+7*86400 {
				health = "expiring_soon"
				days = int((minExp - now) / 86400)
			} else {
				health = "valid"
				days = int((maxExp - now) / 86400)
			}
		} else if count > 0 {
			health = "session"
		} else {
			health = "empty"
		}

		out = append(out, cookieItem{
			Domain:        c.Domain,
			UpdatedAt:     c.UpdatedAt.Format("2006-01-02 15:04:05"),
			CookieCount:   count,
			Health:        health,
			ExpiresInDays: days,
		})
	}

	writeJSON(w, out)
}

func parseNetscapeMetadata(content string) (count int, minExp int64, maxExp int64) {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}
		count++
		if len(fields) >= 5 {
			if exp, err := strconv.ParseInt(fields[4], 10, 64); err == nil && exp > 0 {
				if minExp == 0 || exp < minExp {
					minExp = exp
				}
				if exp > maxExp {
					maxExp = exp
				}
			}
		}
	}
	return
}

// getCookie GET /api/cookies/{domain} — full record including contents.
func (s *Server) getCookie(w http.ResponseWriter, r *http.Request) {
	domain := r.PathValue("domain")
	if !validDomain(domain) {
		http.Error(w, "invalid domain", http.StatusBadRequest)
		return
	}
	cookie, err := s.cookies.GetByDomain(r.Context(), domain)
	if err != nil {
		s.fail(w, err)
		return
	}
	if cookie == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	writeJSON(w, cookie)
}

// upsertCookie PUT /api/cookies/{domain} with {"content": "<netscape>"}.
// The domain is optional: pass "_" (or an empty path segment is not routable,
// so use "_") to auto-detect domains from the Netscape export and store the
// content under every domain found.
func (s *Server) upsertCookie(w http.ResponseWriter, r *http.Request) {
	domain := r.PathValue("domain")
	if domain == "_" {
		domain = ""
	}

	var req struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	content := strings.TrimSpace(req.Content)
	if content == "" {
		http.Error(w, "content is required", http.StatusBadRequest)
		return
	}
	if err := validateNetscape(content); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if domain != "" {
		if !validDomain(domain) {
			http.Error(w, "invalid domain", http.StatusBadRequest)
			return
		}
		if err := s.cookies.Upsert(r.Context(), domain, content); err != nil {
			s.fail(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}

	domains := DetectDomains(content)
	if len(domains) == 0 {
		http.Error(w, "could not detect any domain in cookies content", http.StatusBadRequest)
		return
	}
	for _, d := range domains {
		if err := s.cookies.Upsert(r.Context(), d, content); err != nil {
			s.fail(w, err)
			return
		}
	}
	writeJSON(w, map[string][]string{"domains": domains})
}

// validateNetscape accepts Cookie-Editor exports (with the Netscape header)
// and raw tab-separated netscape lines; rejects everything else.
func validateNetscape(content string) error {
	hasHeader := strings.Contains(content, "Netscape HTTP Cookie File")
	hasTabs := strings.Contains(content, "\t")
	if !hasHeader && !hasTabs {
		return errors.New("content must be in Netscape cookies format")
	}
	return nil
}

// DetectDomains extracts unique registrable domains from a Netscape cookie
// file: field 1 of each data line (".youtube.com" -> "youtube.com").
func DetectDomains(content string) []string {
	seen := map[string]bool{}
	var out []string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}
		d := strings.TrimPrefix(strings.ToLower(fields[0]), ".")
		if d == "" {
			continue
		}
		// Keep only the registrable domain: last two labels.
		labels := strings.Split(d, ".")
		if len(labels) > 2 {
			d = strings.Join(labels[len(labels)-2:], ".")
		}
		if !validDomain(d) || seen[d] {
			continue
		}
		seen[d] = true
		out = append(out, d)
	}
	return out
}

// deleteCookie DELETE /api/cookies/{domain}.
func (s *Server) deleteCookie(w http.ResponseWriter, r *http.Request) {
	domain := r.PathValue("domain")
	if !validDomain(domain) {
		http.Error(w, "invalid domain", http.StatusBadRequest)
		return
	}
	n, err := s.cookies.Delete(r.Context(), domain)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, map[string]int{"deleted": n})
}

func validDomain(domain string) bool {
	if len(domain) < 3 || len(domain) > 255 {
		return false
	}
	return domainRe.MatchString(domain)
}
