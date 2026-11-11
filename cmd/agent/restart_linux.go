package main

import (
	"log/slog"
	"os"
	"os/exec"
)

func usesSystemd() bool {
	_, err := os.Stat("/run/systemd/system")
	return err == nil
}

// ensureFastRestart installs the restart drop-in and reloads systemd when it
// changed. Failures are logged; the agent works either way.
func ensureFastRestart() {
	if !usesSystemd() {
		return
	}
	changed, err := ensureDropIn(systemdDropInDir)
	if err != nil {
		slog.Warn("could not shorten the systemd restart delay", "err", err)
		return
	}
	if changed {
		daemonReload()
	}
}

// removeFastRestart undoes ensureFastRestart, for uninstalling.
func removeFastRestart() {
	if usesSystemd() && removeDropIn(systemdDropInDir) {
		daemonReload()
	}
}

func daemonReload() {
	if out, err := exec.Command("systemctl", "daemon-reload").CombinedOutput(); err != nil {
		slog.Warn("systemctl daemon-reload failed", "err", err, "out", string(out))
	}
}
