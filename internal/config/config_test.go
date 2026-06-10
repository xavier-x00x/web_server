package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaults(t *testing.T) {
	cfg := Defaults()

	if cfg.WorkerCount == 0 {
		t.Error("Defaults() WorkerCount should not be 0")
	}
	if cfg.BasePort != 9001 {
		t.Errorf("Defaults() BasePort = %d, want 9001", cfg.BasePort)
	}
	if cfg.MaxRequests != 500 {
		t.Errorf("Defaults() MaxRequests = %d, want 500", cfg.MaxRequests)
	}
	if cfg.NginxPort != 80 {
		t.Errorf("Defaults() NginxPort = %d, want 80", cfg.NginxPort)
	}
	if cfg.DashboardPort != 8090 {
		t.Errorf("Defaults() DashboardPort = %d, want 8090", cfg.DashboardPort)
	}
	if cfg.MaxMemoryMB != 128 {
		t.Errorf("Defaults() MaxMemoryMB = %d, want 128", cfg.MaxMemoryMB)
	}
	if cfg.HealthCheckInterval != 5 {
		t.Errorf("Defaults() HealthCheckInterval = %d, want 5", cfg.HealthCheckInterval)
	}
	if cfg.DocumentRoot != "www" {
		t.Errorf("Defaults() DocumentRoot = %s, want www", cfg.DocumentRoot)
	}
}

func TestDefaultsWorkerCountRange(t *testing.T) {
	cfg := Defaults()
	// Should be at least 4 and at most 32
	if cfg.WorkerCount < 4 {
		t.Errorf("Defaults() WorkerCount = %d, minimum is 4", cfg.WorkerCount)
	}
	if cfg.WorkerCount > 32 {
		t.Errorf("Defaults() WorkerCount = %d, maximum is 32", cfg.WorkerCount)
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
	}{
		{
			name: "valid minimal config",
			cfg: &Config{
				WorkerCount:         4,
				BasePort:            9001,
				MaxRequests:         500,
				MaxMemoryMB:         128,
				NginxPort:           8080,
				DashboardPort:       8090,
				HealthCheckInterval: 5,
			},
			wantErr: false,
		},
		{
			name: "invalid worker count - zero",
			cfg: &Config{
				WorkerCount: 0,
				BasePort:    9001,
			},
			wantErr: true,
		},
		{
			name: "invalid base port - too low",
			cfg: &Config{
				WorkerCount: 4,
				BasePort:    100,
			},
			wantErr: true,
		},
		{
			name: "invalid max memory - too low",
			cfg: &Config{
				WorkerCount:   4,
				BasePort:      9001,
				MaxRequests:   500,
				MaxMemoryMB:   8,
				NginxPort:     8080,
				DashboardPort: 8090,
			},
			wantErr: true,
		},
		{
			name: "nginx-dashboard port conflict",
			cfg: &Config{
				WorkerCount:   4,
				BasePort:      9001,
				MaxRequests:   500,
				MaxMemoryMB:   128,
				NginxPort:     8090,
				DashboardPort: 8090,
			},
			wantErr: true,
		},
		{
			name: "nginx-worker port conflict",
			cfg: &Config{
				WorkerCount:   4,
				BasePort:      8080,
				MaxRequests:   500,
				MaxMemoryMB:   128,
				NginxPort:     8081,
				DashboardPort: 8090,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestWorkerPorts(t *testing.T) {
	cfg := &Config{
		WorkerCount: 4,
		BasePort:    9001,
	}

	ports := cfg.WorkerPorts()
	if len(ports) != 4 {
		t.Errorf("WorkerPorts() returned %d ports, want 4", len(ports))
	}

	expected := []int{9001, 9002, 9003, 9004}
	for i, p := range ports {
		if p != expected[i] {
			t.Errorf("WorkerPorts()[%d] = %d, want %d", i, p, expected[i])
		}
	}
}

func TestLoadFromFile(t *testing.T) {
	// Create a temp config file
	tmpDir := t.TempDir()
	configContent := `{
		"worker_count": 8,
		"base_port": 8001,
		"nginx_port": 8088
	}`
	configPath := filepath.Join(tmpDir, "test_config.json")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Override environment to clean state
	os.Unsetenv("GOPHERSTACK_WORKERS")
	os.Unsetenv("GOPHERSTACK_BASE_PORT")

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.WorkerCount != 8 {
		t.Errorf("Load() WorkerCount = %d, want 8", cfg.WorkerCount)
	}
	if cfg.BasePort != 8001 {
		t.Errorf("Load() BasePort = %d, want 8001", cfg.BasePort)
	}
	// DashboardPort not in config file, should use default
	if cfg.DashboardPort != 8090 {
		t.Errorf("Load() DashboardPort (default) = %d, want 8090", cfg.DashboardPort)
	}
}

func TestLoadEnvOverrides(t *testing.T) {
	os.Setenv("GOPHERSTACK_WORKERS", "16")
	os.Setenv("GOPHERSTACK_BASE_PORT", "7001")
	os.Setenv("GOPHERSTACK_NGINX_PORT", "3000")
	defer func() {
		os.Unsetenv("GOPHERSTACK_WORKERS")
		os.Unsetenv("GOPHERSTACK_BASE_PORT")
		os.Unsetenv("GOPHERSTACK_NGINX_PORT")
	}()

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.WorkerCount != 16 {
		t.Errorf("Env override WorkerCount = %d, want 16", cfg.WorkerCount)
	}
	if cfg.BasePort != 7001 {
		t.Errorf("Env override BasePort = %d, want 7001", cfg.BasePort)
	}
	if cfg.NginxPort != 3000 {
		t.Errorf("Env override NginxPort = %d, want 3000", cfg.NginxPort)
	}
}

func TestSave(t *testing.T) {
	tmpDir := t.TempDir()
	savePath := filepath.Join(tmpDir, "saved.json")

	cfg := Defaults()
	cfg.WorkerCount = 12
	cfg.BasePort = 9500

	if err := cfg.Save(savePath); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Read back
	loaded, err := Load(savePath)
	if err != nil {
		t.Fatalf("Load() after Save() error = %v", err)
	}

	if loaded.WorkerCount != 12 {
		t.Errorf("Load after Save WorkerCount = %d, want 12", loaded.WorkerCount)
	}
	if loaded.BasePort != 9500 {
		t.Errorf("Load after Save BasePort = %d, want 9500", loaded.BasePort)
	}
}
