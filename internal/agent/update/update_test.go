//go:build !windows

package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/uem/uem/internal/protocol"
)

// fakeBuild is a stand-in agent: a script that prints its version.
func fakeBuild(version string) []byte { return []byte("#!/bin/sh\necho " + version + "\n") }

func setup(t *testing.T, body []byte) (exe string, srv *httptest.Server, p protocol.UpdatePayload) {
	t.Helper()
	dir := t.TempDir()
	exe = filepath.Join(dir, "uem-agent")
	os.WriteFile(exe, []byte("current agent"), 0o755)
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/downloads/uem-agent-linux-amd64" {
			http.NotFound(w, r)
			return
		}
		w.Write(body)
	}))
	t.Cleanup(srv.Close)
	sum := sha256.Sum256(body)
	p = protocol.UpdatePayload{Version: "2026.11.9", File: "uem-agent-linux-amd64", SHA256: hex.EncodeToString(sum[:]), Size: int64(len(body))}
	return exe, srv, p
}

func current(t *testing.T, exe string) string {
	b, _ := os.ReadFile(exe)
	return string(b)
}

func TestInstallSwapsInAVerifiedBuild(t *testing.T) {
	body := fakeBuild("2026.11.9")
	exe, srv, p := setup(t, body)
	if err := Install(context.Background(), srv.Client(), exe, srv.URL+"/", p); err != nil {
		t.Fatal(err)
	}
	if current(t, exe) != string(body) {
		t.Fatal("agent was not replaced")
	}
	if _, err := os.Stat(newPath(exe)); !os.IsNotExist(err) {
		t.Error("staged file left behind")
	}
}

func TestInstallRejectsDamagedDownload(t *testing.T) {
	exe, srv, p := setup(t, fakeBuild("2026.11.9"))
	p.SHA256 = strings.Repeat("0", 64)
	err := Install(context.Background(), srv.Client(), exe, srv.URL, p)
	if err == nil || !strings.Contains(err.Error(), "damaged") {
		t.Fatalf("err = %v", err)
	}
	if current(t, exe) != "current agent" {
		t.Fatal("agent changed after a failed check")
	}
}

func TestInstallRejectsBuildThatReportsAnotherVersion(t *testing.T) {
	exe, srv, p := setup(t, fakeBuild("2026.11.8"))
	err := Install(context.Background(), srv.Client(), exe, srv.URL, p)
	if err == nil || !strings.Contains(err.Error(), "did not run") {
		t.Fatalf("err = %v", err)
	}
	if current(t, exe) != "current agent" {
		t.Fatal("agent changed after a failed check")
	}
}

func TestInstallRejectsUnsafeFileNames(t *testing.T) {
	exe, srv, p := setup(t, fakeBuild("2026.11.9"))
	p.File = "../../etc/passwd"
	if err := Install(context.Background(), srv.Client(), exe, srv.URL, p); err == nil {
		t.Fatal("accepted a path")
	}
}

func TestCleanupRemovesLeftovers(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "uem-agent")
	os.WriteFile(newPath(exe), nil, 0o644)
	os.WriteFile(oldPath(exe), nil, 0o644)
	os.WriteFile(sibling(exe, "old-20261110153000"), nil, 0o644)
	Cleanup(exe)
	for _, f := range []string{newPath(exe), oldPath(exe), sibling(exe, "old-20261110153000")} {
		if _, err := os.Stat(f); !os.IsNotExist(err) {
			t.Errorf("%s left behind", f)
		}
	}
}
