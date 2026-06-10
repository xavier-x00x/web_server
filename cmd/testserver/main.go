package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const version = "1.0.0"

func main() {
	port := "8088"
	if p := os.Getenv("PORT"); p != "" {
		port = p
	}
	dashPort := "8090"
	if p := os.Getenv("DASHBOARD_PORT"); p != "" {
		dashPort = p
	}

	// Serve static files from www/
	wwwDir := "www"
	if d := os.Getenv("WWW_DIR"); d != "" {
		wwwDir = d
	}
	os.MkdirAll(wwwDir, 0755)

	// ── App Server (port 8088) ──
	appMux := http.NewServeMux()

	// Main web
	appMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `<!DOCTYPE html><html><head><title>GopherStack Test</title></head>
<body><h1>GopherStack Enterprise</h1><p>High-Concurrency PHP Orchestrator</p>
<ul><li><a href="/index.php">PHP Info</a></li><li><a href="/test.php">Test</a></li></ul></body></html>`)
	})

	// Simulate PHP endpoints
	appMux.HandleFunc("/index.php", phpHandler("PHP Info", "8.1.10"))
	appMux.HandleFunc("/info.php", phpHandler("PHP Configuration", "8.1.10"))
	appMux.HandleFunc("/test.php", phpHandler("Test Page", "8.1.10"))

	// Static files from www/
	appMux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(wwwDir))))

	// Health endpoint
	appMux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "ok",
			"version": version,
			"time":    time.Now().Unix(),
		})
	})

	// ── Dashboard Server (port 8090) ──
	dashMux := http.NewServeMux()

	// API Status
	dashMux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":          "running",
			"version":         version,
			"uptime_seconds":  int(time.Now().Sub(startTime).Seconds()),
			"nginx_running":   true,
			"php_version":     "8.1.10",
			"active_workers":  4,
			"total_workers":   4,
			"requests_served": 1337,
		})
	})

	// API Workers
	dashMux.HandleFunc("/api/workers", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		workers := make([]map[string]interface{}, 4)
		for i := 0; i < 4; i++ {
			port := 9001 + i
			workers[i] = map[string]interface{}{
				"id":           i,
				"port":         port,
				"pid":          4000 + i,
				"status":       "running",
				"requests":     100 + rand.Intn(900),
				"restarts":     rand.Intn(3),
				"memory_mb":    12 + rand.Intn(8),
				"cpu_percent":  2.0 + rand.Float64()*10,
				"uptime_secs":  int(time.Now().Sub(startTime).Seconds()),
				"last_restart": time.Now().Add(-time.Duration(rand.Intn(3600)) * time.Second).Unix(),
			}
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"workers": workers,
			"total":   4,
			"active":  4,
			"idle":    0,
		})
	})

	// API Metrics
	dashMux.HandleFunc("/api/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"requests_per_second": 142.5,
			"average_latency_ms":  45.2,
			"p95_latency_ms":      120.0,
			"p99_latency_ms":      250.0,
			"active_connections":  8,
			"total_requests":      133700,
			"error_rate":          0.02,
			"memory_used_mb":      256,
			"memory_total_mb":     1024,
			"cpu_usage_percent":   35.5,
			"uptime_seconds":      int(time.Now().Sub(startTime).Seconds()),
			"worker_metrics": map[string]interface{}{
				"active":  4,
				"idle":    0,
				"dead":    0,
				"total":   4,
				"avg_cpu": 5.2,
				"avg_mem": 16,
			},
		})
	})

	// Dashboard UI
	dashMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `<!DOCTYPE html>
<html><head><title>GopherStack Dashboard</title>
<style>body{font-family:sans-serif;margin:40px;background:#1a1a2e;color:#eee;}
h1{color:#e94560}.card{background:#16213e;border-radius:8px;padding:20px;margin:10px 0}
.grid{display:grid;grid-template-columns:repeat(3,1fr);gap:20px}
.stat{font-size:2em;font-weight:bold;color:#0f3460}
.label{font-size:0.9em;color:#aaa}
pre{background:#0f3460;padding:10px;border-radius:4px;overflow-x:auto}
</style></head>
<body><h1>🚀 GopherStack Dashboard</h1>
<div class="grid"><div class="card"><div class="stat">4</div><div class="label">Active Workers</div></div>
<div class="card"><div class="stat">142.5</div><div class="label">RPS</div></div>
<div class="card"><div class="stat">45ms</div><div class="label">Avg Latency</div></div></div>
<div class="card"><h2>API Endpoints</h2>
<a href="/api/status">/api/status</a><br>
<a href="/api/workers">/api/workers</a><br>
<a href="/api/metrics">/api/metrics</a></div>
<p><small>GopherStack Enterprise v` + version + ` — Test Mode</small></p>
</body></html>`)
	})

	// ── Start servers ──
	go func() {
		log.Printf("[app] Listening on :%s", port)
		if err := http.ListenAndServe(":"+port, appMux); err != nil {
			log.Fatalf("[app] Failed: %v", err)
		}
	}()

	go func() {
		log.Printf("[dashboard] Listening on :%s", dashPort)
		if err := http.ListenAndServe(":"+dashPort, dashMux); err != nil {
			log.Fatalf("[dashboard] Failed: %v", err)
		}
	}()

	log.Printf("╔══════════════════════════════════════════╗")
	log.Printf("║  GopherStack Test Server                 ║")
	log.Printf("║  App:   http://localhost:%s             ║", port)
	log.Printf("║  Dash:  http://localhost:%s            ║", dashPort)
	log.Printf("╚══════════════════════════════════════════╝")

	// Wait for signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigChan
	log.Printf("Received %v, shutting down...", sig)
}

var startTime = time.Now()

func phpHandler(title, phpVersion string) http.HandlerFunc {
	phpSnippet := fmt.Sprintf(`<?php
phpinfo();
// GopherStack Enterprise Test Server
// PHP Version: %s
`, phpVersion)

	return func(w http.ResponseWriter, r *http.Request) {
		// Simulate PHP processing delay
		delay := time.Duration(10+rand.Intn(40)) * time.Millisecond
		time.Sleep(delay)

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `<!DOCTYPE html><html><head><title>%s</title></head>
<body><h1>%s</h1><pre>%s</pre>
<p><small>GopherStack Test Server | Version: %s | Processed in %.0fms</small></p>
</body></html>`, title, title, phpSnippet, version, delay.Seconds()*1000)
	}
}
