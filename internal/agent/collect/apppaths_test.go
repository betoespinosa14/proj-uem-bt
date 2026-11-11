package collect

import (
	"testing"

	"github.com/uem/uem/internal/protocol"
)

func TestFillFromAppPaths(t *testing.T) {
	sw := []protocol.Software{
		{Name: "Google Chrome"}, // MSI entry without a location
		{Name: "Microsoft Edge", InstallLocation: `C:\Program Files (x86)\Microsoft\Edge\Application`}, // kept
		{Name: "mozilla firefox"}, // case differs
		{Name: "Paint"},           // only a Windows-folder program matches
		{Name: "Unrelated Tool"},  // no matching program
	}
	progs := []appPath{
		{Exe: `"C:\Program Files\Google\Chrome\Application\chrome.exe"`, ProductName: "Google Chrome"},
		{Exe: `C:\Program Files\Other\msedge.exe`, ProductName: "Microsoft Edge"},
		{Exe: `C:\Program Files\Mozilla Firefox\firefox.exe`, ProductName: "Mozilla Firefox"},
		{Exe: `C:\Windows\System32\mspaint.exe`, ProductName: "Paint"},
		{Exe: `C:\Program Files\Tool\tool.exe`, ProductName: "Some Tool"},
		{Exe: `C:\Program Files\Blank\blank.exe`, ProductName: ""},
	}
	got := fillFromAppPaths(sw, progs)
	want := []string{
		`C:\Program Files\Google\Chrome\Application`,
		`C:\Program Files (x86)\Microsoft\Edge\Application`,
		`C:\Program Files\Mozilla Firefox`,
		"",
		"",
	}
	for i, w := range want {
		if got[i].InstallLocation != w {
			t.Errorf("%s: InstallLocation = %q, want %q", got[i].Name, got[i].InstallLocation, w)
		}
	}
}
