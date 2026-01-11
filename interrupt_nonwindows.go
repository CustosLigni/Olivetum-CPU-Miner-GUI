//go:build !windows

package main

import (
	"os"
)

func sendProcessInterrupt(proc *os.Process) error {
	return proc.Signal(os.Interrupt)
}
