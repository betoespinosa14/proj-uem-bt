package agentdist

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOutdated(t *testing.T) {
	for _, c := range []struct {
		cur, latest string
		want        bool
	}{
		{"2026.11.0", "2026.11.7", true},
		{"2026.11.3-dirty", "2026.11.7", true},
		{"2026.11.7", "2026.11.7", false},
		{"2026.11.7-dirty", "2026.11.7", true},
		{"2026.11.7-2-gabc", "2026.11.7", false},
		{"2026.12.0", "2026.11.7", false},
		{"2026.11.10", "2026.11.9", false},
		{"", "2026.11.7", true},
		{"2026.11.0", "", false},
	} {
		if got := Outdated(c.cur, c.latest); got != c.want {
			t.Errorf("Outdated(%q, %q) = %v, want %v", c.cur, c.latest, got, c.want)
		}
	}
}

func TestSupportsSelfUpdate(t *testing.T) {
	if SupportsSelfUpdate("2026.11.6") || SupportsSelfUpdate("2026.11.6-3-gabc-dirty") || SupportsSelfUpdate("0.0.0-dev") {
		t.Error("older agents must not get update_agent")
	}
	if !SupportsSelfUpdate("2026.11.7") || !SupportsSelfUpdate("2026.12.0") {
		t.Error("2026.11.7 and later support update_agent")
	}
}

func TestFileNameAndLoad(t *testing.T) {
	if FileName("macos", "arm64") != "uem-agent-darwin-arm64" || FileName("windows", "amd64") != "uem-agent-windows-amd64.exe" {
		t.Error("wrong file names")
	}
	dir := t.TempDir()
	if _, err := Load(dir); err == nil {
		t.Error("missing manifest should fail")
	}
	os.WriteFile(filepath.Join(dir, ManifestName), []byte(`{"version":"2026.11.7","files":{"uem-agent-linux-amd64":{"sha256":"ab","size":3}}}`), 0o644)
	m, err := Load(dir)
	if err != nil || m.Version != "2026.11.7" || m.Files["uem-agent-linux-amd64"].Size != 3 {
		t.Fatalf("load: %+v %v", m, err)
	}
}
