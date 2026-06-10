package pool

import (
	"log"
	"os/exec"
	"path/filepath"

	"gopherstack/internal/config"
)

// CleanupZombieProcesses kills any leftover GopherStack PHP processes
// from a previous crash or unclean shutdown. Prevents "port already in use"
// errors when workers try to bind to ports held by zombie processes.
// Note: Only kills gopher-php.exe (GopherStack's renamed binary) — does NOT
// touch php-cgi.exe so XAMPP and other PHP installations are unaffected.
// /T flag ensures child processes of zombies are also killed.
func CleanupZombieProcesses(cfg *config.Config) {
	names := make(map[string]bool)
	names["gopher-nginx.exe"] = true

	if cfg != nil {
		for _, site := range cfg.Sites {
			names[filepath.Base(site.PHPBinaryPath)] = true
		}
	} else {
		// Fallback for older configs
		names["gopher-php.exe"] = true
	}

	for name := range names {
		cmd := exec.Command("taskkill", "/f", "/im", name, "/t")
		if out, err := cmd.CombinedOutput(); err == nil {
			log.Printf("[pool] Cleaned up zombie %s processes (with children)", name)
			_ = out
		}
	}
}
