package main

import (
	"os/exec"
	"runtime"
	"strings"
)

type timeSyncStatus struct {
	Known        bool
	Synchronized bool
}

func checkSystemTimeSync() timeSyncStatus {
	switch runtime.GOOS {
	case "linux":
		out, err := exec.Command("timedatectl", "show", "-p", "NTPSynchronized", "--value").Output()
		if err != nil {
			return timeSyncStatus{}
		}
		v := strings.TrimSpace(string(out))
		if v == "yes" {
			return timeSyncStatus{Known: true, Synchronized: true}
		}
		if v == "no" {
			return timeSyncStatus{Known: true, Synchronized: false}
		}
		return timeSyncStatus{}

	case "windows":
		out, err := exec.Command("w32tm", "/query", "/status").Output()
		if err != nil {
			return timeSyncStatus{}
		}
		source := ""
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(strings.ToLower(line), "source:") {
				source = strings.TrimSpace(line[len("source:"):])
				break
			}
		}
		if source == "" {
			return timeSyncStatus{Known: true, Synchronized: false}
		}
		srcLower := strings.ToLower(source)
		if strings.Contains(srcLower, "local cmos") || strings.Contains(srcLower, "free-running") {
			return timeSyncStatus{Known: true, Synchronized: false}
		}
		return timeSyncStatus{Known: true, Synchronized: true}
	}
	return timeSyncStatus{}
}
