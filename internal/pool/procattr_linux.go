//go:build linux

package pool

import "syscall"

// newProcessAttr returns a SysProcAttr for Linux.
// On Linux we don't need CREATE_NEW_PROCESS_GROUP (Windows-specific);
// process group management is handled differently.
func newProcessAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{}
}
