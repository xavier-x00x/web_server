package orchestrator

import (
	"fmt"
	"log"
	"net"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"gopherstack/internal/config"
)

// PortInfo holds information about a port check result
type PortInfo struct {
	Port    int
	Name    string
	InUse   bool
	Process string // Empty if not in use
}

// CheckPorts verifies that all required ports are available before starting.
// Returns a list of port statuses and any conflicts found.
func CheckPorts(cfg *config.Config) []PortInfo {
	ports := []struct {
		Port int
		Name string
	}{
		{cfg.NginxPort, "nginx"},
		{cfg.DashboardPort, "dashboard"},
	}

	// Add worker ports
	for i := 0; i < cfg.WorkerCount; i++ {
		port := cfg.BasePort + i
		ports = append(ports, struct {
			Port int
			Name string
		}{port, fmt.Sprintf("worker_%d", i)})
	}

	results := make([]PortInfo, 0, len(ports))
	hasConflict := false

	for _, p := range ports {
		info := PortInfo{
			Port: p.Port,
			Name: p.Name,
		}

		if !isPortAvailable(p.Port) {
			info.InUse = true
			info.Process = findProcessOnPort(p.Port)
			hasConflict = true
		}

		results = append(results, info)
	}

	if hasConflict {
		log.Println("[portcheck] ⚠️  Port conflicts detected!")
		for _, r := range results {
			if r.InUse {
				log.Printf("[portcheck]   - Port %d (%s): IN USE by %s", r.Port, r.Name, r.Process)
			}
		}
		log.Println("[portcheck] Try stopping the other program, or change ports in config/gopherstack.json")
	} else {
		log.Println("[portcheck] ✅ All ports are available")
	}

	return results
}

// isPortAvailable checks if a TCP port is available by attempting to listen on it
func isPortAvailable(port int) bool {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return false
	}
	ln.Close()
	return true
}

// findProcessOnPort tries to identify which process is using a port
// Returns a descriptive string like "nginx.exe (PID 1234)" or "Unknown"
func findProcessOnPort(port int) string {
	if runtime.GOOS != "windows" {
		return "Unknown"
	}

	// Use netstat to find the process on Windows
	cmd := exec.Command("netstat", "-ano")
	out, err := cmd.Output()
	if err != nil {
		return "Unknown"
	}

	portStr := fmt.Sprintf(":%d", port)
	var pid string

	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		// Look for LISTENING entries with our port
		if strings.Contains(line, portStr) && strings.Contains(line, "LISTENING") {
			fields := strings.Fields(line)
			if len(fields) >= 5 {
				pid = fields[len(fields)-1]
				break
			}
		}
	}

	if pid == "" {
		return "Unknown"
	}

	// Try to get the process name from PID
	tasklist := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %s", pid), "/NH")
	tlOut, err := tasklist.Output()
	if err != nil {
		return fmt.Sprintf("PID %s", pid)
	}

	tlLines := strings.Split(string(tlOut), "\n")
	for _, line := range tlLines {
		line = strings.TrimSpace(line)
		if line != "" && !strings.Contains(line, "Image Name") && !strings.Contains(line, "===") && !strings.Contains(line, "PID") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				name := fields[0]
				if strings.HasSuffix(name, ".exe") {
					return fmt.Sprintf("%s (PID %s)", name, pid)
				}
			}
		}
	}

	return fmt.Sprintf("PID %s", pid)
}

// WaitForPort waits until a port becomes available, with a timeout
func WaitForPort(port int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if isPortAvailable(port) {
			return true
		}
		time.Sleep(500 * time.Millisecond)
	}
	return false
}
