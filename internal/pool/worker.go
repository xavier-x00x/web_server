package pool

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// WorkerStatus represents the current state of a PHP worker
type WorkerStatus int

const (
	WorkerStatusStopped WorkerStatus = iota
	WorkerStatusStarting
	WorkerStatusRunning
	WorkerStatusDead
)

func (s WorkerStatus) String() string {
	switch s {
	case WorkerStatusStopped:
		return "stopped"
	case WorkerStatusStarting:
		return "starting"
	case WorkerStatusRunning:
		return "running"
	case WorkerStatusDead:
		return "dead"
	default:
		return "unknown"
	}
}

// Worker represents a single PHP-CGI process
type Worker struct {
	ID           int
	Port         int
	PHPBinPath   string
	DocumentRoot string
	PHPIniPath   string

	mu           sync.RWMutex
	cmd          *exec.Cmd
	cancel       context.CancelFunc // for graceful shutdown
	status       WorkerStatus
	pid          int
	requestCount atomic.Int64
	startTime    time.Time
	restartCount int
}

// NewWorker creates a new PHP-CGI worker
func NewWorker(id, port int, phpBinPath, documentRoot string) *Worker {
	return &Worker{
		ID:           id,
		Port:         port,
		PHPBinPath:   phpBinPath,
		DocumentRoot: documentRoot,
		PHPIniPath:   filepath.Join(filepath.Dir(phpBinPath), "..", "..", "config", "php.ini"), // Default fallback
		status:       WorkerStatusStopped,
	}
}

// SetPHPIni sets a custom php.ini path
func (w *Worker) SetPHPIni(path string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.PHPIniPath = path
}

// Start launches the php-cgi.exe process bound to the worker's port
func (w *Worker) Start() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.status == WorkerStatusRunning {
		return nil
	}

	w.status = WorkerStatusStarting

	bindAddr := fmt.Sprintf("127.0.0.1:%d", w.Port)

	args := []string{"-b", bindAddr}
	if w.PHPIniPath != "" {
		args = append(args, "-c", w.PHPIniPath)
	}

	// Context for lifecycle management — cancel triggers process kill
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel

	w.cmd = exec.CommandContext(ctx, w.PHPBinPath, args...)
	w.cmd.Env = append(os.Environ(),
		fmt.Sprintf("PHP_FCGI_MAX_REQUESTS=%d", 0), // We manage recycling ourselves
		"PHP_FCGI_CHILDREN=0",                      // No child processes, we manage the pool
	)
	w.cmd.Dir = w.DocumentRoot
	w.cmd.Stdout = nil
	w.cmd.Stderr = nil

	// Set process group so we can kill the entire group
	w.cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}

	if err := w.cmd.Start(); err != nil {
		cancel() // clean up context on failure
		w.status = WorkerStatusDead
		return fmt.Errorf("failed to start PHP-CGI worker %d on port %d: %w", w.ID, w.Port, err)
	}

	w.pid = w.cmd.Process.Pid
	w.startTime = time.Now()
	w.status = WorkerStatusRunning

	// Monitor the process in background
	go w.monitor()

	return nil
}

// Stop gracefully stops the PHP-CGI process with two-phase shutdown:
//  1. Cancel context (sends kill via exec.CommandContext)
//  2. Wait up to 5s for graceful exit
//  3. Force TerminateProcess if still alive
func (w *Worker) Stop() error {
	return w.StopContext(context.Background())
}

// StopContext stops the worker with an optional context for timeout control
func (w *Worker) StopContext(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.cmd == nil || w.cmd.Process == nil {
		w.status = WorkerStatusStopped
		return nil
	}

	// Phase 1: Cancel context — triggers kill via exec.CommandContext
	if w.cancel != nil {
		w.cancel()
	}

	// Phase 2: Wait with timeout for process to exit
	done := make(chan struct{})
	go func() {
		w.cmd.Wait()
		close(done)
	}()

	// Use caller's context if it has a deadline, otherwise fallback to 5s
	timeout := 5 * time.Second
	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		// Let the caller's context drive the timeout
		select {
		case <-done:
		case <-ctx.Done():
			_ = w.cmd.Process.Kill()
			<-done
		}
	} else {
		select {
		case <-done:
		case <-time.After(timeout):
			// Phase 3: Force kill if still alive
			_ = w.cmd.Process.Kill()
			<-done // wait for final exit
		}
	}

	w.status = WorkerStatusStopped
	w.cmd = nil
	w.cancel = nil
	w.pid = 0

	return nil
}

// Restart stops and starts the worker
func (w *Worker) Restart() error {
	return w.RestartContext(context.Background())
}

// RestartContext stops and starts the worker with a context timeout
func (w *Worker) RestartContext(ctx context.Context) error {
	if err := w.StopContext(ctx); err != nil {
		return err
	}

	// Small delay to allow port to be released
	time.Sleep(100 * time.Millisecond)

	w.mu.Lock()
	w.restartCount++
	w.requestCount.Store(0)
	w.mu.Unlock()

	return w.Start()
}

// IsAlive checks if the worker process is still running
func (w *Worker) IsAlive() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.status == WorkerStatusRunning
}

// Status returns the current worker status
func (w *Worker) Status() WorkerStatus {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.status
}

// IncrementRequests increments the request counter
func (w *Worker) IncrementRequests() {
	w.requestCount.Add(1)
}

// RequestCount returns the total number of requests served
func (w *Worker) RequestCount() int64 {
	return w.requestCount.Load()
}

// PID returns the process ID
func (w *Worker) PID() int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.pid
}

// Uptime returns how long the worker has been running
func (w *Worker) Uptime() time.Duration {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.status != WorkerStatusRunning {
		return 0
	}
	return time.Since(w.startTime)
}

// RestartCount returns how many times this worker has been restarted
func (w *Worker) RestartCount() int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.restartCount
}

// Info returns a snapshot of the worker's current info
type WorkerInfo struct {
	ID           int    `json:"id"`
	Port         int    `json:"port"`
	Status       string `json:"status"`
	PID          int    `json:"pid"`
	RequestCount int64  `json:"request_count"`
	Uptime       string `json:"uptime"`
	RestartCount int    `json:"restart_count"`
}

// Info returns a snapshot of worker information
func (w *Worker) Info() WorkerInfo {
	w.mu.RLock()
	defer w.mu.RUnlock()

	uptime := ""
	if w.status == WorkerStatusRunning {
		uptime = time.Since(w.startTime).Round(time.Second).String()
	}

	return WorkerInfo{
		ID:           w.ID,
		Port:         w.Port,
		Status:       w.status.String(),
		PID:          w.pid,
		RequestCount: w.requestCount.Load(),
		Uptime:       uptime,
		RestartCount: w.restartCount,
	}
}

// monitor watches the process and marks it dead if it exits unexpectedly
func (w *Worker) monitor() {
	if w.cmd == nil {
		return
	}
	w.cmd.Wait()
	w.mu.Lock()
	if w.status == WorkerStatusRunning {
		w.status = WorkerStatusDead
	}
	w.mu.Unlock()
}
