package orchestrator

import (
	"log"
	"os"
	"os/signal"
	"syscall"
)

// HandleSignals sets up OS signal handling for graceful shutdown.
// On Windows, this handles SIGINT (Ctrl+C), SIGTERM, and also
// console close events (window close button, logoff, shutdown).
func HandleSignals(orch *Orchestrator) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)

	go func() {
		sig := <-sigChan
		log.Printf("[signals] Received signal: %v", sig)
		log.Println("[signals] Initiating graceful shutdown of all services...")
		orch.Stop()
		orch.Signal()
	}()

	// Register Windows console control handler for close/logoff/shutdown events
	registerConsoleHandler(orch)
}
