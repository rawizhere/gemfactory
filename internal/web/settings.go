package web

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"

	"gemfactory/internal/scraper"
	"gemfactory/internal/settings"
)

// applySetting runs the runtime side effect of a changed setting, if it has one.
func (s *Server) applySetting(k settings.Key, v string) {
	switch k.Name {
	case settings.DownloadConcurrency.Name:
		if n, err := strconv.Atoi(v); err == nil && s.Downloads != nil {
			s.Downloads.SetConcurrency(n)
		}
	case settings.ScraperUserAgents.Name:
		if uas := scraper.ParseUserAgents(v); len(uas) > 0 && s.Scraper != nil {
			s.Scraper.SetUserAgents(uas)
		}
	}
}

// GET /api/settings — effective values with source info plus read-only system info.
func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	type entry struct {
		Key     string `json:"key"`
		Value   string `json:"value"`
		Source  string `json:"source"`
		Masked  bool   `json:"masked"`
		Default string `json:"default"`
	}
	out := make([]entry, 0, 24)
	for _, k := range settings.Editable {
		value, source := s.Settings.Resolve(ctx, k)
		if k.Masked {
			value = maskKey(value)
		}
		out = append(out, entry{Key: k.Name, Value: value, Source: source, Masked: k.Masked, Default: k.Def})
	}

	system := []map[string]string{}
	if s.AppCfg != nil {
		system = append(system,
			map[string]string{"key": "WEB_PORT", "value": s.AppCfg.WebPort},
			map[string]string{"key": "LOG_LEVEL", "value": os.Getenv("LOG_LEVEL")},
			map[string]string{"key": "TIMEZONE", "value": s.AppCfg.Timezone},
			map[string]string{"key": "APP_DATA_DIR", "value": s.AppCfg.AppDataDir},
			map[string]string{"key": "RELEASE_CHECK_INTERVAL", "value": s.AppCfg.ReleaseCheckInterval.String()},
			map[string]string{"key": "FFMPEG_BINARY", "value": os.Getenv("FFMPEG_BINARY")},
		)
	}

	writeJSON(w, map[string]any{"settings": out, "system": system})
}

// POST /api/settings — partial update; validates everything before applying anything.
func (s *Server) updateSettings(w http.ResponseWriter, r *http.Request) {
	var req map[string]string
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	type pending struct {
		key settings.Key
		val string
	}
	var updates []pending
	for name, raw := range req {
		k, ok := settings.Lookup(name)
		if !ok {
			http.Error(w, "unknown setting "+name, http.StatusBadRequest)
			return
		}
		if k.Masked && isMaskedKey(raw) {
			continue // echoed-back mask = keep current
		}
		// Empty clears the DB value: env or default applies again (for API keys this removes the key).
		v, err := k.Check(raw)
		if err != nil {
			http.Error(w, fmt.Sprintf("%s: %v", name, err), http.StatusBadRequest)
			return
		}
		updates = append(updates, pending{k, v})
	}

	for _, u := range updates {
		if err := s.Settings.Set(ctx, u.key.Name, u.val); err != nil {
			s.fail(w, err)
			return
		}
	}
	for _, u := range updates {
		// Resolve again so a cleared value applies the env/default it falls back to.
		s.applySetting(u.key, s.Settings.Value(ctx, u.key))
	}

	s.logger.Info("Settings updated", slog.Int("count", len(updates)))
	writeJSON(w, map[string]any{"status": "ok"})
}
