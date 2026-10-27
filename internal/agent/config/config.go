// Package config persists the agent's enrolled identity.
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
)

type Config struct {
	ServerURL    string `json:"server_url"`
	DeviceID     string `json:"device_id"`
	DeviceSecret string `json:"device_secret"`
}

var ErrNotEnrolled = errors.New("agent is not enrolled; run `uem-agent enroll --server URL --token TOKEN`")

// DefaultPath is the per-OS system location for the config file;
// UEM_AGENT_CONFIG overrides it.
func DefaultPath() string {
	if p := os.Getenv("UEM_AGENT_CONFIG"); p != "" {
		return p
	}
	switch runtime.GOOS {
	case "windows":
		base := os.Getenv("ProgramData")
		if base == "" {
			base = `C:\ProgramData`
		}
		return filepath.Join(base, "UEM Agent", "agent.json")
	case "darwin":
		return "/Library/Application Support/UEM Agent/agent.json"
	default:
		return "/etc/uem-agent/agent.json"
	}
}

func Load(path string) (Config, error) {
	var c Config
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return c, ErrNotEnrolled
	}
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return c, err
	}
	if c.DeviceID == "" || c.DeviceSecret == "" || c.ServerURL == "" {
		return c, ErrNotEnrolled
	}
	return c, nil
}

// Save writes the config readable only by its owner (root / SYSTEM), since
// it contains the device secret.
func Save(path string, c Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
