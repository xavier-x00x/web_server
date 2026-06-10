package monitor

import (
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopherstack/internal/config"
	"gopherstack/internal/nginx"
	"gopherstack/internal/pool"
)

const maxHistoryPoints = 120 // 10 minutes at 5s intervals

// SystemMetrics holds current system-level metrics
type SystemMetrics struct {
	Timestamp     time.Time `json:"timestamp"`
	GoRoutines    int       `json:"goroutines"`
	MemAllocMB    float64   `json:"mem_alloc_mb"`
	MemSysMB      float64   `json:"mem_sys_mb"`
	NumGC         uint32    `json:"num_gc"`
	TotalRequests int64     `json:"total_requests"`
	ActiveWorkers int       `json:"active_workers"`
	TotalWorkers  int       `json:"total_workers"`
	NginxRunning  bool      `json:"nginx_running"`
	NginxPID      int       `json:"nginx_pid"`
	UptimeSeconds int64     `json:"uptime_seconds"`
}

// MetricsCollector collects and stores metrics
type MetricsCollector struct {
	cfg       *config.Config
	mu        sync.RWMutex
	current   SystemMetrics
	history   []SystemMetrics
	startTime time.Time
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector(cfg *config.Config) *MetricsCollector {
	return &MetricsCollector{
		cfg:       cfg,
		history:   make([]SystemMetrics, 0, maxHistoryPoints),
		startTime: time.Now(),
	}
}

// Collect gathers metrics from all pool managers and the nginx manager
func (mc *MetricsCollector) Collect(pms map[string]*pool.Manager, nm *nginx.Manager) {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// Calculate total requests and workers across all pools
	var totalRequests int64
	var activeWorkers int
	var totalWorkers int

	for _, pm := range pms {
		for _, w := range pm.Workers() {
			totalRequests += w.RequestCount()
		}
		activeWorkers += pm.ActiveWorkerCount()
		totalWorkers += pm.TotalWorkerCount()
	}

	// Try fetching accurate request count from Nginx stub_status using the first site's port
	if nm.IsRunning() && len(mc.cfg.Sites) > 0 {
		nginxReqs, err := mc.fetchNginxRequests(mc.cfg.Sites[0].NginxPort)
		if err == nil && nginxReqs > totalRequests {
			totalRequests = nginxReqs
		}
	}

	metrics := SystemMetrics{
		Timestamp:     time.Now(),
		GoRoutines:    runtime.NumGoroutine(),
		MemAllocMB:    float64(memStats.Alloc) / 1024 / 1024,
		MemSysMB:      float64(memStats.Sys) / 1024 / 1024,
		NumGC:         memStats.NumGC,
		TotalRequests: totalRequests,
		ActiveWorkers: activeWorkers,
		TotalWorkers:  totalWorkers,
		NginxRunning:  nm.IsRunning(),
		NginxPID:      nm.PID(),
		UptimeSeconds: int64(time.Since(mc.startTime).Seconds()),
	}

	mc.mu.Lock()
	mc.current = metrics
	mc.history = append(mc.history, metrics)
	if len(mc.history) > maxHistoryPoints {
		mc.history = mc.history[1:]
	}
	mc.mu.Unlock()
}

// Current returns the latest metrics snapshot
func (mc *MetricsCollector) Current() SystemMetrics {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	return mc.current
}

// History returns the metrics history
func (mc *MetricsCollector) History() []SystemMetrics {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	result := make([]SystemMetrics, len(mc.history))
	copy(result, mc.history)
	return result
}

// Uptime returns the time since the collector was started
func (mc *MetricsCollector) Uptime() time.Duration {
	return time.Since(mc.startTime)
}

func (mc *MetricsCollector) fetchNginxRequests(port int) (int64, error) {
	client := &http.Client{Timeout: 2 * time.Second}
	url := fmt.Sprintf("http://127.0.0.1:%d/nginx_status", port)
	resp, err := client.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return 0, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	lines := strings.Split(string(body), "\n")
	if len(lines) >= 3 {
		// Line 3 looks like: " 12 12 45 "
		parts := strings.Fields(lines[2])
		if len(parts) >= 3 {
			return strconv.ParseInt(parts[2], 10, 64)
		}
	}
	return 0, fmt.Errorf("failed to parse stub_status")
}
