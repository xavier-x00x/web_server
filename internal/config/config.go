package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
)

// Config holds all GopherStack configuration
type Config struct {
	// PHP settings
	PHPBinaryPath string `json:"php_binary_path"` // Path to php-cgi.exe
	WorkerCount   int    `json:"worker_count"`    // Number of PHP workers (default: CPU cores * 2)
	BasePort      int    `json:"base_port"`       // Starting port for PHP workers (default: 9001)
	MaxRequests   int    `json:"max_requests"`    // Requests before worker recycle (default: 500)
	MaxMemoryMB   int    `json:"max_memory_mb"`   // Memory limit per worker in MB (default: 128)
	PHPChildren   int    `json:"php_children"`    // PHP child processes per worker for internal recycling (default: 4)
	EnableOpCache bool   `json:"enable_opcache"`  // Toggle PHP OpCache (default: false)

	// Nginx settings
	NginxBinaryPath string `json:"nginx_binary_path"` // Path to nginx.exe
	NginxPort       int    `json:"nginx_port"`        // Nginx listen port (default: 80)

	// General settings
	DocumentRoot  string `json:"document_root"`  // PHP document root (default: ./www)
	DashboardPort int    `json:"dashboard_port"` // Admin dashboard port (default: 8090)
	LogDir        string `json:"log_dir"`        // Log directory (default: ./logs)
	ConfigDir     string `json:"config_dir"`     // Config directory (default: ./config)
	BinDir        string `json:"bin_dir"`        // Binary directory (default: ./bin)

	// Monitor settings
	HealthCheckInterval int `json:"health_check_interval"` // Health check interval in seconds (default: 5)

	// Internal - resolved paths
	BaseDir string `json:"-"` // Base directory of the application
}

// Load reads configuration from a JSON file, applying defaults and env overrides
func Load(configPath string) (*Config, error) {
	cfg := Defaults()

	// Determine base directory
	exe, err := os.Executable()
	if err != nil {
		cfg.BaseDir, _ = os.Getwd()
	} else {
		cfg.BaseDir = filepath.Dir(exe)
	}

	// Try to use current working directory if executable is in a temp/go-build dir
	if cwd, err := os.Getwd(); err == nil {
		cfg.BaseDir = cwd
	}

	// Try to load config file
	if configPath == "" {
		configPath = filepath.Join(cfg.BaseDir, "config", "gopherstack.json")
	}

	if data, err := os.ReadFile(configPath); err == nil {
		if err := json.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("failed to parse config file %s: %w", configPath, err)
		}
	}

	// Apply environment variable overrides
	cfg.applyEnvOverrides()

	// Resolve relative paths
	cfg.resolvePaths()

	return cfg, nil
}

// Save writes the current configuration to a JSON file
func (c *Config) Save(configPath string) error {
	if configPath == "" {
		configPath = filepath.Join(c.BaseDir, "config", "gopherstack.json")
	}

	// Ensure directory exists
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	return os.WriteFile(configPath, data, 0644)
}

// Validate checks that the configuration is valid
func (c *Config) Validate() error {
	if c.WorkerCount < 1 {
		return fmt.Errorf("worker_count must be at least 1, got %d", c.WorkerCount)
	}
	if c.BasePort < 1024 || c.BasePort > 65535 {
		return fmt.Errorf("base_port must be between 1024 and 65535, got %d", c.BasePort)
	}
	if c.MaxRequests < 1 {
		return fmt.Errorf("max_requests must be at least 1, got %d", c.MaxRequests)
	}
	if c.MaxMemoryMB < 16 {
		return fmt.Errorf("max_memory_mb must be at least 16, got %d", c.MaxMemoryMB)
	}
	if c.NginxPort < 1 || c.NginxPort > 65535 {
		return fmt.Errorf("nginx_port must be between 1 and 65535, got %d", c.NginxPort)
	}
	if c.DashboardPort < 1 || c.DashboardPort > 65535 {
		return fmt.Errorf("dashboard_port must be between 1 and 65535, got %d", c.DashboardPort)
	}
	// Ensure no port conflicts
	if c.NginxPort == c.DashboardPort {
		return fmt.Errorf("nginx port %d conflicts with dashboard port", c.NginxPort)
	}
	ports := map[int]string{
		c.NginxPort:     "nginx",
		c.DashboardPort: "dashboard",
	}
	for i := 0; i < c.WorkerCount; i++ {
		port := c.BasePort + i
		if name, exists := ports[port]; exists {
			return fmt.Errorf("worker port %d conflicts with %s port", port, name)
		}
		ports[port] = fmt.Sprintf("worker_%d", i)
	}
	return nil
}

// WorkerPorts returns a list of all worker ports
func (c *Config) WorkerPorts() []int {
	ports := make([]int, c.WorkerCount)
	for i := 0; i < c.WorkerCount; i++ {
		ports[i] = c.BasePort + i
	}
	return ports
}

func (c *Config) applyEnvOverrides() {
	if v := os.Getenv("GOPHERSTACK_WORKERS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.WorkerCount = n
		}
	}
	if v := os.Getenv("GOPHERSTACK_BASE_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.BasePort = n
		}
	}
	if v := os.Getenv("GOPHERSTACK_NGINX_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.NginxPort = n
		}
	}
	if v := os.Getenv("GOPHERSTACK_DASHBOARD_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.DashboardPort = n
		}
	}
	if v := os.Getenv("GOPHERSTACK_DOCUMENT_ROOT"); v != "" {
		c.DocumentRoot = v
	}
	if v := os.Getenv("GOPHERSTACK_PHP_PATH"); v != "" {
		c.PHPBinaryPath = v
	}
	if v := os.Getenv("GOPHERSTACK_NGINX_PATH"); v != "" {
		c.NginxBinaryPath = v
	}
}

func (c *Config) resolvePaths() {
	resolve := func(p string) string {
		if filepath.IsAbs(p) {
			return p
		}
		return filepath.Join(c.BaseDir, p)
	}

	c.DocumentRoot = resolve(c.DocumentRoot)
	c.LogDir = resolve(c.LogDir)
	c.ConfigDir = resolve(c.ConfigDir)
	c.BinDir = resolve(c.BinDir)

	if c.PHPBinaryPath == "" {
		c.PHPBinaryPath = filepath.Join(c.BinDir, "php", "php-cgi.exe")
	} else {
		c.PHPBinaryPath = resolve(c.PHPBinaryPath)
	}

	if c.NginxBinaryPath == "" {
		c.NginxBinaryPath = filepath.Join(c.BinDir, "nginx", "gopher-nginx.exe")
	} else {
		c.NginxBinaryPath = resolve(c.NginxBinaryPath)
	}
}

// Defaults returns a Config with sane default values
func Defaults() *Config {
	cpuCount := runtime.NumCPU()
	workerCount := cpuCount * 2
	if workerCount < 4 {
		workerCount = 4
	}
	if workerCount > 32 {
		workerCount = 32
	}

	return &Config{
		WorkerCount:         workerCount,
		BasePort:            9001,
		MaxRequests:         500,
		MaxMemoryMB:         128,
		PHPChildren:         4,
		NginxPort:           80,
		DashboardPort:       8090,
		DocumentRoot:        "www",
		LogDir:              "logs",
		ConfigDir:           "config",
		BinDir:              "bin",
		HealthCheckInterval: 5,
		EnableOpCache:       false,
	}
}
