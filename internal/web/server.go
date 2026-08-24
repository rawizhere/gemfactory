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
	"gemfactory/internal/service"
)

//go:embed static
var staticFS embed.FS

// Deps wires the server's collaborators; repositories are injected as interfaces.
type Deps struct {
	AppCfg     *config.Config
	Artists    model.ArtistRepository
	Releases   model.ReleaseRepository
	Configs    model.ConfigRepository
	Cookies    model.CookieRepository
	Downloads  *downloader.Service
	ReleaseSvc *service.ReleaseService
}

type Server struct {
	server     *http.Server
	logger     *zap.Logger
	appCfg     *config.Config
	artists    model.ArtistRepository
	releases   model.ReleaseRepository
	configs    model.ConfigRepository
	cookies    model.CookieRepository
	downloads  *downloader.Service
	releaseSvc *service.ReleaseService
}

func NewServer(port string, logger *zap.Logger, deps Deps) *Server {
	s := &Server{
		logger:     logger,
		appCfg:     deps.AppCfg,
		artists:    deps.Artists,
		releases:   deps.Releases,
		configs:    deps.Configs,
		cookies:    deps.Cookies,
		downloads:  deps.Downloads,
		releaseSvc: deps.ReleaseSvc,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.page)
	mux.HandleFunc("POST /api/parse", s.runParser)
	mux.HandleFunc("GET /api/stats", s.getStats)
	mux.HandleFunc("GET /api/artists", s.listArtists)
	mux.HandleFunc("POST /api/artists", s.createArtist)
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
	mux.HandleFunc("POST /api/downloads", s.submitDownload)
	mux.HandleFunc("GET /api/downloads", s.listDownloads)
	mux.HandleFunc("GET /api/downloads/{id}", s.getDownload)
	mux.HandleFunc("GET /api/downloads/{id}/file", s.downloadFile)
	mux.HandleFunc("GET /api/translation", s.getTranslationConfig)
	mux.HandleFunc("POST /api/translation", s.updateTranslationConfig)
	mux.HandleFunc("POST /api/translation/test", s.testTranslation)

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
	artists, err := s.artists.GetAll(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, artists)
}

func (s *Server) createArtist(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name   string `json:"name"`
		Gender string `json:"gender"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	gender, ok := parseGender(w, req.Gender)
	if !ok {
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	existing, err := s.artists.GetByName(r.Context(), name)
	if err != nil {
		s.fail(w, err)
		return
	}
	if existing != nil {
		http.Error(w, "artist already exists", http.StatusConflict)
		return
	}

	artist := &model.Artist{Name: model.NewUniqueString(name), Gender: gender}
	if err := s.artists.Create(r.Context(), artist); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
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

	existing, err := s.artists.GetAll(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	known := make(map[string]bool, len(existing))
	for _, a := range existing {
		known[strings.ToLower(a.Name.String())] = true
	}

	added, skipped := 0, []string{}
	seen := make(map[string]bool)
	for _, raw := range req.Names {
		name := strings.TrimSpace(raw)
		if name == "" || seen[strings.ToLower(name)] {
			continue
		}
		seen[strings.ToLower(name)] = true
		if known[strings.ToLower(name)] {
			skipped = append(skipped, name)
			continue
		}
		artist := &model.Artist{Name: model.NewUniqueString(name), Gender: gender}
		if err := s.artists.Create(r.Context(), artist); err != nil {
			s.fail(w, err)
			return
		}
		known[strings.ToLower(name)] = true
		added++
	}

	writeJSON(w, map[string]any{"added": added, "skipped": skipped})
}

func (s *Server) importArtistsJSON(w http.ResponseWriter, r *http.Request) {
	var req map[string][]string
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body (expected JSON object with gender keys)", http.StatusBadRequest)
		return
	}

	existing, err := s.artists.GetAll(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	known := make(map[string]bool, len(existing))
	for _, a := range existing {
		known[strings.ToLower(a.Name.String())] = true
	}

	added, skipped := 0, []string{}
	seen := make(map[string]bool)

	for rawGender, names := range req {
		var gender model.Gender
		switch strings.ToLower(strings.TrimSpace(rawGender)) {
		case "male", "m":
			gender = model.GenderMale
		case "female", "f":
			gender = model.GenderFemale
		case "mixed", "mix":
			gender = model.GenderMixed
		default:
			continue
		}

		for _, raw := range names {
			name := strings.TrimSpace(raw)
			if name == "" || seen[strings.ToLower(name)] {
				continue
			}
			seen[strings.ToLower(name)] = true
			if known[strings.ToLower(name)] {
				skipped = append(skipped, name)
				continue
			}
			artist := &model.Artist{Name: model.NewUniqueString(name), Gender: gender}
			if err := s.artists.Create(r.Context(), artist); err != nil {
				s.fail(w, err)
				return
			}
			known[strings.ToLower(name)] = true
			added++
		}
	}

	writeJSON(w, map[string]any{"added": added, "skipped": skipped, "total": added + len(skipped)})
}

func (s *Server) runParser(w http.ResponseWriter, r *http.Request) {
	if s.releaseSvc == nil {
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

	var totalFound int
	if month == "" || month == "all" || month == "year" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		count, err := s.releaseSvc.ParseReleasesForYear(ctx, strconv.Itoa(year))
		if err != nil {
			s.logger.Warn("Parser failed for year", zap.Int("year", year), zap.Error(err))
		} else {
			totalFound = count
		}
	} else {
		query := fmt.Sprintf("%s-%d", month, year)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		count, err := s.releaseSvc.ParseReleasesForMonth(ctx, query)
		if err != nil {
			s.logger.Warn("Parser failed for query", zap.String("query", query), zap.Error(err))
		} else {
			totalFound = count
		}
	}

	writeJSON(w, map[string]any{
		"query": fmt.Sprintf("%s %d", req.Month, year),
		"found": totalFound,
		"year":  year,
	})
}

func (s *Server) getStats(w http.ResponseWriter, r *http.Request) {
	artists, err := s.artists.GetAll(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	releases, err := s.releases.GetAll(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	cookies, err := s.cookies.GetAll(r.Context())
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
	if s.downloads != nil {
		if b, files, err := s.downloads.GetStorageUsage(); err == nil {
			storageFormatted = formatBytes(b)
			storageFiles = files
		}
	}

	writeJSON(w, map[string]any{
		"artists":           len(artists),
		"active_artists":    activeArtists,
		"releases":          len(releases),
		"cookies":           len(cookies),
		"storage_formatted": storageFormatted,
		"storage_files":     storageFiles,
	})
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

	artist, err := s.artists.GetByID(r.Context(), id)
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
		artist.Name = model.NewUniqueString(name)
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

	if err := s.artists.Update(r.Context(), artist); err != nil {
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
	if err := s.artists.Delete(r.Context(), id); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listReleases(w http.ResponseWriter, r *http.Request) {
	releases, err := s.releases.GetAll(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	artists, err := s.artists.GetAll(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}

	names := make(map[int]string, len(artists))
	for _, a := range artists {
		names[a.ArtistID] = a.Name.String()
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
			Title:      rel.Title.String(),
			Date:       rel.Date.Format("2006-01-02"),
			TitleTrack: rel.TitleTrack.String(),
			MV:         rel.MV.String(),
			Spotify:    rel.Spotify.String(),
		})
	}
	writeJSON(w, rows)
}

func (s *Server) deleteRelease(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := s.releases.Delete(r.Context(), id); err != nil {
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
	n, err := s.releases.DeleteByIDs(r.Context(), req.IDs)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, map[string]int{"deleted": n})
}

func (s *Server) listConfig(w http.ResponseWriter, r *http.Request) {
	rows, err := s.configs.GetAll(r.Context())
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

	isDedicatedKey := func(key string) bool {
		switch key {
		case "TRANSLATION_FALLBACK_ORDER", "TRANSLATION_PROMPT",
			"SUBS_SOURCE_PREF_RU", "GEMINI_API_KEY", "GEMINI_MODELS",
			"GROQ_API_KEY", "GROQ_MODELS", "OPENCODE_API_KEY", "OPENCODE_MODELS",
			"DOWNLOAD_CONCURRENCY", "CLIP_CRF", "SUBS_CRF", "CLIP_PRESET", "CLIP_AUDIO_BITRATE", "CLIP_DELETE_STATUS",
			"NVIDIA_API_KEY", "NVIDIA_MODELS",
			"DOWNLOAD_RETENTION_HOURS", "TRANSLATION_TIMEOUT", "SUBS_GOOGLE_ONLY",
			"TG_FILE_LIMIT_MB", "YTDLP_PROXY":
			return true
		default:
			return false
		}
	}

	sensitive := func(key string) bool {
		key = strings.ToUpper(key)
		return strings.Contains(key, "TOKEN") ||
			strings.Contains(key, "PASSWORD") ||
			strings.Contains(key, "SECRET")
	}

	// Legacy seed rows and startup-only settings: present in the table but never
	// read back from it. They duplicate env/runtime values shown under System Info.
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
		if isDedicatedKey(c.Key) || isDeadDBKey(c.Key) {
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

	if s.appCfg != nil {
		out = append(out,
			entry{Source: "env", Key: "WEB_PORT", Value: s.appCfg.WebPort, Description: "Web UI HTTP server port"},
			entry{Source: "env", Key: "HEALTH_PORT", Value: s.appCfg.HealthPort, Description: "Health check HTTP server port"},
			entry{Source: "env", Key: "APP_DATA_DIR", Value: s.appCfg.AppDataDir, Description: "Runtime data directory path"},
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

	var req struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if err := s.configs.Set(r.Context(), key, req.Value); err != nil {
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
