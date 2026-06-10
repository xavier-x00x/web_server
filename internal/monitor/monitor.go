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
	poolManager  *pool.Manager
	nginxManager *nginx.Manager
	metrics      *MetricsCollector
	proxyLn      net.Listener

	mu               sync.RWMutex
	running          bool
	stopChan         chan struct{}
	zombieCheckCount int // counter for periodic zombie cleanup
}

// NewMonitor creates a new health monitor
func NewMonitor(cfg *config.Config, pm *pool.Manager, nm *nginx.Manager) *Monitor {
	return &Monitor{
		cfg:          cfg,
		poolManager:  pm,
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
	// 1. Check and restart dead PHP workers
	restarted := m.poolManager.RestartDeadWorkers()
	if restarted > 0 {
		log.Printf("[monitor] Auto-healed %d dead workers", restarted)
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
	m.metrics.Collect(m.poolManager, m.nginxManager)

	// (Zombie cleanup is now only performed during initial startup in pool manager)
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

	// 1. Pick a worker
	worker := m.poolManager.NextWorker()
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
