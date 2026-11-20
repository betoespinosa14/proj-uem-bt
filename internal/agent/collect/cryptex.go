package collect

import (
	"os"
	"path/filepath"
	"strings"

	"howett.net/plist"

	"github.com/uem/uem/internal/protocol"
)

// cryptexApps is where macOS 13+ keeps Safari: in a separately updated system
// volume ("cryptex"), with /Applications/Safari.app only a link to it.
// system_profiler doesn't list apps there, so without this Safari looked
// "not installed" (and third-party) on every Mac.
const cryptexApps = "/System/Cryptexes/App/System/Applications"

// addBundledApps appends the .app bundles in dir that aren't already in sw
// (matched by name, ignoring case), as Apple apps.
func addBundledApps(sw []protocol.Software, dir string) []protocol.Software {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return sw
	}
	have := map[string]bool{}
	for _, s := range sw {
		have[strings.ToLower(s.Name)] = true
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".app") {
			continue
		}
		var info struct {
			DisplayName string `plist:"CFBundleDisplayName"`
			Name        string `plist:"CFBundleName"`
			Version     string `plist:"CFBundleShortVersionString"`
		}
		if b, err := os.ReadFile(filepath.Join(dir, e.Name(), "Contents", "Info.plist")); err == nil {
			_, _ = plist.Unmarshal(b, &info)
		}
		name := strings.TrimSpace(info.DisplayName)
		if name == "" {
			name = strings.TrimSpace(info.Name)
		}
		if name == "" {
			name = strings.TrimSuffix(e.Name(), ".app")
		}
		if have[strings.ToLower(name)] {
			continue
		}
		have[strings.ToLower(name)] = true
		sw = append(sw, protocol.Software{
			Name: name, Version: info.Version, Publisher: "Software Signing", Source: "apple",
			InstallLocation: filepath.Join(dir, e.Name()),
		})
	}
	return sw
}
