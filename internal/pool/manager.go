package pool

import (
	"fmt"
	"log"
	"os/exec"
	"strings"
	"sync"

	"gopherstack/internal/config"
)

// Manager manages the pool of PHP-CGI workers
type Manager struct {
	cfg      *config.Config
	workers  []*Worker
	balancer *Balancer
	mu       sync.RWMutex
	running  bool
	phpIniPath string
}

// NewManager creates a new pool manager
func NewManager(cfg *config.Config) *Manager {
	return &Manager{
		cfg: cfg,
	}
}

// Start initializes and starts all workers in the pool
func (m *Manager) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return nil
	}

	log.Printf("[pool] Starting %d PHP workers on ports %d-%d",
		m.cfg.WorkerCount, m.cfg.BasePort, m.cfg.BasePort+m.cfg.WorkerCount-1)

	m.workers = make([]*Worker, m.cfg.WorkerCount)

	for i := 0; i < m.cfg.WorkerCount; i++ {
		port := m.cfg.BasePort + i
		worker := NewWorker(i, port, m.cfg.PHPBinaryPath, m.cfg.DocumentRoot)
		if m.phpIniPath != "" {
			worker.SetPHPIni(m.phpIniPath)
		}
		m.workers[i] = worker

		if err := worker.Start(); err != nil {
			log.Printf("[pool] Warning: Failed to start worker %d: %v", i, err)
			continue
		}
		log.Printf("[pool] Worker %d started on port %d (PID: %d)", i, port, worker.PID())
	}

	m.balancer = NewBalancer(m.workers)
	m.running = true

	activeCount := 0
	for _, w := range m.workers {
		if w.IsAlive() {
			activeCount++
		}
	}

	log.Printf("[pool] Pool started with %d workers", activeCount)
	return nil
}

// Stop gracefully stops all workers
func (m *Manager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return nil
	}

	log.Println("[pool] Stopping all workers...")

	var errs []error
	for _, w := range m.workers {
		if err := w.Stop(); err != nil {
			errs = append(errs, fmt.Errorf("worker %d: %w", w.ID, err))
		}
	}

	m.running = false
	log.Println("[pool] All workers stopped")

	if len(errs) > 0 {
		return fmt.Errorf("errors stopping workers: %v", errs)
	}
	return nil
}

// NextWorker returns the next available worker using round-robin
func (m *Manager) NextWorker() *Worker {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.balancer == nil {
		return nil
	}
	return m.balancer.Next()
}

// Workers returns a copy of the worker slice
func (m *Manager) Workers() []*Worker {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*Worker, len(m.workers))
	copy(result, m.workers)
	return result
}

// WorkerInfos returns info for all workers
func (m *Manager) WorkerInfos() []WorkerInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	infos := make([]WorkerInfo, len(m.workers))
	for i, w := range m.workers {
		infos[i] = w.Info()
	}
	return infos
}

// ActiveWorkerCount returns the number of alive workers
func (m *Manager) ActiveWorkerCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for _, w := range m.workers {
		if w.IsAlive() {
			count++
		}
	}
	return count
}

// TotalWorkerCount returns total number of workers
func (m *Manager) TotalWorkerCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.workers)
}

// GetWorkerByPort returns a worker by its listening port
func (m *Manager) GetWorkerByPort(port int) *Worker {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, w := range m.workers {
		if w.Port == port {
			return w
		}
	}
	return nil
}

// RestartWorker restarts a specific worker by ID
func (m *Manager) RestartWorker(id int) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if id < 0 || id >= len(m.workers) {
		return fmt.Errorf("invalid worker ID: %d", id)
	}

	log.Printf("[pool] Restarting worker %d", id)
	return m.workers[id].Restart()
}

// RestartDeadWorkers restarts any workers that have died
func (m *Manager) RestartDeadWorkers() int {
	m.mu.RLock()
	workers := make([]*Worker, len(m.workers))
	copy(workers, m.workers)
	m.mu.RUnlock()

	restarted := 0
	for _, w := range workers {
		if w.Status() == WorkerStatusDead {
			log.Printf("[pool] Worker %d is dead, restarting...", w.ID)
			if err := w.Restart(); err != nil {
				log.Printf("[pool] Failed to restart worker %d: %v", w.ID, err)
			} else {
				log.Printf("[pool] Worker %d restarted successfully (PID: %d)", w.ID, w.PID())
				restarted++
			}
		}
	}
	return restarted
}

// RecycleWorker recycles a worker that has exceeded max requests
func (m *Manager) RecycleWorker(w *Worker) {
	if w.RequestCount() >= int64(m.cfg.MaxRequests) {
		log.Printf("[pool] Worker %d reached %d requests, recycling...", w.ID, w.RequestCount())
		if err := w.Restart(); err != nil {
			log.Printf("[pool] Failed to recycle worker %d: %v", w.ID, err)
		}
	}
}

// ScaleUp adds more workers to the pool
func (m *Manager) ScaleUp(count int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	currentCount := len(m.workers)
	for i := 0; i < count; i++ {
		id := currentCount + i
		port := m.cfg.BasePort + id
		worker := NewWorker(id, port, m.cfg.PHPBinaryPath, m.cfg.DocumentRoot)
		if m.phpIniPath != "" {
			worker.SetPHPIni(m.phpIniPath)
		}

		if err := worker.Start(); err != nil {
			return fmt.Errorf("failed to start new worker %d: %w", id, err)
		}

		m.workers = append(m.workers, worker)
		log.Printf("[pool] Scaled up: worker %d started on port %d", id, port)
	}

	m.balancer = NewBalancer(m.workers)
	m.cfg.WorkerCount = len(m.workers)
	return nil
}

// ScaleDown removes workers from the pool
func (m *Manager) ScaleDown(count int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if count >= len(m.workers) {
		return fmt.Errorf("cannot scale down below 1 worker")
	}

	for i := 0; i < count; i++ {
		idx := len(m.workers) - 1
		worker := m.workers[idx]
		worker.Stop()
		m.workers = m.workers[:idx]
		log.Printf("[pool] Scaled down: worker %d stopped", worker.ID)
	}

	m.balancer = NewBalancer(m.workers)
	m.cfg.WorkerCount = len(m.workers)
	return nil
}

// IsRunning returns whether the pool manager is running
func (m *Manager) IsRunning() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.running
}

// SetPHPIni updates the php.ini path for all workers
func (m *Manager) SetPHPIni(path string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.phpIniPath = path
	for _, w := range m.workers {
		w.SetPHPIni(path)
	}
}

// RestartAll restarts all workers in the pool
func (m *Manager) RestartAll() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, w := range m.workers {
		if err := w.Restart(); err != nil {
			return err
		}
	}
	return nil
}

// PHPVersion returns the PHP version string by executing php-cgi -v
func (m *Manager) PHPVersion() string {
	cmd := exec.Command(m.cfg.PHPBinaryPath, "-v")
	out, err := cmd.Output()
	if err != nil {
		return "Unknown"
	}

	// Example output: PHP 7.4.16 (cgi-fcgi) (built: Mar  2 2021 14:14:13)
	// We just want the first part or the whole first line
	lines := strings.Split(string(out), "\n")
	if len(lines) > 0 {
		return strings.TrimSpace(lines[0])
	}
	return "Unknown"
}


