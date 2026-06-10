//go:build linux

package nginx

import "syscall"

// nginxProcessAttr returns a SysProcAttr for Linux.
// On Linux, process group management doesn't need CREATION_FLAGS.
func nginxProcessAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{}
}
