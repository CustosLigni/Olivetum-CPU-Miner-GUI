//go:build windows

package main

import (
	"sync"
	"syscall"
)

var (
	kernel32             = syscall.NewLazyDLL("kernel32.dll")
	procAllocConsole     = kernel32.NewProc("AllocConsole")
	procGetConsoleWindow = kernel32.NewProc("GetConsoleWindow")
	user32               = syscall.NewLazyDLL("user32.dll")
	procShowWindow       = user32.NewProc("ShowWindow")
	hiddenConsoleOnce    sync.Once
)

func ensureHiddenConsole() {
	hiddenConsoleOnce.Do(func() {
		hwnd := consoleWindow()
		if hwnd == 0 {
			_, _, _ = procAllocConsole.Call()
			hwnd = consoleWindow()
		}
		if hwnd != 0 {
			const swHide = 0
			_, _, _ = procShowWindow.Call(hwnd, swHide)
		}
	})
}

func consoleWindow() uintptr {
	hwnd, _, _ := procGetConsoleWindow.Call()
	return hwnd
}
