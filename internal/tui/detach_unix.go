//go:build !windows

package tui

import (
	"os/exec"
	"syscall"
)

// detachProcess puts c in a new session so closing the terminal (SIGHUP) does not kill it.
func detachProcess(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
