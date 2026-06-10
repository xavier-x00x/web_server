package monitor

import (
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"gopherstack/internal/config"
	"gopherstack/internal/nginx"
	"gopherstack/internal/pool"
)

// Monitor performs periodic health checks and auto-healing
type Monitor struct {
	cfg          *config.Config
	poolManagers map[string]*pool.Manager
	nginxManager *nginx.Manager
	metrics      *MetricsCollector
	proxyLn      net.Listener

	mu               sync.RWMutex
	running          bool
	stopChan         chan struct{}
	zombieCheckCount int // counter for periodic zombie cleanup
}

// NewMonitor creates a new health monitor
func NewMonitor(cfg *config.Config, pms map[string]*pool.Manager, nm *nginx.Manager) *Monitor {
	return &Monitor{
		cfg:          cfg,
		poolManagers: pms,
		nginxManager: nm,
		metrics:      NewMetricsCollector(cfg),
		stopChan:     make(chan struct{}),
	}
}

// Start begins periodic health monitoring
func (m *Monitor) Start() {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return
	}
	m.running = true
	m.mu.Unlock()

	interval := time.Duration(m.cfg.HealthCheckInterval) * time.Second
	log.Printf("[monitor] Starting health checks every %v", interval)

	// Start TCP Stats Proxy (Nginx -> Go Proxy -> PHP Workers)
	go m.startStatsProxy()

	go m.runLoop(interval)
}

// Stop halts the monitor
func (m *Monitor) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return
	}

	close(m.stopChan)
	if m.proxyLn != nil {
		m.proxyLn.Close()
	}
	m.running = false
	log.Println("[monitor] Stopped")
}

// Metrics returns the metrics collector
func (m *Monitor) Metrics() *MetricsCollector {
	return m.metrics
}

func (m *Monitor) runLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopChan:
			return
		case <-ticker.C:
			m.check()
		}
	}
}

func (m *Monitor) check() {
	// 1. Check and restart dead PHP workers across all pools
	totalRestarted := 0
	for _, pm := range m.poolManagers {
		restarted := pm.RestartDeadWorkers()
		totalRestarted += restarted
	}

	if totalRestarted > 0 {
		log.Printf("[monitor] Auto-healed %d dead workers across all sites", totalRestarted)
	}

	// 2. Check Nginx status
	if !m.nginxManager.IsRunning() {
		log.Println("[monitor] Nginx is not running, attempting restart...")
		if err := m.nginxManager.Start(); err != nil {
			log.Printf("[monitor] Failed to restart Nginx: %v", err)
		} else {
			log.Println("[monitor] Nginx restarted successfully")
		}
	}

	// 3. Collect metrics
	m.metrics.Collect(m.poolManagers, m.nginxManager)
}

func (m *Monitor) startStatsProxy() {
	addr := "127.0.0.1:9000"
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Printf("[monitor] TCP Proxy Listen error on %s: %v", addr, err)
		return
	}
	m.proxyLn = ln
	log.Printf("[monitor] TCP Stats Proxy listening on %s (Nginx Gateway)", addr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-m.stopChan:
				return
			default:
				log.Printf("[monitor] TCP Proxy Accept error: %v", err)
				continue
			}
		}

		go m.handleProxyConn(conn)
	}
}

func (m *Monitor) handleProxyConn(in net.Conn) {
	defer in.Close()

	// 1. Pick a worker from the first available pool manager
	var pm *pool.Manager
	for _, p := range m.poolManagers {
		pm = p
		break
	}

	if pm == nil {
		log.Printf("[monitor] Proxy Error: No pool managers available")
		return
	}

	worker := pm.NextWorker()
	if worker == nil {
		log.Printf("[monitor] Proxy Error: No workers available")
		return
	}

	// 2. Increment stats
	worker.IncrementRequests()

	// 3. Connect to worker
	out, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", worker.Port), 2*time.Second)
	if err != nil {
		log.Printf("[monitor] Proxy failed to connect to worker %d: %v", worker.ID, err)
		return
	}
	defer out.Close()

	// 4. Relay traffic bi-directionally
	done := make(chan struct{})
	go func() {
		io.Copy(out, in)
		if tcp, ok := out.(*net.TCPConn); ok {
			tcp.CloseWrite()
		}
		done <- struct{}{}
	}()

	io.Copy(in, out)
	if tcp, ok := in.(*net.TCPConn); ok {
		tcp.CloseWrite()
	}
	<-done
}
