package pool

import (
	"log"
	"os/exec"
)

// CleanupZombieProcesses kills any leftover GopherStack PHP processes
// from a previous crash or unclean shutdown. Prevents "port already in use"
// errors when workers try to bind to ports held by zombie processes.
// Note: Only kills gopher-php.exe (GopherStack's renamed binary) — does NOT
// touch php-cgi.exe so XAMPP and other PHP installations are unaffected.
// /T flag ensures child processes of zombies are also killed.
func (m *Manager) CleanupZombieProcesses() {
	names := []string{"gopher-php.exe", "gopher-nginx.exe"}
	for _, name := range names {
		cmd := exec.Command("taskkill", "/f", "/im", name, "/t")
		if out, err := cmd.CombinedOutput(); err == nil {
			log.Printf("[pool] Cleaned up zombie %s processes (with children)", name)
			_ = out
		}
	}
}
