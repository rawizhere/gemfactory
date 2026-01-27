// Package health provides an HTTP server for monitoring the bot's status and metrics.
package health

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Server: Health check HTTP server.
type Server struct {
	server *http.Server
	db     DatabaseInterface
	logger *zap.Logger
}

// NewServer: Initializes the health check server.
func NewServer(port string, logger *zap.Logger, db DatabaseInterface) *Server {
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

	// Register routes
	mux.HandleFunc("/health", healthServer.healthHandler)
	mux.HandleFunc("/ready", healthServer.readyHandler)
	mux.HandleFunc("/live", healthServer.liveHandler)
	mux.Handle("/metrics", promhttp.Handler())

	return healthServer
}

// Start: Runs the health check server.
func (s *Server) Start() error {
	s.logger.Info("Starting health check server", zap.String("addr", s.server.Addr))
	return s.server.ListenAndServe()
}

// Stop: Gracefully shuts down the health check server.
func (s *Server) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	s.logger.Info("Stopping health check server")
	return s.server.Shutdown(ctx)
}

// healthHandler: Reports the general system health based on database and component checks.
func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	status := "healthy"
	code := http.StatusOK

	if err := s.checkDatabase(); err != nil {
		status = "unhealthy"
		code = http.StatusServiceUnavailable
		s.logger.Error("Health check failed", zap.Error(err))
	}

	if err := s.checkComponents(); err != nil {
		status = "unhealthy"
		code = http.StatusServiceUnavailable
		s.logger.Error("Component check failed", zap.Error(err))
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if _, err := fmt.Fprintf(w, `{"status":"%s","timestamp":"%s"}`, status, time.Now().Format(time.RFC3339)); err != nil {
		s.logger.Error("Failed to write response", zap.Error(err))
	}
}

// readyHandler: Reports the readiness status.
func (s *Server) readyHandler(w http.ResponseWriter, r *http.Request) {
	status := "ready"
	code := http.StatusOK

	if err := s.checkReadiness(); err != nil {
		status = "not ready"
		code = http.StatusServiceUnavailable
		s.logger.Error("Readiness check failed", zap.Error(err))
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if _, err := fmt.Fprintf(w, `{"status":"%s","timestamp":"%s"}`, status, time.Now().Format(time.RFC3339)); err != nil {
		s.logger.Error("Failed to write response", zap.Error(err))
	}
}

// liveHandler: Reports the liveness status.
func (s *Server) liveHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if _, err := fmt.Fprintf(w, `{"status":"alive","timestamp":"%s"}`, time.Now().Format(time.RFC3339)); err != nil {
		s.logger.Error("Failed to write response", zap.Error(err))
	}
}

// checkDatabase: Verifies the database connection remains active.
func (s *Server) checkDatabase() error {
	if s.db == nil {
		return fmt.Errorf("database connection is nil")
	}

	if err := s.db.Ping(); err != nil {
		return fmt.Errorf("database ping failed: %w", err)
	}

	return nil
}

// checkComponents: Placeholder for future checks.
func (s *Server) checkComponents() error {
	if s.server == nil {
		return fmt.Errorf("health check server is not initialized")
	}
	return nil
}

// checkReadiness: Verifies dependencies required to serve traffic.
func (s *Server) checkReadiness() error {
	if s.db == nil {
		return fmt.Errorf("database is not initialized")
	}

	if err := s.checkDatabase(); err != nil {
		return fmt.Errorf("database is not ready: %w", err)
	}

	return nil
}
