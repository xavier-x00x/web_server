package proxy

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"gopherstack/internal/pool"
)

// FCGIProxy proxies HTTP requests to PHP-CGI workers via FastCGI protocol
type FCGIProxy struct {
	poolManager  *pool.Manager
	documentRoot string
}

// NewFCGIProxy creates a new FastCGI proxy handler
func NewFCGIProxy(pm *pool.Manager, documentRoot string) *FCGIProxy {
	return &FCGIProxy{
		poolManager:  pm,
		documentRoot: documentRoot,
	}
}

// ServeHTTP handles incoming HTTP requests and proxies them to a PHP worker
func (p *FCGIProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	worker := p.poolManager.NextWorker()
	if worker == nil {
		http.Error(w, "No PHP workers available", http.StatusServiceUnavailable)
		return
	}

	// Determine the script to execute
	scriptName := r.URL.Path
	if scriptName == "/" {
		scriptName = "/index.php"
	}

	// If the path doesn't end in .php, try index.php
	if !strings.HasSuffix(scriptName, ".php") {
		scriptName = "/index.php"
	}

	scriptFilename := filepath.Join(p.documentRoot, filepath.FromSlash(scriptName))

	// Connect to the PHP-CGI worker
	addr := fmt.Sprintf("127.0.0.1:%d", worker.Port)
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		log.Printf("[proxy] Failed to connect to worker %d at %s: %v", worker.ID, addr, err)
		http.Error(w, "Failed to connect to PHP worker", http.StatusBadGateway)
		return
	}
	defer conn.Close()

	// Build FastCGI request
	fcgiReq := newFCGIRequest(r, scriptFilename, scriptName, p.documentRoot)

	// Send the FastCGI request
	if err := fcgiReq.writeTo(conn); err != nil {
		log.Printf("[proxy] Failed to send FastCGI request to worker %d: %v", worker.ID, err)
		http.Error(w, "Failed to send request to PHP worker", http.StatusBadGateway)
		return
	}

	// Read the FastCGI response
	if err := readFCGIResponse(conn, w); err != nil {
		log.Printf("[proxy] Failed to read FastCGI response from worker %d: %v", worker.ID, err)
		// Don't write error if we already started writing response
		return
	}

	worker.IncrementRequests()
}
