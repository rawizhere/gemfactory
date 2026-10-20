package web

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"gemfactory/internal/config"
	"gemfactory/internal/downloader"
	"gemfactory/internal/model"
	"gemfactory/internal/scraper"
	"gemfactory/internal/service"
	"gemfactory/internal/storage"
)

//go:embed static
var staticFS embed.FS

// Deps wires the server's collaborators; repositories are injected as interfaces.
type Deps struct {
	AppCfg     *config.Config
	DB         *storage.Postgres // pinged by /health; nil in tests
	Artists    model.ArtistRepository
	Releases   model.ReleaseRepository
	Configs    model.ConfigRepository
	Cookies    model.CookieRepository
	Downloads  *downloader.Service
	ReleaseSvc *service.ReleaseService
	Scraper    *scraper.Fetcher
}

type Server struct {
	Deps
	server *http.Server
	logger *zap.Logger
}

func NewServer(port string, logger *zap.Logger, deps Deps) *Server {
	s := &Server{
		Deps:   deps,
		logger: logger,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.page)
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("POST /api/parse", s.runParser)
	mux.HandleFunc("GET /api/stats", s.getStats)
	mux.HandleFunc("GET /api/artists", s.listArtists)
	mux.HandleFunc("POST /api/artists/batch", s.createArtists)
	mux.HandleFunc("POST /api/artists/import-json", s.importArtistsJSON)
	mux.HandleFunc("PATCH /api/artists/{id}", s.updateArtist)
	mux.HandleFunc("DELETE /api/artists/{id}", s.deleteArtist)
	mux.HandleFunc("GET /api/releases", s.listReleases)
	mux.HandleFunc("DELETE /api/releases/{id}", s.deleteRelease)
	mux.HandleFunc("POST /api/releases/delete", s.deleteReleases)
	mux.HandleFunc("GET /api/settings", s.getSettings)
	mux.HandleFunc("POST /api/settings", s.updateSettings)
	mux.HandleFunc("GET /api/config", s.listConfig)
	mux.HandleFunc("PUT /api/config/{key}", s.updateConfig)
	mux.HandleFunc("GET /api/cookies", s.listCookies)
	mux.HandleFunc("GET /api/cookies/{domain}", s.getCookie)
	mux.HandleFunc("PUT /api/cookies/{domain}", s.upsertCookie)
	mux.HandleFunc("DELETE /api/cookies/{domain}", s.deleteCookie)
	mux.HandleFunc("GET /api/downloads/storage", s.getStorageUsage)
	mux.HandleFunc("POST /api/downloads/storage/clean", s.cleanStorage)
	mux.HandleFunc("GET /api/translation", s.getTranslationConfig)
	mux.HandleFunc("POST /api/translation/test", s.testTranslation)
	mux.HandleFunc("GET /api/translation/models", s.getModels)
	mux.HandleFunc("POST /api/translation/models/check", s.checkModels)

	static, _ := fs.Sub(staticFS, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(static))))

	s.server = &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}
	return s
}

func (s *Server) Start() error {
	s.logger.Info("Starting admin web server", zap.String("addr", s.server.Addr))
	return s.server.ListenAndServe()
}

func (s *Server) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.server.Shutdown(ctx)
}

// health answers the container healthcheck: 200 while the database responds, 503 otherwise.
func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if s.DB != nil {
		if err := s.DB.Ping(); err != nil {
			s.logger.Error("Health check failed", zap.Error(err))
			w.WriteHeader(http.StatusServiceUnavailable)
			writeJSON(w, map[string]string{"status": "unhealthy"})
			return
		}
	}
	writeJSON(w, map[string]string{"status": "healthy"})
}

func (s *Server) page(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	data, err := staticFS.ReadFile("static/index.html")
	if err != nil {
		http.Error(w, "page not found", http.StatusInternalServerError)
		return
	}
	_, _ = w.Write(data)
}

func (s *Server) listArtists(w http.ResponseWriter, r *http.Request) {
	artists, err := s.Artists.GetAll(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, artists)
}

func (s *Server) createArtists(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Names  []string `json:"names"`
		Gender string   `json:"gender"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	gender, ok := parseGender(w, req.Gender)
	if !ok {
		return
	}

	added, skipped, err := s.addArtists(r.Context(), map[model.Gender][]string{gender: req.Names})
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, map[string]any{"added": added, "skipped": skipped})
}

func (s *Server) importArtistsJSON(w http.ResponseWriter, r *http.Request) {
	var req map[string][]string
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body (expected JSON object with gender keys)", http.StatusBadRequest)
		return
	}

	byGender := make(map[model.Gender][]string, len(req))
	for rawGender, names := range req {
		switch strings.ToLower(strings.TrimSpace(rawGender)) {
		case "male", "m":
			byGender[model.GenderMale] = append(byGender[model.GenderMale], names...)
		case "female", "f":
			byGender[model.GenderFemale] = append(byGender[model.GenderFemale], names...)
		case "mixed", "mix":
			byGender[model.GenderMixed] = append(byGender[model.GenderMixed], names...)
		}
	}

	added, skipped, err := s.addArtists(r.Context(), byGender)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, map[string]any{"added": added, "skipped": skipped, "total": added + len(skipped)})
}

// addArtists creates the given names, skipping blanks, duplicates and names that already exist (case-insensitive).
func (s *Server) addArtists(ctx context.Context, byGender map[model.Gender][]string) (int, []string, error) {
	existing, err := s.Artists.GetAll(ctx)
	if err != nil {
		return 0, nil, err
	}
	known := make(map[string]bool, len(existing))
	for _, a := range existing {
		known[strings.ToLower(a.Name)] = true
	}

	added, skipped, seen := 0, []string{}, map[string]bool{}
	for gender, names := range byGender {
		for _, raw := range names {
			name := strings.TrimSpace(raw)
			key := strings.ToLower(name)
			if name == "" || seen[key] {
				continue
			}
			seen[key] = true
			if known[key] {
				skipped = append(skipped, name)
				continue
			}
			if err := s.Artists.Create(ctx, &model.Artist{Name: name, Gender: gender}); err != nil {
				return added, skipped, err
			}
			added++
		}
	}
	return added, skipped, nil
}

func (s *Server) runParser(w http.ResponseWriter, r *http.Request) {
	if s.ReleaseSvc == nil {
		http.Error(w, "parser service unavailable", http.StatusServiceUnavailable)
		return
	}
	var req struct {
		Month string `json:"month"`
		Year  int    `json:"year"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	year := req.Year
	if year <= 0 {
		year = time.Now().Year()
	}

	month := strings.ToLower(strings.TrimSpace(req.Month))

	var (
		found int
		err   error
	)
	if month == "" || month == "all" || month == "year" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		found, err = s.ReleaseSvc.ParseReleasesForYear(ctx, strconv.Itoa(year))
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		found, err = s.ReleaseSvc.ParseReleasesForMonth(ctx, fmt.Sprintf("%s-%d", month, year))
	}
	if err != nil {
		s.logger.Warn("Parser failed", zap.String("month", month), zap.Int("year", year), zap.Error(err))
		http.Error(w, fmt.Sprintf("parser failed after saving %d releases: %v", found, err), http.StatusBadGateway)
		return
	}

	writeJSON(w, map[string]any{
		"query": fmt.Sprintf("%s %d", req.Month, year),
		"found": found,
		"year":  year,
	})
}

func (s *Server) getStats(w http.ResponseWriter, r *http.Request) {
	artists, err := s.Artists.GetAll(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	releases, err := s.Releases.GetAll(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	cookies, err := s.Cookies.GetAll(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}

	activeArtists := 0
	for _, a := range artists {
		if a.IsActive {
			activeArtists++
		}
	}

	storageFormatted := "0 B"
	storageFiles := 0
	if s.Downloads != nil {
		if b, files, err := s.Downloads.GetStorageUsage(); err == nil {
			storageFormatted = formatBytes(b)
			storageFiles = files
		}
	}

	writeJSON(w, map[string]any{
		"warnings":          s.warnings(r.Context(), cookies),
		"artists":           len(artists),
		"active_artists":    activeArtists,
		"releases":          len(releases),
		"cookies":           len(cookies),
		"storage_formatted": storageFormatted,
		"storage_files":     storageFiles,
	})
}

// warning is a problem shown in the panel header; tab is where it gets fixed.
type warning struct {
	Text string `json:"text"`
	Tab  string `json:"tab"`
}

// warnings lists cheap-to-detect problems: expiring cookies, a missing Gemini key and a failed scraper run.
// A key that Gemini rejects needs a network call, so the panel asks /api/translation for it separately.
func (s *Server) warnings(ctx context.Context, cookies []model.CookieSummary) []warning {
	out := []warning{}
	now := time.Now()
	for _, c := range cookies {
		rec, err := s.Cookies.GetByDomain(ctx, c.Domain)
		if err != nil || rec == nil {
			continue
		}
		switch _, health, days := cookieHealth(rec.Content, now); health {
		case "expired":
			out = append(out, warning{c.Domain + " cookies expired", "cookies"})
		case "expiring_soon":
			out = append(out, warning{fmt.Sprintf("%s cookies expire in %dd", c.Domain, max(days, 0)), "cookies"})
		}
	}
	if tc := s.translationConfig(ctx); tc.Key == "" && !tc.YouTubeOnly {
		out = append(out, warning{"Gemini API key not set", "settings/translation"})
	}
	if s.ReleaseSvc != nil {
		if p := s.ReleaseSvc.LastParse(); p != nil && p.Err != "" {
			out = append(out, warning{"Last release scrape failed " + p.At.Format("Jan 2 15:04"), "releases"})
		}
	}
	return out
}

func (s *Server) updateArtist(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req struct {
		Name     *string `json:"name"`
		IsActive *bool   `json:"is_active"`
		Gender   *string `json:"gender"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	artist, err := s.Artists.GetByID(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	if artist == nil {
		http.Error(w, "artist not found", http.StatusNotFound)
		return
	}

	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			http.Error(w, "name cannot be empty", http.StatusBadRequest)
			return
		}
		other, err := s.Artists.GetByName(r.Context(), name)
		if err != nil {
			s.fail(w, err)
			return
		}
		if other != nil && other.ArtistID != artist.ArtistID {
			http.Error(w, "artist "+other.Name+" already exists", http.StatusConflict)
			return
		}
		artist.Name = name
	}
	if req.IsActive != nil {
		artist.IsActive = *req.IsActive
	}
	if req.Gender != nil {
		gender, ok := parseGender(w, *req.Gender)
		if !ok {
			return
		}
		artist.Gender = gender
	}

	if err := s.Artists.Update(r.Context(), artist); err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, artist)
}

func (s *Server) deleteArtist(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := s.Artists.Delete(r.Context(), id); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listReleases(w http.ResponseWriter, r *http.Request) {
	releases, err := s.Releases.GetAll(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	artists, err := s.Artists.GetAll(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}

	names := make(map[int]string, len(artists))
	for _, a := range artists {
		names[a.ArtistID] = a.Name
	}

	type row struct {
		ID         int    `json:"id"`
		Artist     string `json:"artist"`
		Title      string `json:"title"`
		Date       string `json:"date"`
		TitleTrack string `json:"title_track"`
		MV         string `json:"mv"`
		Spotify    string `json:"spotify"`
	}

	rows := make([]row, 0, len(releases))
	for _, rel := range releases {
		rows = append(rows, row{
			ID:         rel.ReleaseID,
			Artist:     names[rel.ArtistID],
			Title:      rel.Title,
			Date:       rel.Date.Format("2006-01-02"),
			TitleTrack: rel.TitleTrack,
			MV:         rel.MV,
			Spotify:    rel.Spotify,
		})
	}
	writeJSON(w, rows)
}

func (s *Server) deleteRelease(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := s.Releases.Delete(r.Context(), id); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deleteReleases(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs []int `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if len(req.IDs) == 0 {
		http.Error(w, "ids is required", http.StatusBadRequest)
		return
	}
	n, err := s.Releases.DeleteByIDs(r.Context(), req.IDs)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, map[string]int{"deleted": n})
}

func (s *Server) listConfig(w http.ResponseWriter, r *http.Request) {
	rows, err := s.Configs.GetAll(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}

	type entry struct {
		Source      string `json:"source"`
		Key         string `json:"key"`
		Value       string `json:"value"`
		Description string `json:"description"`
		Editable    bool   `json:"editable"`
	}

	sensitive := func(key string) bool {
		key = strings.ToUpper(key)
		return strings.Contains(key, "TOKEN") ||
			strings.Contains(key, "PASSWORD") ||
			strings.Contains(key, "SECRET")
	}

	// Legacy seed rows never read back; they duplicate values shown under System Info.
	isDeadDBKey := func(key string) bool {
		switch key {
		case "RELEASE_CHECK_INTERVAL", "RATE_LIMIT_REQUESTS", "RATE_LIMIT_WINDOW",
			"SCRAPER_DELAY", "LOG_LEVEL", "TIMEZONE", "APP_DATA_DIR",
			"BOT_TOKEN", "ADMIN_USERNAME", "DB_DSN",
			"WEB_PORT", "WEB_ENABLED", "HEALTH_PORT", "HEALTH_CHECK_ENABLED":
			return true
		default:
			return false
		}
	}

	out := []entry{}
	for _, c := range rows {
		if isRegisteredSetting(c.Key) || isDeadDBKey(c.Key) {
			continue
		}
		value := c.Value
		if sensitive(c.Key) {
			value = "•••"
		}
		out = append(out, entry{
			Source:      "db",
			Key:         c.Key,
			Value:       value,
			Description: c.Description,
			Editable:    !sensitive(c.Key),
		})
	}

	if s.AppCfg != nil {
		out = append(out,
			entry{Source: "env", Key: "WEB_PORT", Value: s.AppCfg.WebPort, Description: "Web UI HTTP server port"},
			entry{Source: "env", Key: "APP_DATA_DIR", Value: s.AppCfg.AppDataDir, Description: "Runtime data directory path"},
		)
	}

	writeJSON(w, out)
}

func (s *Server) updateConfig(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if key == "" {
		http.Error(w, "invalid key", http.StatusBadRequest)
		return
	}
	if strings.Contains(strings.ToUpper(key), "TOKEN") ||
		strings.Contains(strings.ToUpper(key), "PASSWORD") ||
		strings.Contains(strings.ToUpper(key), "SECRET") {
		http.Error(w, "sensitive keys cannot be edited here", http.StatusForbidden)
		return
	}
	if isRegisteredSetting(key) {
		// Registered settings go through /api/settings for validation and runtime apply.
		http.Error(w, key+" is managed in the settings sections", http.StatusBadRequest)
		return
	}

	var req struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if err := s.Configs.Set(r.Context(), key, req.Value); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func parseGender(w http.ResponseWriter, raw string) (model.Gender, bool) {
	gender := model.Gender(strings.TrimSpace(raw))
	switch gender {
	case model.GenderFemale, model.GenderMale, model.GenderMixed:
		return gender, true
	default:
		http.Error(w, "gender must be female, male or mixed", http.StatusBadRequest)
		return "", false
	}
}

func (s *Server) fail(w http.ResponseWriter, err error) {
	if errors.Is(err, context.Canceled) {
		return
	}
	s.logger.Error("web request failed", zap.Error(err))
	http.Error(w, "internal error", http.StatusInternalServerError)
}

func pathID(w http.ResponseWriter, r *http.Request) (int, bool) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id <= 0 {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return 0, false
	}
	return id, true
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	_ = json.NewEncoder(w).Encode(v)
}
