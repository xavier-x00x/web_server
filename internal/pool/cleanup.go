package pool

import (
	"log"
	"os/exec"
)

// cleanupZombieProcesses kills any leftover GopherStack PHP processes
// from a previous crash or unclean shutdown. Prevents "port already in use"
// errors when workers try to bind to ports held by zombie processes.
func (m *Manager) cleanupZombieProcesses() {
	names := []string{"gopher-php.exe", "php-cgi.exe"}
	for _, name := range names {
		cmd := exec.Command("taskkill", "/f", "/im", name)
		if out, err := cmd.CombinedOutput(); err == nil {
			log.Printf("[pool] Cleaned up zombie %s processes", name)
			_ = out
		}
	}
}
