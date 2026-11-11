package collect

import (
	"strings"

	"github.com/uem/uem/internal/protocol"
)

// Some Windows installers leave an app's uninstall entry without an
// InstallLocation, e.g. Google Chrome's enterprise MSI. Usage tracking needs
// that folder to recognise the app's running programs, so it is taken from
// the "App Paths" registry instead, where apps register their main program
// for the Run box.

// appPath is a program registered under App Paths, with the ProductName from
// its version info.
type appPath struct {
	Exe, ProductName string
}

// fillFromAppPaths sets the InstallLocation of entries that have none to the
// folder of the registered program whose product name equals the entry's
// name. Existing locations are kept, and programs in the Windows folder are
// never used, so an OS program can't be attributed to an app.
func fillFromAppPaths(sw []protocol.Software, progs []appPath) []protocol.Software {
	dirs := map[string]string{} // lower(product name) → program folder
	for _, p := range progs {
		exe := strings.ReplaceAll(strings.Trim(p.Exe, `" `), "/", `\`)
		name := strings.ToLower(strings.TrimSpace(p.ProductName))
		i := strings.LastIndex(exe, `\`)
		if name == "" || i <= 2 || isWindowsDir(exe) {
			continue
		}
		if _, ok := dirs[name]; !ok {
			dirs[name] = exe[:i]
		}
	}
	for i := range sw {
		if strings.Trim(sw[i].InstallLocation, `" `) != "" {
			continue
		}
		if dir := dirs[strings.ToLower(strings.TrimSpace(sw[i].Name))]; dir != "" {
			sw[i].InstallLocation = dir
		}
	}
	return sw
}

// isWindowsDir reports whether a path is inside the Windows folder of any drive.
func isWindowsDir(path string) bool {
	lower := strings.ToLower(path)
	return len(lower) > 3 && lower[1] == ':' && strings.HasPrefix(lower[2:], `\windows\`)
}
