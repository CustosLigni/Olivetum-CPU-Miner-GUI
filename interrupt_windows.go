//go:build windows

package main

import (
	"os"
	"time"

	"golang.org/x/sys/windows"
)

func sendProcessInterrupt(proc *os.Process) error {
	if proc == nil {
		return nil
	}
	ensureHiddenConsole()

	pid := uint32(proc.Pid)
	var lastErr error

	// Some Go programs react to CTRL_C_EVENT, some to CTRL_BREAK_EVENT.
	// Try both, best-effort.
	if err := windows.GenerateConsoleCtrlEvent(windows.CTRL_C_EVENT, pid); err != nil {
		lastErr = err
	} else {
		lastErr = nil
	}
	time.Sleep(25 * time.Millisecond)
	if err := windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, pid); err != nil {
		if lastErr == nil {
			lastErr = err
		}
	} else {
		lastErr = nil
	}
	if lastErr == nil {
		return nil
	}
	return proc.Signal(os.Interrupt)
}
