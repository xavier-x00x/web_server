package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
)

// SiteConfig represents a single PHP site environment
type SiteConfig struct {
	Name          string `json:"name"`
	PHPBinaryPath string `json:"php_binary_path"`
	WorkerCount   int    `json:"worker_count"`
	BasePort      int    `json:"base_port"`
	NginxPort     int    `json:"nginx_port"`
	DocumentRoot  string `json:"document_root"`
}

// WorkerPorts returns a list of all worker ports for this site
func (s *SiteConfig) WorkerPorts() []int {
	ports := make([]int, s.WorkerCount)
	for i := 0; i < s.WorkerCount; i++ {
		ports[i] = s.BasePort + i
	}
	return ports
}

// Config holds all GopherStack configuration
type Config struct {
	// Sites configuration
	Sites []SiteConfig `json:"sites"`

	// Global PHP settings
	MaxRequests   int  `json:"max_requests"`    // Requests before worker recycle (default: 500)
	MaxMemoryMB   int  `json:"max_memory_mb"`   // Memory limit per worker in MB (default: 128)
	PHPChildren   int  `json:"php_children"`    // PHP child processes per worker (default: 4)
	EnableOpCache bool `json:"enable_opcache"`  // Toggle PHP OpCache (default: false)

	// Global Nginx settings
	NginxBinaryPath string `json:"nginx_binary_path"` // Path to nginx.exe

	// Global General settings
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
	if len(c.Sites) == 0 {
		return fmt.Errorf("no sites configured in gopherstack.json")
	}

	if c.MaxRequests < 1 {
		return fmt.Errorf("max_requests must be at least 1, got %d", c.MaxRequests)
	}
	if c.MaxMemoryMB < 16 {
		return fmt.Errorf("max_memory_mb must be at least 16, got %d", c.MaxMemoryMB)
	}
	if c.DashboardPort < 1 || c.DashboardPort > 65535 {
		return fmt.Errorf("dashboard_port must be between 1 and 65535, got %d", c.DashboardPort)
	}

	ports := map[int]string{
		c.DashboardPort: "dashboard",
	}

	for _, site := range c.Sites {
		if site.Name == "" {
			return fmt.Errorf("site missing name")
		}
		if site.WorkerCount < 1 {
			return fmt.Errorf("site %s: worker_count must be at least 1, got %d", site.Name, site.WorkerCount)
		}
		if site.BasePort < 1024 || site.BasePort > 65535 {
			return fmt.Errorf("site %s: base_port must be between 1024 and 65535, got %d", site.Name, site.BasePort)
		}
		if site.NginxPort < 1 || site.NginxPort > 65535 {
			return fmt.Errorf("site %s: nginx_port must be between 1 and 65535, got %d", site.Name, site.NginxPort)
		}

		if name, exists := ports[site.NginxPort]; exists {
			return fmt.Errorf("site %s: nginx port %d conflicts with %s port", site.Name, site.NginxPort, name)
		}
		ports[site.NginxPort] = fmt.Sprintf("nginx_%s", site.Name)

		for i := 0; i < site.WorkerCount; i++ {
			port := site.BasePort + i
			if name, exists := ports[port]; exists {
				return fmt.Errorf("site %s: worker port %d conflicts with %s port", site.Name, port, name)
			}
			ports[port] = fmt.Sprintf("worker_%s_%d", site.Name, i)
		}
	}

	return nil
}

func (c *Config) applyEnvOverrides() {
	if v := os.Getenv("GOPHERSTACK_DASHBOARD_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.DashboardPort = n
		}
	}
	if v := os.Getenv("GOPHERSTACK_NGINX_PATH"); v != "" {
		c.NginxBinaryPath = v
	}
	// For backward compatibility, apply old env vars to the first site
	if len(c.Sites) > 0 {
		if v := os.Getenv("GOPHERSTACK_WORKERS"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				c.Sites[0].WorkerCount = n
			}
		}
		if v := os.Getenv("GOPHERSTACK_BASE_PORT"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				c.Sites[0].BasePort = n
			}
		}
		if v := os.Getenv("GOPHERSTACK_NGINX_PORT"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				c.Sites[0].NginxPort = n
			}
		}
		if v := os.Getenv("GOPHERSTACK_DOCUMENT_ROOT"); v != "" {
			c.Sites[0].DocumentRoot = v
		}
		if v := os.Getenv("GOPHERSTACK_PHP_PATH"); v != "" {
			c.Sites[0].PHPBinaryPath = v
		}
	}
}

func (c *Config) resolvePaths() {
	resolve := func(p string) string {
		if filepath.IsAbs(p) {
			return p
		}
		return filepath.Join(c.BaseDir, p)
	}

	c.LogDir = resolve(c.LogDir)
	c.ConfigDir = resolve(c.ConfigDir)
	c.BinDir = resolve(c.BinDir)

	if c.NginxBinaryPath == "" {
		c.NginxBinaryPath = filepath.Join(c.BinDir, "nginx", "gopher-nginx.exe")
	} else {
		c.NginxBinaryPath = resolve(c.NginxBinaryPath)
	}

	for i := range c.Sites {
		c.Sites[i].DocumentRoot = resolve(c.Sites[i].DocumentRoot)
		if c.Sites[i].PHPBinaryPath == "" {
			c.Sites[i].PHPBinaryPath = filepath.Join(c.BinDir, "php", "php-cgi.exe")
		} else {
			c.Sites[i].PHPBinaryPath = resolve(c.Sites[i].PHPBinaryPath)
		}
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

	defaultSite := SiteConfig{
		Name:          "default",
		PHPBinaryPath: filepath.Join("bin", "php", "8.1.10", "gopher-php.exe"),
		WorkerCount:   workerCount,
		BasePort:      9001,
		NginxPort:     8088,
		DocumentRoot:  "www",
	}

	return &Config{
		Sites:               []SiteConfig{defaultSite},
		MaxRequests:         500,
		MaxMemoryMB:         128,
		PHPChildren:         4,
		DashboardPort:       8090,
		LogDir:              "logs",
		ConfigDir:           "config",
		BinDir:              "bin",
		HealthCheckInterval: 5,
		EnableOpCache:       false,
	}
}
