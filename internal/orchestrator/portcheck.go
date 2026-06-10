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
		{cfg.DashboardPort, "dashboard"},
	}

	// Add site ports
	for _, site := range cfg.Sites {
		ports = append(ports, struct {
			Port int
			Name string
		}{site.NginxPort, fmt.Sprintf("nginx_%s", site.Name)})

		for i := 0; i < site.WorkerCount; i++ {
			port := site.BasePort + i
			ports = append(ports, struct {
				Port int
				Name string
			}{port, fmt.Sprintf("worker_%s_%d", site.Name, i)})
		}
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

func checkSiteWorkerConflicts(site config.SiteConfig) map[int]bool {
	conflicts := make(map[int]bool)
	for i := 0; i < site.WorkerCount; i++ {
		port := site.BasePort + i
		if !isPortAvailable(port) {
			conflicts[port] = true
		}
	}
	return conflicts
}

// FindAvailableRange finds the first block of n consecutive free ports
// starting from startPort. Returns the starting port of the free block.
// Searches up to 65535 - count.
func FindAvailableRange(startPort, count int) (int, error) {
	if startPort < 1024 {
		startPort = 1024
	}

	for port := startPort; port < 65535-count; port++ {
		// Quick check: is the first port available?
		if !isPortAvailable(port) {
			continue
		}

		// Check the full block
		allFree := true
		for i := 0; i < count; i++ {
			if !isPortAvailable(port + i) {
				allFree = false
				port += i // Skip ahead to the conflicted port
				break
			}
		}

		if allFree {
			return port, nil
		}
	}

	return 0, fmt.Errorf("could not find %d consecutive free ports above %d", count, startPort)
}

// ResolveWorkerPortConflicts checks for worker port conflicts and
// automatically shifts the worker port range to a conflict-free block for each site.
// Returns true if ports were shifted, false if no conflicts found.
func ResolveWorkerPortConflicts(cfg *config.Config) bool {
	shiftedAny := false

	for i, site := range cfg.Sites {
		conflicts := checkSiteWorkerConflicts(site)
		if len(conflicts) == 0 {
			continue // No conflicts for this site
		}

		log.Printf("[portcheck] ⚠️  Site %s: %d worker port(s) in use, searching for free range...", site.Name, len(conflicts))

		// Log which ports are conflicted
		for port := range conflicts {
			proc := findProcessOnPort(port)
			log.Printf("[portcheck]   - Site %s Port %d: IN USE by %s", site.Name, port, proc)
		}

		// Find a free range starting above the current base
		newBase, err := FindAvailableRange(site.BasePort+site.WorkerCount+100, site.WorkerCount)
		if err != nil {
			// Try wider range
			newBase, err = FindAvailableRange(10000, site.WorkerCount)
			if err != nil {
				log.Printf("[portcheck] ❌ Could not find free port range for site %s: %v", site.Name, err)
				log.Printf("[portcheck] Falling back to original ports — workers may fail to start")
				continue
			}
		}

		log.Printf("[portcheck] ✅ Found free port block for site %s: %d-%d (shifted from %d-%d)",
			site.Name, newBase, newBase+site.WorkerCount-1,
			site.BasePort, site.BasePort+site.WorkerCount-1)

		cfg.Sites[i].BasePort = newBase
		shiftedAny = true
	}

	return shiftedAny
}
