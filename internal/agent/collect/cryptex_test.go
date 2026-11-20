package collect

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/uem/uem/internal/protocol"
)

const safariPlist = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleName</key><string>Safari</string>
<key>CFBundleShortVersionString</key><string>26.1</string>
</dict></plist>`

func TestAddBundledApps(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{"Safari.app": safariPlist, "NoPlist.app": "", "Terminal.app": safariPlist} {
		c := filepath.Join(dir, name, "Contents")
		if err := os.MkdirAll(c, 0o755); err != nil {
			t.Fatal(err)
		}
		if body != "" && name != "Terminal.app" {
			if err := os.WriteFile(filepath.Join(c, "Info.plist"), []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	_ = os.WriteFile(filepath.Join(dir, "README"), nil, 0o644)

	// "terminal" is already listed by system_profiler; Safari isn't.
	got := addBundledApps([]protocol.Software{{Name: "Terminal", Source: "apple"}}, dir)
	byName := map[string]protocol.Software{}
	for _, s := range got {
		if _, dup := byName[s.Name]; dup {
			t.Fatalf("%s listed twice: %+v", s.Name, got)
		}
		byName[s.Name] = s
	}
	if len(got) != 3 {
		t.Fatalf("want Terminal, Safari, NoPlist; got %+v", got)
	}
	s := byName["Safari"]
	if s.Version != "26.1" || s.Source != "apple" || s.Publisher != "Software Signing" {
		t.Fatalf("safari: %+v", s)
	}
	if byName["NoPlist"].Source != "apple" {
		t.Fatalf("an app without Info.plist is named after its folder: %+v", got)
	}
	if got := addBundledApps(nil, filepath.Join(dir, "missing")); len(got) != 0 {
		t.Fatalf("missing folder: %+v", got)
	}
}

// On a Mac, the real folder holds Safari.
func TestCryptexAppsOnThisMac(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS only")
	}
	if _, err := os.Stat(cryptexApps); err != nil {
		t.Skip("no cryptex apps folder")
	}
	got := addBundledApps(nil, cryptexApps)
	for _, s := range got {
		t.Logf("%s %s", s.Name, s.Version)
		if s.Name == "Safari" && s.Version != "" {
			return
		}
	}
	t.Fatalf("Safari not found in %s: %+v", cryptexApps, got)
}
