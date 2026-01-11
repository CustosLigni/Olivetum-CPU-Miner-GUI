//go:build windows

package main

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

func configureChildProcess(cmd *exec.Cmd) {
	ensureHiddenConsole()
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	if consoleWindow() == 0 {
		cmd.SysProcAttr.CreationFlags |= windows.CREATE_NO_WINDOW
	}
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_NEW_PROCESS_GROUP
}
