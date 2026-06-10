//go:build windows

package nginx

import "syscall"

// nginxProcessAttr returns a SysProcAttr configured for Windows.
// CREATE_NEW_PROCESS_GROUP allows killing the process tree safely.
func nginxProcessAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}
