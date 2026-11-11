package main

import (
	"bytes"
	"os"
	"path/filepath"
)

// The systemd unit written by kardianos/service hard-codes RestartSec=120, so
// after an update (the agent exits for systemd to start the new binary) or a
// crash, a Linux device stayed offline for two minutes. A drop-in next to the
// unit shortens the delay; the agent keeps it in place itself, so devices pick
// it up through a normal update.

const (
	systemdDropInDir = "/etc/systemd/system/uem-agent.service.d"
	dropInName       = "10-beacon-restart.conf"
	dropInContent    = "# Written by the BEACON agent: restart within seconds after an update or a crash.\n[Service]\nRestartSec=5\n"
)

// ensureDropIn writes the restart drop-in into dir unless it is already there
// with the right content, and reports whether it changed anything.
func ensureDropIn(dir string) (bool, error) {
	path := filepath.Join(dir, dropInName)
	if cur, err := os.ReadFile(path); err == nil && bytes.Equal(cur, []byte(dropInContent)) {
		return false, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false, err
	}
	if err := os.WriteFile(path, []byte(dropInContent), 0o644); err != nil {
		return false, err
	}
	return true, nil
}

// removeDropIn deletes the drop-in, and dir too when nothing else is in it.
// It reports whether the drop-in existed.
func removeDropIn(dir string) bool {
	err := os.Remove(filepath.Join(dir, dropInName))
	os.Remove(dir) // fails harmlessly when other drop-ins remain
	return err == nil
}
