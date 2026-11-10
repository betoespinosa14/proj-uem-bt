package collect

import (
	"encoding/xml"
	"strings"
)

// Microsoft Store (MSIX/AppX) apps aren't in the registry's uninstall list.
// Their names and publishers come from each package's AppxManifest.xml.

type appxManifest struct {
	Identity struct {
		Name string `xml:"Name,attr"`
	} `xml:"Identity"`
	Properties struct {
		DisplayName          string `xml:"DisplayName"`
		PublisherDisplayName string `xml:"PublisherDisplayName"`
		Framework            bool   `xml:"Framework"`
		ResourcePackage      bool   `xml:"ResourcePackage"`
	} `xml:"Properties"`
	Applications []struct {
		Executable string `xml:"Executable,attr"`
		Visual     struct {
			DisplayName  string `xml:"DisplayName,attr"`
			AppListEntry string `xml:"AppListEntry,attr"`
		} `xml:"VisualElements"`
	} `xml:"Applications>Application"`
}

// appxApp is what a Store package contributes to the software list.
type appxApp struct {
	Name      string // display name, possibly an unresolved "ms-resource:" reference
	Fallback  string // VisualElements name or a readable package name, used if Name can't be resolved
	Publisher string
}

// parseAppxManifest reads a package manifest. ok is false for packages that
// aren't apps a user opens: frameworks, resource packs, and packages with no
// executable listed in the Start menu.
func parseAppxManifest(data []byte) (app appxApp, ok bool) {
	var m appxManifest
	if err := xml.Unmarshal(data, &m); err != nil || m.Properties.Framework || m.Properties.ResourcePackage {
		return app, false
	}
	visual := ""
	for _, a := range m.Applications {
		if a.Executable != "" && !strings.EqualFold(a.Visual.AppListEntry, "none") {
			ok = true
			if visual == "" {
				visual = a.Visual.DisplayName
			}
		}
	}
	if !ok {
		return app, false
	}
	app.Name = strings.TrimSpace(m.Properties.DisplayName)
	app.Publisher = strings.TrimSpace(m.Properties.PublisherDisplayName)
	app.Fallback = readablePackageName(m.Identity.Name)
	if v := strings.TrimSpace(visual); v != "" && !isResourceRef(v) {
		app.Fallback = v
	}
	if app.Name == "" {
		app.Name = app.Fallback
	}
	if isResourceRef(app.Publisher) {
		app.Publisher = ""
	}
	return app, true
}

func isResourceRef(s string) bool { return strings.HasPrefix(strings.ToLower(s), "ms-resource:") }

// resourceURIs turns a manifest "ms-resource:" reference into the full URIs
// Windows can resolve for a package, most likely first.
func resourceURIs(ref, packageName string) []string {
	rest := ref[len("ms-resource:"):]
	if strings.HasPrefix(rest, "///") { // package-relative: "ms-resource:///Resources/X"
		return []string{"ms-resource://" + packageName + rest[2:]}
	}
	if strings.HasPrefix(rest, "//") {
		return []string{ref}
	}
	rest = strings.TrimPrefix(rest, "/")
	if strings.Contains(rest, "/") {
		return []string{"ms-resource://" + packageName + "/" + rest}
	}
	return []string{
		"ms-resource://" + packageName + "/Resources/" + rest,
		"ms-resource://" + packageName + "/" + rest,
	}
}

// readablePackageName makes "Microsoft.WindowsCalculator" read "WindowsCalculator"
// and "5319275A.WhatsAppDesktop" read "WhatsAppDesktop": the part after the
// publisher prefix.
func readablePackageName(name string) string {
	if i := strings.LastIndex(name, "."); i >= 0 && i < len(name)-1 {
		return name[i+1:]
	}
	return name
}

// officeApps are the main programs of a Microsoft Office / Microsoft 365
// install. They all live in one folder that belongs to the suite's single
// uninstall entry, so each is listed (and its use counted) under its own name.
var officeApps = []struct{ exe, name string }{
	{"WINWORD.EXE", "Microsoft Word"},
	{"EXCEL.EXE", "Microsoft Excel"},
	{"POWERPNT.EXE", "Microsoft PowerPoint"},
	{"OUTLOOK.EXE", "Microsoft Outlook"},
	{"ONENOTE.EXE", "Microsoft OneNote"},
	{"MSACCESS.EXE", "Microsoft Access"},
	{"MSPUB.EXE", "Microsoft Publisher"},
	{"VISIO.EXE", "Microsoft Visio"},
	{"WINPROJ.EXE", "Microsoft Project"},
	{"lync.exe", "Skype for Business"},
}

// officeDirs are where an Office suite's programs live, relative to its install location.
var officeDirs = []string{`root\Office16`, `Office16`, `root\Office15`, `Office15`}
