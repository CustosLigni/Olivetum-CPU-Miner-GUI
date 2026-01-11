package main

import (
	"os"
	"path/filepath"
	"strings"
)

func expandUserPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", nil
	}
	if path == "~" || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if path == "~" {
			return home, nil
		}
		rest := strings.TrimLeft(path[2:], `/\`)
		return filepath.Join(home, filepath.FromSlash(rest)), nil
	}
	return path, nil
}
