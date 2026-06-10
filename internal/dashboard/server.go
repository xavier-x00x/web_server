package dashboard

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"time"

	"gopherstack/internal/config"
	"gopherstack/internal/monitor"
	"gopherstack/internal/nginx"
	"gopherstack/internal/pool"
)

//go:embed static
var staticFiles embed.FS

// Server serves the admin dashboard
type Server struct {
	cfg          *config.Config
	poolManagers map[string]*pool.Manager
	nginxManager *nginx.Manager
	monitor      *monitor.Monitor
	httpServer   *http.Server
}

// NewServer creates a new dashboard server
func NewServer(cfg *config.Config, pms map[string]*pool.Manager, nm *nginx.Manager, mon *monitor.Monitor) *Server {
	return &Server{
		cfg:          cfg,
		poolManagers: pms,
		nginxManager: nm,
		monitor:      mon,
	}
}

// Start starts the dashboard HTTP server
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// API endpoints
	api := NewAPI(s.cfg, s.poolManagers, s.nginxManager, s.monitor)
	mux.HandleFunc("/api/status", api.HandleStatus)
	mux.HandleFunc("/api/workers", api.HandleWorkers)
	mux.HandleFunc("/api/workers/scale", api.HandleScale)
	mux.HandleFunc("/api/workers/restart", api.HandleRestartWorker)
	mux.HandleFunc("/api/metrics", api.HandleMetrics)
	mux.HandleFunc("/api/metrics/history", api.HandleMetricsHistory)
	mux.HandleFunc("/api/reload", api.HandleReload)
	mux.HandleFunc("/api/config", api.HandleConfig)
	mux.HandleFunc("/api/settings/nginx_port", api.HandleUpdateNginxPort)
	mux.HandleFunc("/api/settings/php_config", api.HandleUpdatePHPConfig)
	mux.HandleFunc("/api/shutdown", api.HandleShutdown)
	mux.HandleFunc("/api/start", api.HandleStart)

	// WebSocket for real-time metrics
	mux.HandleFunc("/ws/metrics", api.HandleWebSocket)

	// Static files for the dashboard UI
	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		return fmt.Errorf("failed to setup static files: %w", err)
	}
	mux.Handle("/", http.FileServer(http.FS(staticFS)))

	s.httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.cfg.DashboardPort),
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	log.Printf("[dashboard] Listening on http://localhost:%d", s.cfg.DashboardPort)
	return s.httpServer.ListenAndServe()
}

// Stop gracefully shuts down the dashboard server
func (s *Server) Stop() {
	if s.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.httpServer.Shutdown(ctx)
	}
}
