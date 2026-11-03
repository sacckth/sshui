//go:build windows

package tui

import "os/exec"

func detachProcess(c *exec.Cmd) {}
