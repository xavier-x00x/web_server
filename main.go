package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"gopherstack/internal/config"
	"gopherstack/internal/orchestrator"
	"gopherstack/internal/service"
)

const version = "1.0.0"

func main() {
	// Setup logging
	log.SetFlags(log.Ldate | log.Ltime | log.Lmsgprefix)
	log.SetPrefix("")

	// Parse command
	command := "start"
	configPath := ""

	args := os.Args[1:]
	for i, arg := range args {
		switch arg {
		case "--config", "-c":
			if i+1 < len(args) {
				configPath = args[i+1]
			}
		case "--version", "-v":
			fmt.Printf("GopherStack Enterprise v%s\n", version)
			os.Exit(0)
		case "--help", "-h":
			printHelp()
			os.Exit(0)
		default:
			if !strings.HasPrefix(arg, "-") && i == 0 {
				command = arg
			}
		}
	}

	// Load configuration
	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Execute command
	switch command {
	case "start":
		runForeground(cfg)
	case "install":
		runInstall(cfg)
	case "uninstall":
		runUninstall(cfg)
	case "service-start":
		runServiceStart(cfg)
	case "service-stop":
		runServiceStop(cfg)
	case "run-service":
		runAsService(cfg)
	case "status":
		runStatus(cfg)
	case "version":
		fmt.Printf("GopherStack Enterprise v%s\n", version)
	default:
		fmt.Printf("Unknown command: %s\n", command)
		printHelp()
		os.Exit(1)
	}
}

func runForeground(cfg *config.Config) {
	orch := orchestrator.New(cfg)

	// Setup signal handling
	orchestrator.HandleSignals(orch)

	// Start the orchestrator
	if err := orch.Start(); err != nil {
		log.Fatalf("Failed to start GopherStack: %v", err)
	}

	// Wait for stop signal
	orch.Wait()
}

func runInstall(cfg *config.Config) {
	fmt.Println("Installing GopherStack as Windows Service...")
	if err := service.Install(cfg); err != nil {
		log.Fatalf("Failed to install service: %v", err)
	}
	fmt.Println("✓ Service installed successfully!")
	fmt.Println("  Run 'gopherstack service-start' to start the service")
}

func runUninstall(cfg *config.Config) {
	fmt.Println("Uninstalling GopherStack Windows Service...")
	if err := service.Uninstall(cfg); err != nil {
		log.Fatalf("Failed to uninstall service: %v", err)
	}
	fmt.Println("✓ Service uninstalled successfully!")
}

func runServiceStart(cfg *config.Config) {
	fmt.Println("Starting GopherStack service...")
	if err := service.ServiceStart(cfg); err != nil {
		log.Fatalf("Failed to start service: %v", err)
	}
	fmt.Println("✓ Service started!")
}

func runServiceStop(cfg *config.Config) {
	fmt.Println("Stopping GopherStack service...")
	if err := service.ServiceStop(cfg); err != nil {
		log.Fatalf("Failed to stop service: %v", err)
	}
	fmt.Println("✓ Service stopped!")
}

func runAsService(cfg *config.Config) {
	if err := service.RunAsService(cfg); err != nil {
		log.Fatalf("Service error: %v", err)
	}
}

func runStatus(cfg *config.Config) {
	fmt.Printf("GopherStack Enterprise v%s\n", version)
	fmt.Printf("Dashboard: http://localhost:%d\n", cfg.DashboardPort)
	fmt.Printf("Nginx Port: %d\n", cfg.NginxPort)
	fmt.Printf("Workers: %d (ports %d-%d)\n", cfg.WorkerCount, cfg.BasePort, cfg.BasePort+cfg.WorkerCount-1)
	fmt.Printf("Document Root: %s\n", cfg.DocumentRoot)
}

func printHelp() {
	fmt.Println(`
╔══════════════════════════════════════════════════╗
║      GopherStack Enterprise v` + version + `              ║
║      High-Concurrency PHP Orchestrator           ║
╚══════════════════════════════════════════════════╝

USAGE:
  gopherstack [command] [options]

COMMANDS:
  start           Start GopherStack in foreground mode (default)
  install         Install as Windows Service
  uninstall       Uninstall Windows Service
  service-start   Start the Windows Service
  service-stop    Stop the Windows Service
  run-service     Run as Windows Service (internal)
  status          Show configuration status
  version         Show version

OPTIONS:
  --config, -c    Path to configuration file
  --version, -v   Show version
  --help, -h      Show this help

ENVIRONMENT VARIABLES:
  GOPHERSTACK_WORKERS        Number of PHP workers
  GOPHERSTACK_BASE_PORT      Starting port for workers
  GOPHERSTACK_NGINX_PORT     Nginx listen port
  GOPHERSTACK_DASHBOARD_PORT Dashboard port
  GOPHERSTACK_DOCUMENT_ROOT  PHP document root
  GOPHERSTACK_PHP_PATH       Path to php-cgi.exe
  GOPHERSTACK_NGINX_PATH     Path to nginx.exe
`)
}
