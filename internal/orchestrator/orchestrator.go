package orchestrator

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"gopherstack/internal/config"
	"gopherstack/internal/dashboard"
	"gopherstack/internal/monitor"
	"gopherstack/internal/nginx"
	"gopherstack/internal/pool"
)

// Orchestrator is the main coordinator for all GopherStack components
type Orchestrator struct {
	cfg          *config.Config
	poolManagers map[string]*pool.Manager
	nginxManager *nginx.Manager
	monitor      *monitor.Monitor
	dashboard    *dashboard.Server
	stopChan     chan struct{}
}

// New creates a new Orchestrator
func New(cfg *config.Config) *Orchestrator {
	return &Orchestrator{
		cfg:          cfg,
		poolManagers: make(map[string]*pool.Manager),
		stopChan:     make(chan struct{}),
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

	// Ensure global directories exist
	os.MkdirAll(o.cfg.LogDir, 0755)
	os.MkdirAll(o.cfg.ConfigDir, 0755)

	// Step 0: Check port availability before starting anything
	log.Println("[orchestrator] Checking port availability...")

	// Also check and auto-resolve worker port conflicts (e.g., with Laragon)
	if ResolveWorkerPortConflicts(o.cfg) {
		log.Println("[orchestrator] ✅ Workers will use shifted port range")
	}

	// Standard port check (nginx + dashboard)
	ports := CheckPorts(o.cfg)
	hasConflicts := false
	for _, p := range ports {
		if p.InUse {
			hasConflicts = true
		}
	}
	if hasConflicts {
		log.Println("[orchestrator] ⚠️  Port conflicts detected on critical ports — will attempt to start anyway")
		log.Println("[orchestrator] If startup fails, free the ports above or change config")
	}

	// Step 1: Ensure gopher binaries exist (copied from originals)
	if err := o.ensureGopherBinaries(); err != nil {
		return fmt.Errorf("failed to ensure gopher binaries: %w", err)
	}

	// Step 2: Global Cleanup of Zombie Processes BEFORE starting any pools
	log.Println("[orchestrator] Cleaning up any zombie processes...")
	pool.CleanupZombieProcesses(o.cfg)

	// Step 3: Create default files and start PHP worker pools per site
	log.Println("[orchestrator] Generating PHP configuration and starting worker pools...")
	
	for _, site := range o.cfg.Sites {
		os.MkdirAll(site.DocumentRoot, 0755)
		o.ensureDefaultPHPFiles(site)
		
		log.Printf("[orchestrator] Starting PHP worker pool for site: %s", site.Name)
		
		phpGen := pool.NewPHPConfigGenerator(o.cfg, site)
		phpIniPath := filepath.Join(o.cfg.ConfigDir, fmt.Sprintf("php_%s.ini", site.Name))
		if err := phpGen.Generate(phpIniPath); err != nil {
			return fmt.Errorf("failed to generate PHP config for site %s: %w", site.Name, err)
		}

		pm := pool.NewManager(o.cfg, site)
		pm.SetPHPIni(phpIniPath) // Pass the generated ini path
		if err := pm.Start(); err != nil {
			return fmt.Errorf("failed to start PHP pool for site %s: %w", site.Name, err)
		}
		
		o.poolManagers[site.Name] = pm
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
	o.monitor = monitor.NewMonitor(o.cfg, o.poolManagers, o.nginxManager)
	o.monitor.Start()

	// Step 6: Start Admin Dashboard
	log.Printf("[orchestrator] Starting admin dashboard on port %d...", o.cfg.DashboardPort)
	o.dashboard = dashboard.NewServer(o.cfg, o.poolManagers, o.nginxManager, o.monitor)
	go func() {
		if err := o.dashboard.Start(); err != nil {
			log.Printf("[orchestrator] Dashboard error: %v", err)
		}
	}()

	log.Println("")
	log.Println("╔══════════════════════════════════════════╗")
	for _, site := range o.cfg.Sites {
		log.Printf("║  Site [%s] Nginx: http://localhost:%-5d ║\n", site.Name, site.NginxPort)
	}
	log.Printf("║  Dashboard:   http://localhost:%-10d ║\n", o.cfg.DashboardPort)
	
	totalActive := 0
	for _, pm := range o.poolManagers {
		totalActive += pm.ActiveWorkerCount()
	}
	log.Printf("║  PHP Workers: %d active                   ║\n", totalActive)
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

	if o.poolManagers != nil {
		for name, pm := range o.poolManagers {
			log.Printf("[orchestrator] Stopping pool for site: %s", name)
			pm.Stop()
		}
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

// PoolManagers returns the pool managers
func (o *Orchestrator) PoolManagers() map[string]*pool.Manager {
	return o.poolManagers
}

// NginxManager returns the nginx manager
func (o *Orchestrator) NginxManager() *nginx.Manager {
	return o.nginxManager
}

func (o *Orchestrator) ensureDefaultPHPFiles(site config.SiteConfig) {
	indexPath := filepath.Join(site.DocumentRoot, "index.php")
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
    <title>GopherStack Enterprise - ` + site.Name + `</title>
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
        <h1>⚡ GopherStack - <?= htmlspecialchars("` + site.Name + `") ?></h1>
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
		log.Printf("[orchestrator] Created default index.php for site %s", site.Name)
	}
}

func (o *Orchestrator) ensureGopherBinaries() error {
	log.Println("[orchestrator] Ensuring gopher binaries exist...")

	// For PHP (each site)
	for _, site := range o.cfg.Sites {
		phpPath := site.PHPBinaryPath
		if _, err := os.Stat(phpPath); os.IsNotExist(err) {
			origPath := filepath.Join(filepath.Dir(phpPath), "php-cgi.exe")
			if _, err := os.Stat(origPath); err == nil {
				log.Printf("[orchestrator] Copying %s to %s for site %s", filepath.Base(origPath), filepath.Base(phpPath), site.Name)
				if err := copyFile(origPath, phpPath); err != nil {
					return fmt.Errorf("failed to copy PHP binary for site %s: %w", site.Name, err)
				}
			} else {
				return fmt.Errorf("neither %s nor %s found for site %s", phpPath, origPath, site.Name)
			}
		}
	}

	// For Nginx
	nginxPath := o.cfg.NginxBinaryPath
	if _, err := os.Stat(nginxPath); os.IsNotExist(err) {
		origPath := filepath.Join(filepath.Dir(nginxPath), "nginx.exe")
		if _, err := os.Stat(origPath); err == nil {
			log.Printf("[orchestrator] Copying %s to %s", filepath.Base(origPath), filepath.Base(nginxPath))
			if err := copyFile(origPath, nginxPath); err != nil {
				return fmt.Errorf("failed to copy Nginx binary: %w", err)
			}
		} else {
			return fmt.Errorf("neither %s nor %s found", nginxPath, origPath)
		}
	}

	return nil
}

func copyFile(src, dst string) error {
	sourceFileStat, err := os.Stat(src)
	if err != nil {
		return err
	}

	if !sourceFileStat.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", src)
	}

	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()

	if _, err := io.Copy(destination, source); err != nil {
		return err
	}

	return os.Chmod(dst, sourceFileStat.Mode())
}
