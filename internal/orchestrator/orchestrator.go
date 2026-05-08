package orchestrator

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"gopherstack/internal/config"
	"gopherstack/internal/dashboard"
	"gopherstack/internal/monitor"
	"gopherstack/internal/nginx"
	"gopherstack/internal/pool"
	"gopherstack/internal/setup"
)

// Orchestrator is the main coordinator for all GopherStack components
type Orchestrator struct {
	cfg          *config.Config
	poolManager  *pool.Manager
	nginxManager *nginx.Manager
	monitor      *monitor.Monitor
	dashboard    *dashboard.Server
	stopChan     chan struct{}
}

// New creates a new Orchestrator
func New(cfg *config.Config) *Orchestrator {
	return &Orchestrator{
		cfg:      cfg,
		stopChan: make(chan struct{}),
	}
}

// Start initializes and starts all components
func (o *Orchestrator) Start() error {
	log.Println("╔══════════════════════════════════════════╗")
	log.Println("║   GopherStack Enterprise v1.0.0          ║")
	log.Println("║   High-Concurrency PHP Orchestrator      ║")
	log.Println("╚══════════════════════════════════════════╝")

	// Validate config
	if err := o.cfg.Validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	// Ensure directories exist
	os.MkdirAll(o.cfg.DocumentRoot, 0755)
	os.MkdirAll(o.cfg.LogDir, 0755)
	os.MkdirAll(o.cfg.ConfigDir, 0755)

	// Step 1: Ensure binaries are downloaded
	log.Println("[orchestrator] Checking binaries...")
	downloader := setup.NewDownloader(o.cfg, func(downloaded, total int64, name string) {
		if total > 0 {
			pct := float64(downloaded) / float64(total) * 100
			fmt.Printf("\r[setup] Downloading %s: %.1f%%", name, pct)
		} else {
			fmt.Printf("\r[setup] Downloading %s: %d MB", name, downloaded/(1024*1024))
		}
	})
	if err := downloader.EnsureBinaries(); err != nil {
		return fmt.Errorf("failed to setup binaries: %w", err)
	}

	// Step 2: Create default PHP files if www directory is empty
	o.ensureDefaultPHPFiles()

	// Step 3: Start PHP worker pool
	log.Println("[orchestrator] Generating PHP configuration...")
	phpGen := pool.NewPHPConfigGenerator(o.cfg)
	phpIniPath := filepath.Join(o.cfg.ConfigDir, "php.ini")
	if err := phpGen.Generate(phpIniPath); err != nil {
		return fmt.Errorf("failed to generate PHP config: %w", err)
	}

	log.Println("[orchestrator] Starting PHP worker pool...")
	o.poolManager = pool.NewManager(o.cfg)
	o.poolManager.SetPHPIni(phpIniPath) // Pass the generated ini path
	if err := o.poolManager.Start(); err != nil {
		return fmt.Errorf("failed to start PHP pool: %w", err)
	}

	// Step 4: Start Nginx
	log.Println("[orchestrator] Starting Nginx...")
	o.nginxManager = nginx.NewManager(o.cfg)
	if err := o.nginxManager.Start(); err != nil {
		log.Printf("[orchestrator] Warning: Failed to start Nginx: %v", err)
		log.Println("[orchestrator] The system will work via the built-in proxy")
	}

	// Step 5: Start Monitor
	log.Println("[orchestrator] Starting health monitor...")
	o.monitor = monitor.NewMonitor(o.cfg, o.poolManager, o.nginxManager)
	o.monitor.Start()

	// Step 6: Start Admin Dashboard
	log.Printf("[orchestrator] Starting admin dashboard on port %d...", o.cfg.DashboardPort)
	o.dashboard = dashboard.NewServer(o.cfg, o.poolManager, o.nginxManager, o.monitor)
	go func() {
		if err := o.dashboard.Start(); err != nil {
			log.Printf("[orchestrator] Dashboard error: %v", err)
		}
	}()

	log.Println("")
	log.Println("╔══════════════════════════════════════════╗")
	log.Printf("║  Nginx:       http://localhost:%-10d ║\n", o.cfg.NginxPort)
	log.Printf("║  Dashboard:   http://localhost:%-10d ║\n", o.cfg.DashboardPort)
	log.Printf("║  PHP Workers: %d active                   ║\n", o.poolManager.ActiveWorkerCount())
	log.Println("╚══════════════════════════════════════════╝")
	log.Println("")
	log.Println("[orchestrator] GopherStack is running. Press Ctrl+C to stop.")

	return nil
}

// Stop gracefully shuts down all components
func (o *Orchestrator) Stop() error {
	log.Println("[orchestrator] Shutting down GopherStack...")

	// Stop in reverse order
	if o.dashboard != nil {
		o.dashboard.Stop()
	}

	if o.monitor != nil {
		o.monitor.Stop()
	}

	if o.nginxManager != nil {
		o.nginxManager.Stop()
	}

	if o.poolManager != nil {
		o.poolManager.Stop()
	}

	log.Println("[orchestrator] GopherStack stopped.")
	return nil
}

// Wait blocks until the orchestrator is signaled to stop
func (o *Orchestrator) Wait() {
	<-o.stopChan
}

// Signal sends a stop signal to the orchestrator
func (o *Orchestrator) Signal() {
	select {
	case <-o.stopChan:
	default:
		close(o.stopChan)
	}
}

// PoolManager returns the pool manager
func (o *Orchestrator) PoolManager() *pool.Manager {
	return o.poolManager
}

// NginxManager returns the nginx manager
func (o *Orchestrator) NginxManager() *nginx.Manager {
	return o.nginxManager
}

func (o *Orchestrator) ensureDefaultPHPFiles() {
	indexPath := fmt.Sprintf("%s/index.php", o.cfg.DocumentRoot)
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		defaultPHP := `<?php
/**
 * GopherStack Enterprise - Default Welcome Page
 * PHP <?= phpversion() ?> is running successfully!
 */
?>
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>GopherStack Enterprise</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: 'Segoe UI', system-ui, -apple-system, sans-serif;
            background: linear-gradient(135deg, #0f0c29, #302b63, #24243e);
            color: #e0e0e0;
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
        }
        .container {
            text-align: center;
            padding: 3rem;
            background: rgba(255,255,255,0.05);
            border-radius: 20px;
            backdrop-filter: blur(10px);
            border: 1px solid rgba(255,255,255,0.1);
            max-width: 600px;
            box-shadow: 0 25px 50px rgba(0,0,0,0.3);
        }
        h1 {
            font-size: 2.5rem;
            background: linear-gradient(135deg, #667eea, #764ba2);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
            margin-bottom: 1rem;
        }
        .version {
            color: #a78bfa;
            font-size: 1.1rem;
            margin-bottom: 2rem;
        }
        .info {
            display: grid;
            grid-template-columns: 1fr 1fr;
            gap: 1rem;
            margin-top: 2rem;
        }
        .info-card {
            background: rgba(255,255,255,0.05);
            padding: 1rem;
            border-radius: 12px;
            border: 1px solid rgba(255,255,255,0.08);
        }
        .info-card h3 { color: #818cf8; font-size: 0.85rem; text-transform: uppercase; }
        .info-card p { color: #e0e0e0; font-size: 1.2rem; margin-top: 0.5rem; }
        .badge {
            display: inline-block;
            background: linear-gradient(135deg, #10b981, #059669);
            color: white;
            padding: 0.3rem 1rem;
            border-radius: 20px;
            font-size: 0.85rem;
            margin-top: 1rem;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>⚡ GopherStack Enterprise</h1>
        <p class="version">High-Concurrency PHP Orchestrator for Windows Server</p>
        <span class="badge">✓ Running</span>
        <div class="info">
            <div class="info-card">
                <h3>PHP Version</h3>
                <p><?= phpversion() ?></p>
            </div>
            <div class="info-card">
                <h3>Server</h3>
                <p><?= php_sapi_name() ?></p>
            </div>
            <div class="info-card">
                <h3>OS</h3>
                <p><?= PHP_OS ?></p>
            </div>
            <div class="info-card">
                <h3>Time</h3>
                <p><?= date('H:i:s') ?></p>
            </div>
        </div>
    </div>
</body>
</html>
`
		os.WriteFile(indexPath, []byte(defaultPHP), 0644)
		log.Println("[orchestrator] Created default index.php")
	}
}
