package orchestrator

import (
	"log"
	"os"
	"syscall"
	"unsafe"
)

var (
	kernel32                  = syscall.NewLazyDLL("kernel32.dll")
	procSetConsoleCtrlHandler = kernel32.NewProc("SetConsoleCtrlHandler")
)

// Windows console control event types
const (
	CTRL_CLOSE_EVENT    = 2
	CTRL_LOGOFF_EVENT   = 5
	CTRL_SHUTDOWN_EVENT = 6
	CREATE_NO_WINDOW    = 0x08000000
)

// registerConsoleHandler registers a Windows console control handler
// to ensure nginx and PHP processes are killed when the console window
// is closed (X button), or on user logoff/system shutdown.
func registerConsoleHandler(orch *Orchestrator) {
	// Create a callback for SetConsoleCtrlHandler
	handler := func(ctrlType uint) uint {
		switch ctrlType {
		case CTRL_CLOSE_EVENT:
			log.Println("[signals] Console window is closing...")
			cleanupAndExit(orch)
			return 1 // Handled
		case CTRL_LOGOFF_EVENT:
			log.Println("[signals] User logoff detected...")
			cleanupAndExit(orch)
			return 1
		case CTRL_SHUTDOWN_EVENT:
			log.Println("[signals] System shutdown detected...")
			cleanupAndExit(orch)
			return 1
		}
		return 0 // Not handled, let default handler process it
	}

	// Register the handler using SetConsoleCtrlHandler Windows API
	cb := syscall.NewCallback(func(ctrlType uintptr) uintptr {
		return uintptr(handler(uint(ctrlType)))
	})

	ret, _, err := procSetConsoleCtrlHandler.Call(cb, 1)
	if ret == 0 {
		log.Printf("[signals] Warning: Failed to register console control handler: %v", err)
	} else {
		log.Println("[signals] Windows console control handler registered")
	}
}

// cleanupAndExit performs graceful shutdown of all services and exits
func cleanupAndExit(orch *Orchestrator) {
	log.Println("[signals] Cleaning up all child processes (nginx, PHP)...")

	// Stop the orchestrator which stops nginx and PHP workers
	orch.Stop()

	// Also force-kill any remaining nginx/php-cgi processes as a safety net
	forceKillRemainingProcesses()

	log.Println("[signals] All services stopped. Exiting.")
	orch.Signal()
	os.Exit(0)
}

// forceKillRemainingProcesses kills any remaining nginx.exe and php-cgi.exe
// processes as a safety net in case graceful shutdown didn't catch them all.
// taskkill /f /IM gopher-php.exe
func forceKillRemainingProcesses() {
	processesToKill := []string{"nginx.exe", "gopher-php.exe"}

	for _, procName := range processesToKill {
		// Use taskkill to force-kill by process name
		// /F = force, /IM = image name, /T = kill child processes
		cmd := syscall.StringToUTF16Ptr("taskkill /F /IM " + procName + " /T")

		var si syscall.StartupInfo
		si.Cb = uint32(unsafe.Sizeof(si))
		si.Flags = syscall.STARTF_USESHOWWINDOW
		si.ShowWindow = 0 // SW_HIDE - don't show the taskkill window

		var pi syscall.ProcessInformation

		err := syscall.CreateProcess(
			nil,
			cmd,
			nil, nil, false,
			CREATE_NO_WINDOW,
			nil, nil,
			&si, &pi,
		)
		if err == nil {
			syscall.WaitForSingleObject(pi.Process, syscall.INFINITE)
			syscall.CloseHandle(pi.Process)
			syscall.CloseHandle(pi.Thread)
		}
	}
}
