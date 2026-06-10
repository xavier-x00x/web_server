//go:build linux

package orchestrator

import "log"

// registerConsoleHandler is a no-op on Linux.
// On Windows, this registers a handler for console close/logoff/shutdown events
// via the Windows SetConsoleCtrlHandler API.
func registerConsoleHandler(orch *Orchestrator) {
	log.Println("[signals] Linux build: console control handler not needed")
}
