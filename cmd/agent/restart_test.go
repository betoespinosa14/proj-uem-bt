package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDropIn(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "uem-agent.service.d")
	path := filepath.Join(dir, dropInName)

	if changed, err := ensureDropIn(dir); err != nil || !changed {
		t.Fatalf("first ensure: changed=%v err=%v, want true, nil", changed, err)
	}
	if b, _ := os.ReadFile(path); string(b) != dropInContent {
		t.Fatalf("content = %q", b)
	}
	if changed, err := ensureDropIn(dir); err != nil || changed {
		t.Fatalf("second ensure: changed=%v err=%v, want false, nil", changed, err)
	}
	if err := os.WriteFile(path, []byte("[Service]\nRestartSec=120\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if changed, _ := ensureDropIn(dir); !changed {
		t.Fatal("stale drop-in was not rewritten")
	}
	if !removeDropIn(dir) {
		t.Fatal("remove reported no drop-in")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("empty drop-in dir left behind: %v", err)
	}
	if removeDropIn(dir) {
		t.Fatal("second remove reported a drop-in")
	}
}
