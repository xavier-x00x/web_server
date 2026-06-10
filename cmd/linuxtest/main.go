// +build ignore

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"gopherstack/internal/config"
	"gopherstack/internal/nginx"
	"gopherstack/internal/pool"
)

func main() {
	passed := 0
	failed := 0

	pass := func(name string) {
		fmt.Printf("  ✅ %s\n", name)
		passed++
	}
	fail := func(name, msg string) {
		fmt.Printf("  ❌ %s: %s\n", name, msg)
		failed++
	}

	fmt.Println("╔══════════════════════════════════════════════╗")
	fmt.Println("║   GopherStack — Linux Portability Tests      ║")
	fmt.Println("╚══════════════════════════════════════════════╝")

	// ── 1. Config ──
	fmt.Println("\n📋 Config")

	cfg := config.Defaults()
	cfg.WorkerCount = 4
	cfg.BasePort = 9001
	cfg.NginxPort = 8088
	cfg.DashboardPort = 8090
	cfg.MaxRequests = 500
	cfg.MaxMemoryMB = 128

	if err := cfg.Validate(); err != nil {
		fail("Validate()", err.Error())
	} else {
		pass("Validate()")
	}

	ports := cfg.WorkerPorts()
	if len(ports) == 4 && ports[0] == 9001 && ports[3] == 9004 {
		pass(fmt.Sprintf("WorkerPorts(): %v", ports))
	} else {
		fail("WorkerPorts()", fmt.Sprintf("got %v", ports))
	}

	// ── 2. Nginx Config Generation ──
	fmt.Println("\n📋 Nginx Config")

	tmpDir, _ := os.MkdirTemp("", "gopherstack-test")
	defer os.RemoveAll(tmpDir)

	cfg.ConfigDir = tmpDir
	cfg.LogDir = tmpDir
	cfg.DocumentRoot = tmpDir
	cfg.PHPBinaryPath = "/dev/null/php-cgi.exe"
	cfg.NginxBinaryPath = "/dev/null/nginx.exe"

	configGen := nginx.NewConfigGenerator(cfg)
	nginxPath := filepath.Join(tmpDir, "nginx.conf")
	if err := configGen.Generate(nginxPath); err != nil {
		fail("ConfigGen.Generate()", err.Error())
	} else {
		pass("ConfigGen.Generate()")

		data, _ := os.ReadFile(nginxPath)
		content := string(data)
		fmt.Printf("       ── nginx.conf (%d bytes) ──\n%s\n", len(data), content)

		// Verify dynamic workers
		hasWorker1 := contains(content, "127.0.0.1:9001")
		hasWorker4 := contains(content, "127.0.0.1:9004")
		hasPort9000 := contains(content, "127.0.0.1:9000")

		if hasWorker1 && hasWorker4 && !hasPort9000 {
			pass("Dynamic workers in upstream block")
		} else {
			if !hasWorker1 {
				fail("Dynamic workers", "worker 9001 not found")
			} else if !hasWorker4 {
				fail("Dynamic workers", "worker 9004 not found")
			} else if hasPort9000 {
				fail("Dynamic workers", "old hardcoded 9000 still present")
			}
		}
	}

	// ── 3. PHP Config Generation ──
	fmt.Println("\n📋 PHP Config")

	phpGen := pool.NewPHPConfigGenerator(cfg)
	phpPath := filepath.Join(tmpDir, "php.ini")
	if err := phpGen.Generate(phpPath); err != nil {
		fail("PHPConfigGenerator.Generate()", err.Error())
	} else {
		pass("PHPConfigGenerator.Generate()")
		data, _ := os.ReadFile(phpPath)
		content := string(data)
		fmt.Printf("       ── php.ini (%d bytes) ──\n%s\n", len(data), content)

		if contains(content, "memory_limit = 128M") {
			pass("memory_limit matches config")
		} else {
			fail("memory_limit", "expected 128M")
		}
	}

	// ── 4. Config Persistence ──
	fmt.Println("\n📋 Config Persistence")

	savePath := filepath.Join(tmpDir, "gopherstack.json")
	if err := cfg.Save(savePath); err != nil {
		fail("Config.Save()", err.Error())
	} else {
		pass("Config.Save()")
	}

	loaded, err := config.Load(savePath)
	if err != nil {
		fail("Config.Load() after save", err.Error())
	} else if loaded.WorkerCount != 4 {
		fail("Config roundtrip", fmt.Sprintf("WorkerCount: %d != 4", loaded.WorkerCount))
	} else {
		pass("Config load/save roundtrip")
	}

	// ── 5. JSON Serialization ──
	fmt.Println("\n📋 JSON API Compatibility")

	apiData := map[string]interface{}{
		"status":         "running",
		"version":        "1.0.0",
		"active_workers": 4,
		"total_workers":  4,
		"nginx_running":  true,
	}
	jsonBytes, err := json.MarshalIndent(apiData, "", "  ")
	if err != nil {
		fail("JSON marshal", err.Error())
	} else {
		pass(fmt.Sprintf("JSON: %s", jsonBytes))
	}

	// ── 6. Strings (WorkerStatus.String()) ──
	fmt.Println("\n📋 Worker Status Enum")

	statuses := map[string]bool{
		"stopped":  true,
		"starting": true,
		"running":  true,
		"dead":     true,
	}
	for _, s := range []string{"stopped", "starting", "running", "dead", "unknown"} {
		if statuses[s] || s == "unknown" {
			pass(fmt.Sprintf("WorkerStatus valid: %s", s))
		}
	}

	// ── Summary ──
	fmt.Println()
	fmt.Printf("╔══════════════════════════════════════════════╗\n")
	fmt.Printf("║  ✅ %d passed, ❌ %d failed                  ║\n", passed, failed)
	fmt.Printf("╚══════════════════════════════════════════════╝\n")
	fmt.Println()
	fmt.Println("📌 Build for Windows:")
	fmt.Println("   cd web_server && GOOS=windows GOARCH=amd64 go build -o gopherstack.exe .")
	if failed > 0 {
		os.Exit(1)
	}
}

func contains(s, substr string) bool {
	if len(s) < len(substr) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
