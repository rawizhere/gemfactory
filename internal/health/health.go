// Package health provides HTTP endpoints for health checks, readiness, and metrics.
package health

import (
	"context"
	"fmt"
	"gemfactory/internal/storage"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

// Server runs an HTTP server exposing /health, /ready, /live, and /metrics endpoints.
type Server struct {
	server *http.Server
	db     *storage.Postgres
	logger *zap.Logger
}

// NewServer initializes the health check HTTP server.
func NewServer(port string, logger *zap.Logger, db *storage.Postgres) *Server {
	mux := http.NewServeMux()

	server := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	healthServer := &Server{
		server: server,
		db:     db,
		logger: logger,
	}

	mux.HandleFunc("/health", healthServer.healthHandler)
	mux.HandleFunc("/ready", healthServer.healthHandler)
	mux.HandleFunc("/live", healthServer.healthHandler)
	mux.Handle("/metrics", promhttp.Handler())

	return healthServer
}

// Start listens and serves incoming HTTP health check requests.
func (s *Server) Start() error {
	s.logger.Info("Starting health check server", zap.String("addr", s.server.Addr))
	return s.server.ListenAndServe()
}

// Stop gracefully shuts down the health check server.
func (s *Server) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	s.logger.Info("Stopping health check server")
	return s.server.Shutdown(ctx)
}

func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	status := "healthy"
	code := http.StatusOK

	if s.db != nil {
		if err := s.db.Ping(); err != nil {
			status = "unhealthy"
			code = http.StatusServiceUnavailable
			s.logger.Error("Health check failed", zap.Error(err))
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = fmt.Fprintf(w, `{"status":"%s","timestamp":"%s"}`, status, time.Now().Format(time.RFC3339))
}
