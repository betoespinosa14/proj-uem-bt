// Package update replaces the running agent binary with a newer build from the server.
//
// The new build is downloaded next to the agent, checked against the size and sha256 the
// server sent, and run once ("version") to prove it works on this machine. Only then is it
// swapped in; any failure leaves the current agent untouched. The caller restarts the
// service afterwards so the new binary takes over.
package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/uem/uem/internal/protocol"
)

var fileName = regexp.MustCompile(`^uem-agent-(linux|darwin|windows)-(amd64|arm64)(\.exe)?$`)

// Paths of the staged and the replaced binary for exe.
func newPath(exe string) string { return sibling(exe, "new") }
func oldPath(exe string) string { return sibling(exe, "old") }

func sibling(exe, tag string) string {
	if runtime.GOOS == "windows" {
		return strings.TrimSuffix(exe, ".exe") + "." + tag + ".exe"
	}
	return exe + "." + tag
}

// Install downloads p.File from serverURL and puts it in place of exe.
func Install(ctx context.Context, client *http.Client, exe, serverURL string, p protocol.UpdatePayload) error {
	if !fileName.MatchString(p.File) || len(p.SHA256) != 64 || p.Size <= 0 || p.Version == "" {
		return errors.New("invalid update details")
	}
	staged := newPath(exe)
	if err := download(ctx, client, strings.TrimRight(serverURL, "/")+"/downloads/"+p.File, staged, p); err != nil {
		os.Remove(staged)
		return err
	}
	if err := runsAs(ctx, staged, p.Version); err != nil {
		os.Remove(staged)
		return err
	}
	if err := swap(exe, staged); err != nil {
		os.Remove(staged)
		return fmt.Errorf("could not replace the agent: %w", err)
	}
	return nil
}

func download(ctx context.Context, client *http.Client, url, dst string, p protocol.UpdatePayload) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: %s", resp.Status)
	}
	f, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(resp.Body, p.Size+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	if n != p.Size || !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), p.SHA256) {
		return errors.New("download is damaged (size or checksum mismatch)")
	}
	return nil
}

// runsAs checks the staged build starts and reports the expected version.
func runsAs(ctx context.Context, bin, version string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "version").CombinedOutput()
	if err != nil || !strings.Contains(string(out), version) {
		return fmt.Errorf("the new agent did not run (%s)", strings.TrimSpace(string(out)+" "+errString(err)))
	}
	return nil
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// swap puts staged in place of exe. On Windows a running exe can't be overwritten but can
// be renamed, so the current one moves aside first (and comes back if the swap fails).
func swap(exe, staged string) error {
	if runtime.GOOS != "windows" {
		return os.Rename(staged, exe)
	}
	old := oldPath(exe)
	os.Remove(old)
	if err := os.Rename(exe, old); err != nil {
		return err
	}
	if err := os.Rename(staged, exe); err != nil {
		os.Rename(old, exe)
		return err
	}
	return nil
}

// Cleanup removes files left by an earlier update (a failed download, or the previous
// Windows binary once it is no longer running).
func Cleanup(exe string) {
	os.Remove(newPath(exe))
	os.Remove(oldPath(exe))
}
