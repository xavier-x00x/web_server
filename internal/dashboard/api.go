package dashboard

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"gopherstack/internal/config"
	"gopherstack/internal/monitor"
	"gopherstack/internal/nginx"
	"gopherstack/internal/pool"
)

// API provides REST API endpoints for the dashboard
type API struct {
	cfg          *config.Config
	poolManagers map[string]*pool.Manager
	nginxManager *nginx.Manager
	monitor      *monitor.Monitor

	// WebSocket subscribers
	wsMu      sync.RWMutex
	wsClients map[chan []byte]struct{}
}

// NewAPI creates a new API handler
func NewAPI(cfg *config.Config, pms map[string]*pool.Manager, nm *nginx.Manager, mon *monitor.Monitor) *API {
	api := &API{
		cfg:          cfg,
		poolManagers: pms,
		nginxManager: nm,
		monitor:      mon,
		wsClients:    make(map[chan []byte]struct{}),
	}

	// Start broadcasting metrics to WebSocket clients
	go api.broadcastLoop()

	return api
}

// StatusResponse represents the system status
type StatusResponse struct {
	Status        string `json:"status"`
	Version       string `json:"version"`
	PHPVersion    string `json:"php_version"`
	Uptime        string `json:"uptime"`
	NginxRunning  bool   `json:"nginx_running"`
	NginxPID      int    `json:"nginx_pid"`
	ActiveWorkers int    `json:"active_workers"`
	TotalWorkers  int    `json:"total_workers"`
	TotalRequests int64  `json:"total_requests"`
}

// HandleStatus returns overall system status
func (a *API) HandleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	metrics := a.monitor.Metrics().Current()
	
	// Aggregate from all pools
	var phpVersion string
	var totalWorkers, activeWorkers int
	for _, pm := range a.poolManagers {
		if phpVersion == "" {
			phpVersion = pm.PHPVersion()
		} else if phpVersion != pm.PHPVersion() && phpVersion != "Multi-Version" {
			phpVersion = "Multi-Version"
		}
		totalWorkers += pm.TotalWorkerCount()
		activeWorkers += pm.ActiveWorkerCount()
	}

	if phpVersion == "" {
		phpVersion = "Unknown"
	}

	resp := StatusResponse{
		Status:        "running",
		Version:       "1.0.0",
		PHPVersion:    phpVersion,
		Uptime:        a.monitor.Metrics().Uptime().Round(time.Second).String(),
		NginxRunning:  a.nginxManager.IsRunning(),
		NginxPID:      a.nginxManager.PID(),
		ActiveWorkers: activeWorkers,
		TotalWorkers:  totalWorkers,
		TotalRequests: metrics.TotalRequests,
	}

	writeJSON(w, resp)
}

type UIWorkerInfo struct {
	pool.WorkerInfo
	Site string `json:"site"`
}

// HandleWorkers returns worker information
func (a *API) HandleWorkers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var allInfos []UIWorkerInfo
	for siteName, pm := range a.poolManagers {
		infos := pm.WorkerInfos()
		for _, info := range infos {
			allInfos = append(allInfos, UIWorkerInfo{
				WorkerInfo: info,
				Site:       siteName,
			})
		}
	}

	writeJSON(w, allInfos)
}

// ScaleRequest represents a scale up/down request
type ScaleRequest struct {
	Action string `json:"action"` // "up" or "down"
	Count  int    `json:"count"`
}

// HandleScale scales workers up or down across all pools (simplified)
func (a *API) HandleScale(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ScaleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Count < 1 {
		http.Error(w, "Count must be at least 1", http.StatusBadRequest)
		return
	}

	var lastErr error
	activeWorkers := 0
	for _, pm := range a.poolManagers {
		switch req.Action {
		case "up":
			lastErr = pm.ScaleUp(req.Count)
		case "down":
			lastErr = pm.ScaleDown(req.Count)
		}
		activeWorkers += pm.ActiveWorkerCount()
	}

	if lastErr != nil {
		http.Error(w, lastErr.Error(), http.StatusInternalServerError)
		return
	}

	// Reload nginx to update upstream
	if a.nginxManager.IsRunning() {
		a.nginxManager.Reload()
	}

	writeJSON(w, map[string]interface{}{
		"success":        true,
		"active_workers": activeWorkers,
	})
}

// HandleRestartWorker restarts a specific worker across all pools
func (a *API) HandleRestartWorker(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	idStr := r.URL.Query().Get("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid worker ID", http.StatusBadRequest)
		return
	}

	for _, pm := range a.poolManagers {
		_ = pm.RestartWorker(id) // Ignore error since the ID might only be valid for some pools
	}

	writeJSON(w, map[string]interface{}{
		"success":   true,
		"worker_id": id,
	})
}

// HandleMetrics returns current metrics
func (a *API) HandleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	writeJSON(w, a.monitor.Metrics().Current())
}

// HandleMetricsHistory returns metrics history
func (a *API) HandleMetricsHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	writeJSON(w, a.monitor.Metrics().History())
}

// HandleReload triggers a configuration reload
func (a *API) HandleReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var errors []string

	if a.nginxManager.IsRunning() {
		if err := a.nginxManager.Reload(); err != nil {
			errors = append(errors, fmt.Sprintf("nginx reload: %v", err))
		}
	}

	writeJSON(w, map[string]interface{}{
		"success": len(errors) == 0,
		"errors":  errors,
	})
}

// UpdatePortRequest represents a request to change the Nginx port
type UpdatePortRequest struct {
	Port int `json:"port"`
}

// HandleUpdateNginxPort changes the Nginx listening port (for the first site for now)
func (a *API) HandleUpdateNginxPort(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req UpdatePortRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Port < 1 || req.Port > 65535 {
		http.Error(w, "Port must be between 1 and 65535", http.StatusBadRequest)
		return
	}

	if req.Port == a.cfg.DashboardPort {
		http.Error(w, "Port conflicts with Dashboard port", http.StatusBadRequest)
		return
	}

	// Update config for the first site (simplified for UI backward compatibility)
	if len(a.cfg.Sites) > 0 {
		a.cfg.Sites[0].NginxPort = req.Port
	}
	
	if err := a.cfg.Save(""); err != nil {
		log.Printf("[api] Failed to save config: %v", err)
	}

	// Restart Nginx to apply new port
	if a.nginxManager.IsRunning() {
		a.nginxManager.Stop()
		if err := a.nginxManager.Start(); err != nil {
			http.Error(w, fmt.Sprintf("Failed to restart Nginx on new port: %v", err), http.StatusInternalServerError)
			return
		}
	}

	writeJSON(w, map[string]interface{}{
		"success": true,
		"port":    req.Port,
	})
}

// HandleConfig returns current configuration
func (a *API) HandleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	writeJSON(w, a.cfg)
}

// PHPConfigRequest represents a request to change PHP settings
type PHPConfigRequest struct {
	EnableOpCache bool `json:"enable_opcache"`
	MaxMemoryMB   int  `json:"max_memory_mb"`
}

// HandleUpdatePHPConfig updates PHP configuration and restarts workers
func (a *API) HandleUpdatePHPConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req PHPConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Update global config
	a.cfg.EnableOpCache = req.EnableOpCache
	if req.MaxMemoryMB >= 16 {
		a.cfg.MaxMemoryMB = req.MaxMemoryMB
	}
	
	if err := a.cfg.Save(""); err != nil {
		log.Printf("[api] Failed to save config: %v", err)
	}

	// Regenerate php.ini for all sites
	for _, site := range a.cfg.Sites {
		phpGen := pool.NewPHPConfigGenerator(a.cfg, site)
		phpIniPath := filepath.Join(a.cfg.ConfigDir, fmt.Sprintf("php_%s.ini", site.Name))
		if err := phpGen.Generate(phpIniPath); err != nil {
			http.Error(w, fmt.Sprintf("Failed to generate php.ini: %v", err), http.StatusInternalServerError)
			return
		}
	}

	// Restart all workers in all pools
	for _, pm := range a.poolManagers {
		if err := pm.RestartAll(); err != nil {
			log.Printf("[api] Warning: Failed to restart some workers: %v", err)
		}
	}

	writeJSON(w, map[string]interface{}{
		"success":        true,
		"enable_opcache": a.cfg.EnableOpCache,
		"max_memory_mb":  a.cfg.MaxMemoryMB,
	})
}


// HandleShutdown stops the PHP and Nginx workers but keeps the orchestrator running
func (a *API) HandleShutdown(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	writeJSON(w, map[string]interface{}{
		"success": true,
		"message": "Stopping PHP and Nginx...",
	})

	go func() {
		time.Sleep(100 * time.Millisecond)
		if a.nginxManager != nil {
			a.nginxManager.Stop()
		}
		if a.poolManagers != nil {
			for _, pm := range a.poolManagers {
				pm.Stop()
			}
		}
	}()
}

// HandleStart starts the PHP and Nginx workers
func (a *API) HandleStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	writeJSON(w, map[string]interface{}{
		"success": true,
		"message": "Starting PHP and Nginx...",
	})

	go func() {
		time.Sleep(100 * time.Millisecond)
		if a.poolManagers != nil {
			for _, pm := range a.poolManagers {
				pm.Start()
			}
		}
		if a.nginxManager != nil {
			a.nginxManager.Start()
		}
	}()
}

// HandleWebSocket handles WebSocket connections for real-time metrics
func (a *API) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Simple SSE (Server-Sent Events) fallback since we want to avoid external deps
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	// Create a channel for this client
	msgChan := make(chan []byte, 10)

	a.wsMu.Lock()
	a.wsClients[msgChan] = struct{}{}
	a.wsMu.Unlock()

	defer func() {
		a.wsMu.Lock()
		delete(a.wsClients, msgChan)
		a.wsMu.Unlock()
	}()

	// Send initial data
	data, _ := json.Marshal(a.monitor.Metrics().Current())
	fmt.Fprintf(w, "data: %s\n\n", data)
	flusher.Flush()

	// Stream updates
	for {
		select {
		case msg := <-msgChan:
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func (a *API) broadcastLoop() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		metrics := a.monitor.Metrics().Current()
		data, err := json.Marshal(metrics)
		if err != nil {
			continue
		}

		a.wsMu.RLock()
		for ch := range a.wsClients {
			select {
			case ch <- data:
			default:
				// Client is slow, skip
			}
		}
		a.wsMu.RUnlock()
	}
}

func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("[api] Failed to write JSON response: %v", err)
	}
}
