package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	_ "embed"
)

var (
	//go:embed assets/olivetum_pow_genesis.json
	embeddedGenesisJSON []byte
)

func ensureGenesisFile() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, configDirName, "olivetum_pow_genesis.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if existing, err := os.ReadFile(path); err == nil && bytes.Equal(existing, embeddedGenesisJSON) {
		return path, nil
	}
	if err := os.WriteFile(path, embeddedGenesisJSON, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func findGeth() (string, error) {
	names := []string{"geth"}
	if runtime.GOOS == "windows" {
		names = []string{"geth.exe", "geth"}
	}
	exe, err := os.Executable()
	if err == nil {
		dir := filepath.Dir(exe)
		for _, name := range names {
			candidate := filepath.Join(dir, name)
			if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
				return candidate, nil
			}
		}
	}
	for _, name := range names {
		p, err := exec.LookPath(name)
		if err == nil {
			return p, nil
		}
	}
	return "", errors.New("geth not found")
}

func isGethInitialized(dataDir string) bool {
	dataDir = strings.TrimSpace(dataDir)
	if dataDir == "" {
		return false
	}
	st, err := os.Stat(filepath.Join(dataDir, "geth", "chaindata"))
	return err == nil && st.IsDir()
}

func runGethInit(gethPath, dataDir, genesisPath string) (string, error) {
	cmd := exec.Command(gethPath, "--datadir", dataDir, "init", genesisPath)
	configureChildProcess(cmd)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	out, err := cmd.CombinedOutput()
	outStr := strings.TrimSpace(string(out))
	if err != nil {
		if outStr == "" {
			return "", fmt.Errorf("geth init failed: %w", err)
		}
		return outStr, fmt.Errorf("geth init failed: %w", err)
	}
	return outStr, nil
}

func wipeNodeData(dataDir string) error {
	dataDir = strings.TrimSpace(dataDir)
	if dataDir == "" {
		return errors.New("node data directory is required")
	}
	var err error
	dataDir, err = expandUserPath(dataDir)
	if err != nil {
		return err
	}
	if dataDir == "" {
		return errors.New("node data directory is required")
	}
	gethDir := filepath.Join(dataDir, "geth")
	if err := os.RemoveAll(gethDir); err != nil {
		return err
	}
	_ = os.Remove(filepath.Join(dataDir, "geth.ipc"))
	return nil
}
