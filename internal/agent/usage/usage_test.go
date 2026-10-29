package usage

import (
	"testing"
	"time"

	"github.com/uem/uem/internal/protocol"
)

func TestAppFor(t *testing.T) {
	win := NewIndex("windows", []protocol.Software{
		{Name: "Adobe Acrobat (64-bit)", InstallLocation: `C:\Program Files\Adobe\Acrobat DC\`},
		{Name: "Adobe Common", InstallLocation: `C:\Program Files\Adobe`},
		{Name: "Zoom Workplace", InstallLocation: `"C:\Users\ana\AppData\Roaming\Zoom"`},
		{Name: "Bad root", InstallLocation: `C:\`},
	})
	lin := NewIndex("linux", []protocol.Software{{Name: "firefox"}, {Name: "libreoffice-calc"}})
	mac := NewIndex("darwin", nil)
	cases := []struct {
		ix   *Index
		exe  string
		want string
	}{
		{mac, "/Applications/Slack.app/Contents/MacOS/Slack", "Slack"},
		{mac, "/Applications/Adobe Photoshop 2024/Adobe Photoshop 2024.app/Contents/MacOS/Adobe Photoshop 2024", "Adobe Photoshop 2024"},
		{mac, "/Applications/Google Chrome.app/Contents/Frameworks/Google Chrome Framework.framework/Helpers/Google Chrome Helper.app/Contents/MacOS/Google Chrome Helper", ""},
		{mac, "/System/Applications/Weather.app/Contents/PlugIns/WeatherWidget.appex/Contents/MacOS/WeatherWidget", ""},
		{mac, "/Applications/Company Portal.app/Contents/XPCServices/Microsoft Single Sign on for Mac.xpc/Contents/MacOS/Microsoft Single Sign on for Mac", ""},
		{mac, "/System/Applications/Calendar.app/Contents/MacOS/Calendar", "Calendar"},
		{mac, "/Users/ana/Applications/Notion.app/Contents/MacOS/Notion", "Notion"},
		{mac, "/Library/Application Support/Adobe/Adobe Desktop Common/ADS/Adobe Desktop Service.app/Contents/MacOS/Adobe Desktop Service", ""},
		{mac, "/System/Library/CoreServices/Finder.app/Contents/MacOS/Finder", ""},
		{mac, "/usr/bin/ssh", ""},
		{win, `C:\Program Files\Adobe\Acrobat DC\Acrobat\Acrobat.exe`, "Adobe Acrobat (64-bit)"},
		{win, `c:\program files\adobe\other\x.exe`, "Adobe Common"},
		{win, `C:\Users\ana\AppData\Roaming\Zoom\bin\Zoom.exe`, "Zoom Workplace"},
		{win, `C:\Windows\System32\notepad.exe`, ""},
		{lin, "/usr/lib/firefox/firefox", "firefox"},
		{lin, "/usr/bin/bash", ""},
	}
	for _, c := range cases {
		if got := c.ix.AppFor(c.exe); got != c.want {
			t.Errorf("AppFor(%q) = %q, want %q", c.exe, got, c.want)
		}
	}
}

func TestIsHuman(t *testing.T) {
	cases := []struct {
		goos string
		p    Proc
		want bool
	}{
		{"darwin", Proc{User: "ana", UID: 501}, true},
		{"darwin", Proc{User: "root", UID: 0}, false},
		{"darwin", Proc{User: "_spotlight", UID: 89}, false},
		{"linux", Proc{User: "ana", UID: 1000}, true},
		{"linux", Proc{User: "zabbix", UID: 112}, false},
		{"windows", Proc{User: `CORP\ana`, UID: -1}, true},
		{"windows", Proc{User: `NT AUTHORITY\SYSTEM`, UID: -1}, false},
		{"windows", Proc{User: `Window Manager\DWM-1`, UID: -1}, false},
		{"windows", Proc{User: `CORP\PC01$`, UID: -1}, false},
		{"windows", Proc{User: "", UID: -1}, false},
	}
	for _, c := range cases {
		if got := IsHuman(c.goos, c.p); got != c.want {
			t.Errorf("IsHuman(%s, %+v) = %v, want %v", c.goos, c.p, got, c.want)
		}
	}
}

func TestTracker(t *testing.T) {
	tr := NewTracker("darwin")
	now := time.Date(2026, 10, 29, 9, 0, 0, 0, time.Local)
	slack := Proc{Exe: "/Applications/Slack.app/Contents/MacOS/Slack", User: "ana", UID: 501}
	helper := Proc{Exe: "/Applications/Slack.app/Contents/Frameworks/Slack Helper.app/Contents/MacOS/Slack Helper", User: "ana", UID: 501}
	for i := range 3 {
		tr.Sample(now.Add(time.Duration(i)*time.Minute), []Proc{slack, helper, {Exe: slack.Exe, User: "root", UID: 0}})
	}
	got := tr.Flush()
	if len(got) != 1 || got[0].App != "Slack" || got[0].User != "ana" || got[0].Minutes != 3 || got[0].Date != "2026-10-29" {
		t.Fatalf("flush = %+v", got)
	}
	if len(tr.Flush()) != 0 {
		t.Fatal("flush did not clear")
	}
	tr.Restore(got)
	tr.Sample(now.Add(5*time.Minute), []Proc{slack})
	if again := tr.Flush(); len(again) != 1 || again[0].Minutes != 4 || !again[0].LastSeen.Equal(now.Add(5*time.Minute)) {
		t.Fatalf("after restore = %+v", again)
	}
}

func TestAppForFront(t *testing.T) {
	mac := NewIndex("darwin", nil)
	win := NewIndex("windows", []protocol.Software{{Name: "Zoom Workplace", InstallLocation: `C:\Program Files\Zoom`}})
	cases := []struct {
		ix         *Index
		path, want string
	}{
		{mac, "/Applications/Visual Studio Code.app", "Visual Studio Code"},
		{mac, "/Applications/Adobe Photoshop 2024/Adobe Photoshop 2024.app", "Adobe Photoshop 2024"},
		{mac, "/System/Library/CoreServices/loginwindow.app", ""},
		{mac, "/System/Library/CoreServices/Finder.app", ""},
		{mac, "", ""},
		{win, `C:\Program Files\Zoom\bin\Zoom.exe`, "Zoom Workplace"},
		{win, `C:\Windows\explorer.exe`, ""},
	}
	for _, c := range cases {
		if got := c.ix.AppForFront(c.path); got != c.want {
			t.Errorf("AppForFront(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}

func TestTrackerActive(t *testing.T) {
	tr := NewTracker("darwin")
	at := time.Date(2026, 10, 29, 9, 0, 0, 0, time.Local)
	code := "/Applications/Visual Studio Code.app"
	slack := "/Applications/Slack.app"
	after := 2 * time.Minute
	tick := func(user, app string, idle time.Duration) {
		tr.Active(user, app, idle, after, at)
		at = at.Add(5 * time.Second)
	}
	tick("ana", code, 0)                 // first sample: credited one 5 s interval
	tick("ana", code, 3*time.Second)     // +5 s
	tick("ana", code, 90*time.Second)    // reading, still under the threshold: +5 s
	tick("ana", code, 2*time.Minute)     // idle at the threshold: not counted
	tick("ana", slack, time.Second)      // +5 s to Slack
	at = at.Add(time.Hour)               // laptop slept
	tick("ana", slack, 0)                // gap capped at 10 s
	tr.Active("ben", code, 0, after, at) // a second user, first sample: +5 s

	got := map[string]int{}
	for _, e := range tr.Flush() {
		got[e.App+"/"+e.User] = e.ActiveSeconds
		if e.Minutes != 0 {
			t.Errorf("%s/%s minutes = %d, want 0", e.App, e.User, e.Minutes)
		}
	}
	want := map[string]int{"Visual Studio Code/ana": 15, "Slack/ana": 15, "Visual Studio Code/ben": 5}
	if len(got) != len(want) {
		t.Fatalf("entries = %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s active = %d s, want %d s", k, got[k], v)
		}
	}
}

func TestTrackerRunningAndActiveMerge(t *testing.T) {
	tr := NewTracker("darwin")
	now := time.Date(2026, 10, 29, 9, 0, 0, 0, time.Local)
	tr.Sample(now, []Proc{{Exe: "/Applications/Slack.app/Contents/MacOS/Slack", User: "ana", UID: 501}})
	tr.Active("ana", "/Applications/Slack.app", 0, time.Minute, now)
	got := tr.Flush()
	if len(got) != 1 || got[0].Minutes != 1 || got[0].ActiveSeconds != 5 {
		t.Fatalf("flush = %+v", got)
	}
	tr.Restore(got)
	if again := tr.Flush(); len(again) != 1 || again[0].ActiveSeconds != 5 {
		t.Fatalf("restore lost active time: %+v", again)
	}
}
