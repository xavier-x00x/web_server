//go:build windows

package pool

import "syscall"

// newProcessAttr returns a SysProcAttr configured for Windows process groups.
// This allows killing the entire process tree (worker + any child processes).
func newProcessAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}
