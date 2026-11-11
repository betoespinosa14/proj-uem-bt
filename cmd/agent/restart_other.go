//go:build !linux

package main

// launchd and the Windows service manager already restart the agent within
// seconds; only systemd needs the drop-in.
func ensureFastRestart() {}
func removeFastRestart() {}
