package orchestrator

import (
	"log"
	"os"
	"os/signal"
	"syscall"
)

// HandleSignals sets up OS signal handling for graceful shutdown
func HandleSignals(orch *Orchestrator) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		log.Printf("[signals] Received signal: %v", sig)
		orch.Stop()
		orch.Signal()
	}()
}
