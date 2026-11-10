// Package agentdist describes the agent builds the server offers for download:
// which version they are and the checksum of each file. `make build-agents`
// writes dist/manifest.json; the server reads it on each request, so a new
// build in dist/ is offered without restarting the server.
package agentdist

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ManifestName is the manifest's file name inside the dist directory.
const ManifestName = "manifest.json"

// SelfUpdateSince is the first agent release that understands the update_agent command.
// Older agents are updated once by a script instead (see BootstrapScript).
const SelfUpdateSince = "2026.11.7"

// File is one agent binary.
type File struct {
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// Manifest lists the agent builds in the dist directory.
type Manifest struct {
	Version string          `json:"version"`
	Files   map[string]File `json:"files"`
}

// Load reads the manifest from dir.
func Load(dir string) (Manifest, error) {
	var m Manifest
	if dir == "" {
		return m, fmt.Errorf("no agent builds configured")
	}
	b, err := os.ReadFile(filepath.Join(dir, ManifestName))
	if err != nil {
		return m, fmt.Errorf("no agent build manifest: %w", err)
	}
	if err := json.Unmarshal(b, &m); err != nil || m.Version == "" {
		return Manifest{}, fmt.Errorf("invalid agent build manifest")
	}
	return m, nil
}

// FileName is the download name for a platform, from the device's os_family and arch.
func FileName(osFamily, arch string) string {
	goos := osFamily
	if osFamily == "macos" {
		goos = "darwin"
	}
	name := "uem-agent-" + goos + "-" + arch
	if goos == "windows" {
		name += ".exe"
	}
	return name
}

// Base strips build metadata from a version: "2026.11.6-3-gabc-dirty" -> "2026.11.6".
func Base(v string) string {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexByte(v, '-'); i >= 0 {
		v = v[:i]
	}
	return v
}

// Compare compares the release parts of two versions numerically (-1, 0, 1).
func Compare(a, b string) int {
	as, bs := strings.Split(Base(a), "."), strings.Split(Base(b), ".")
	for i := 0; i < len(as) || i < len(bs); i++ {
		x, y := 0, 0
		if i < len(as) {
			x, _ = strconv.Atoi(as[i])
		}
		if i < len(bs) {
			y, _ = strconv.Atoi(bs[i])
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

// Outdated reports whether an agent at version current is behind latest. A build between
// releases ("2026.11.6-3-gabc") counts as its release; an uncommitted build of the same
// release ("2026.11.6-dirty") counts as behind it, so it can be replaced by the real one.
func Outdated(current, latest string) bool {
	if latest == "" || current == latest {
		return false
	}
	switch c := Compare(current, latest); {
	case current == "" || c < 0:
		return true
	case c == 0:
		return strings.HasSuffix(current, "-dirty") && !strings.HasSuffix(latest, "-dirty")
	}
	return false
}

// SupportsSelfUpdate reports whether an agent at version v understands update_agent.
func SupportsSelfUpdate(v string) bool {
	return Compare(v, SelfUpdateSince) >= 0
}

// Write records the version and checksum of every agent binary in dir into its manifest.
func Write(dir, version string) (Manifest, error) {
	m := Manifest{Version: version, Files: map[string]File{}}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return m, err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "uem-agent-") {
			continue
		}
		f, err := hashFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return m, err
		}
		m.Files[e.Name()] = f
	}
	b, _ := json.MarshalIndent(m, "", "  ")
	tmp := filepath.Join(dir, "."+ManifestName+".tmp")
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return m, err
	}
	return m, os.Rename(tmp, filepath.Join(dir, ManifestName))
}

func hashFile(path string) (File, error) {
	f, err := os.Open(path)
	if err != nil {
		return File{}, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return File{}, err
	}
	return File{SHA256: hex.EncodeToString(h.Sum(nil)), Size: n}, nil
}
