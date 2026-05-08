package nginx

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"gopherstack/internal/config"
)

// Manager handles Nginx process lifecycle
type Manager struct {
	cfg     *config.Config
	cmd     *exec.Cmd
	mu      sync.RWMutex
	running bool
	pid     int
}

// NewManager creates a new Nginx manager
func NewManager(cfg *config.Config) *Manager {
	return &Manager{
		cfg: cfg,
	}
}

// Start launches the Nginx process
func (m *Manager) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return nil
	}

	// Ensure the nginx config is generated
	configGen := NewConfigGenerator(m.cfg)
	configPath := filepath.Join(m.cfg.ConfigDir, "nginx.conf")

	if err := configGen.Generate(configPath); err != nil {
		return fmt.Errorf("failed to generate nginx config: %w", err)
	}

	// Validate the config
	if err := m.validateConfig(configPath); err != nil {
		return fmt.Errorf("nginx config validation failed: %w", err)
	}

	// Get nginx directory (needed as prefix)
	nginxDir := filepath.Dir(m.cfg.NginxBinaryPath)

	// Ensure logs directory exists within nginx dir
	os.MkdirAll(filepath.Join(nginxDir, "logs"), 0755)
	// Also create a temp directory for nginx
	os.MkdirAll(filepath.Join(nginxDir, "temp"), 0755)

	absConfigPath, _ := filepath.Abs(configPath)

	m.cmd = exec.Command(m.cfg.NginxBinaryPath,
		"-c", absConfigPath,
		"-p", nginxDir+"/",
	)
	m.cmd.Dir = nginxDir
	m.cmd.Stdout = os.Stdout
	m.cmd.Stderr = os.Stderr
	m.cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}

	if err := m.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start nginx: %w", err)
	}

	m.pid = m.cmd.Process.Pid
	m.running = true

	log.Printf("[nginx] Started (PID: %d)", m.pid)

	// Monitor nginx in background
	go func() {
		m.cmd.Wait()
		m.mu.Lock()
		m.running = false
		m.mu.Unlock()
		log.Println("[nginx] Process exited")
	}()

	return nil
}

// Stop gracefully stops Nginx
func (m *Manager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running || m.cmd == nil || m.cmd.Process == nil {
		return nil
	}

	log.Println("[nginx] Stopping...")

	// Send quit signal via nginx -s stop
	stopCmd := exec.Command(m.cfg.NginxBinaryPath, "-s", "stop",
		"-p", filepath.Dir(m.cfg.NginxBinaryPath)+"/",
		"-c", filepath.Join(m.cfg.ConfigDir, "nginx.conf"),
	)
	stopCmd.Dir = filepath.Dir(m.cfg.NginxBinaryPath)
	if err := stopCmd.Run(); err != nil {
		// If graceful stop fails, kill the process
		m.cmd.Process.Kill()
	}

	// Wait a bit for shutdown
	time.Sleep(500 * time.Millisecond)
	m.running = false
	log.Println("[nginx] Stopped")

	return nil
}

// Reload sends a reload signal to Nginx for config hot-reload
func (m *Manager) Reload() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.running {
		return fmt.Errorf("nginx is not running")
	}

	// Regenerate config
	configGen := NewConfigGenerator(m.cfg)
	configPath := filepath.Join(m.cfg.ConfigDir, "nginx.conf")
	if err := configGen.Generate(configPath); err != nil {
		return fmt.Errorf("failed to regenerate nginx config: %w", err)
	}

	// Validate before reload
	if err := m.validateConfig(configPath); err != nil {
		return fmt.Errorf("nginx config validation failed: %w", err)
	}

	reloadCmd := exec.Command(m.cfg.NginxBinaryPath, "-s", "reload",
		"-p", filepath.Dir(m.cfg.NginxBinaryPath)+"/",
		"-c", configPath,
	)
	reloadCmd.Dir = filepath.Dir(m.cfg.NginxBinaryPath)

	if err := reloadCmd.Run(); err != nil {
		return fmt.Errorf("failed to reload nginx: %w", err)
	}

	log.Println("[nginx] Configuration reloaded")
	return nil
}

// IsRunning returns whether Nginx is running
func (m *Manager) IsRunning() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.running
}

// PID returns the Nginx process ID
func (m *Manager) PID() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.pid
}

func (m *Manager) validateConfig(configPath string) error {
	absConfigPath, _ := filepath.Abs(configPath)
	testCmd := exec.Command(m.cfg.NginxBinaryPath, "-t",
		"-c", absConfigPath,
		"-p", filepath.Dir(m.cfg.NginxBinaryPath)+"/",
	)
	testCmd.Dir = filepath.Dir(m.cfg.NginxBinaryPath)
	output, err := testCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("config validation error: %s - %w", string(output), err)
	}
	return nil
}
