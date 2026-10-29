// Package usage meters how long installed applications run, per user. The
// agent samples running processes once a minute, maps each to an installed
// application and accumulates minutes per (day, app, user) until they are
// flushed to the server.
package usage

import (
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/uem/uem/internal/protocol"
)

// maxEntries bounds memory while the agent is offline for a long time.
const maxEntries = 5000

// Proc is the subset of a running process the tracker needs.
type Proc struct {
	Exe  string
	User string
	UID  int64 // -1 when unknown (always on Windows)
}

// Index maps executables to installed application names. It is rebuilt from
// each inventory so names match the Software tab.
type Index struct {
	goos     string
	prefixes []prefix          // windows: install location → app name, longest first
	packages map[string]string // linux: lower(package name) → package name
}

type prefix struct{ dir, name string }

// NewIndex builds an index from the installed software list.
func NewIndex(goos string, software []protocol.Software) *Index {
	ix := &Index{goos: goos, packages: map[string]string{}}
	for _, s := range software {
		switch goos {
		case "windows":
			dir := strings.TrimRight(strings.ToLower(strings.Trim(s.InstallLocation, `" `)), `\/`)
			if len(dir) > 3 && s.Name != "" { // skip empty and bare drive roots like C:\
				ix.prefixes = append(ix.prefixes, prefix{dir + `\`, s.Name})
			}
		case "linux":
			ix.packages[strings.ToLower(s.Name)] = s.Name
		}
	}
	sort.Slice(ix.prefixes, func(i, j int) bool { return len(ix.prefixes[i].dir) > len(ix.prefixes[j].dir) })
	return ix
}

// AppFor returns the installed application an executable belongs to, or ""
// when it isn't a user-facing installed app.
func (ix *Index) AppFor(exe string) string {
	if exe == "" {
		return ""
	}
	switch ix.goos {
	case "darwin":
		return macApp(exe)
	case "windows":
		lower := strings.ToLower(strings.ReplaceAll(exe, "/", `\`))
		for _, p := range ix.prefixes {
			if strings.HasPrefix(lower, p.dir) {
				return p.name
			}
		}
	case "linux":
		base := exe[strings.LastIndex(exe, "/")+1:]
		return ix.packages[strings.ToLower(base)]
	}
	return ""
}

// AppForFront maps the front app reported by the session probe: a bundle
// path on macOS ("/Applications/Foo.app"), an executable elsewhere.
func (ix *Index) AppForFront(path string) string {
	if ix.goos == "darwin" {
		if !strings.HasSuffix(path, ".app") {
			return ""
		}
		return macApp(path + "/Contents/MacOS/main")
	}
	return ix.AppFor(path)
}

// macApp maps ".../Applications/Foo.app/Contents/MacOS/Foo" to "Foo". Only
// an app's own main executable under an Applications folder counts: widgets
// (.appex), XPC services, plug-ins and nested helper apps run without the app
// being open, and background helpers in /Library aren't apps users open.
func macApp(exe string) string {
	if strings.HasPrefix(exe, "/System/Library/") || strings.HasPrefix(exe, "/Library/Apple/") {
		return ""
	}
	i := strings.Index(exe, "/Applications/")
	if i < 0 {
		return ""
	}
	rest := exe[i+len("/Applications/"):]
	j := strings.Index(rest, ".app/")
	if j < 0 {
		return ""
	}
	bundle, inner := rest[:j], rest[j+len(".app/"):]
	main, ok := strings.CutPrefix(inner, "Contents/MacOS/")
	if !ok || main == "" || strings.Contains(main, "/") {
		return ""
	}
	return bundle[strings.LastIndex(bundle, "/")+1:]
}

// IsHuman reports whether a process owner is an interactive user rather than
// a service or system account.
func IsHuman(goos string, p Proc) bool {
	u := strings.TrimSpace(p.User)
	if u == "" {
		return false
	}
	switch goos {
	case "windows":
		domain, name, ok := strings.Cut(u, `\`)
		if !ok {
			name = domain
			domain = ""
		}
		switch strings.ToUpper(domain) {
		case "NT AUTHORITY", "WINDOW MANAGER", "FONT DRIVER HOST", "NT SERVICE", "IIS APPPOOL":
			return false
		}
		return !strings.HasSuffix(name, "$") && !strings.EqualFold(name, "SYSTEM")
	case "darwin":
		return !strings.HasPrefix(u, "_") && u != "root" && (p.UID < 0 || p.UID >= 500)
	default:
		return u != "root" && u != "nobody" && (p.UID < 0 || p.UID >= 1000)
	}
}

type key struct{ date, app, user string }

type counter struct {
	minutes  int
	active   time.Duration
	lastSeen time.Time
}

// maxActiveGap caps the time credited between two probe samples, so sleep,
// a stalled probe or a lost sample never counts as activity.
const maxActiveGap = 10 * time.Second

// Tracker accumulates sampled minutes. It is safe for concurrent use.
type Tracker struct {
	goos  string
	mu    sync.Mutex
	ix    *Index
	data  map[key]*counter
	lastP map[string]time.Time // user → time of their previous probe sample
}

func NewTracker(goos string) *Tracker {
	return &Tracker{goos: goos, ix: NewIndex(goos, nil), data: map[key]*counter{}, lastP: map[string]time.Time{}}
}

// SetSoftware rebuilds the executable → app index from an inventory.
func (t *Tracker) SetSoftware(software []protocol.Software) {
	ix := NewIndex(t.goos, software)
	t.mu.Lock()
	t.ix = ix
	t.mu.Unlock()
}

// Sample counts one minute for every (app, user) pair running at now. Several
// processes of the same app and user count once.
func (t *Tracker) Sample(now time.Time, procs []Proc) {
	t.mu.Lock()
	defer t.mu.Unlock()
	date := now.Format(time.DateOnly)
	seen := map[key]bool{}
	for _, p := range procs {
		if !IsHuman(t.goos, p) {
			continue
		}
		app := t.ix.AppFor(p.Exe)
		if app == "" {
			continue
		}
		k := key{date, app, p.User}
		if seen[k] {
			continue
		}
		seen[k] = true
		c := t.data[k]
		if c == nil {
			if len(t.data) >= maxEntries {
				continue
			}
			c = &counter{}
			t.data[k] = c
		}
		c.minutes++
		c.lastSeen = now
	}
}

// Active records one probe sample: when the user's last input is more recent
// than idleAfter, the time since their previous sample (at most
// maxActiveGap) counts as active use of the front app.
func (t *Tracker) Active(user, frontPath string, idle, idleAfter time.Duration, now time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	prev, ok := t.lastP[user]
	t.lastP[user] = now
	gap := maxActiveGap / 2 // first sample: assume one probe interval
	if ok {
		gap = min(now.Sub(prev), maxActiveGap)
	}
	if gap <= 0 || idle >= idleAfter || user == "" {
		return
	}
	app := t.ix.AppForFront(frontPath)
	if app == "" {
		return
	}
	k := key{now.Format(time.DateOnly), app, user}
	c := t.data[k]
	if c == nil {
		if len(t.data) >= maxEntries {
			return
		}
		c = &counter{}
		t.data[k] = c
	}
	c.active += gap
	if now.After(c.lastSeen) {
		c.lastSeen = now
	}
}

// Flush returns the accumulated entries and clears them.
func (t *Tracker) Flush() []protocol.UsageEntry {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]protocol.UsageEntry, 0, len(t.data))
	for k, c := range t.data {
		out = append(out, protocol.UsageEntry{
			Date: k.date, App: k.app, User: k.user, Minutes: c.minutes,
			ActiveSeconds: int(c.active.Round(time.Second) / time.Second), LastSeen: c.lastSeen,
		})
	}
	t.data = map[key]*counter{}
	return out
}

// Restore puts back entries that could not be delivered.
func (t *Tracker) Restore(entries []protocol.UsageEntry) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, e := range entries {
		k := key{e.Date, e.App, e.User}
		c := t.data[k]
		if c == nil {
			if len(t.data) >= maxEntries {
				continue
			}
			c = &counter{}
			t.data[k] = c
		}
		c.minutes += e.Minutes
		c.active += time.Duration(e.ActiveSeconds) * time.Second
		if e.LastSeen.After(c.lastSeen) {
			c.lastSeen = e.LastSeen
		}
	}
}

// Reset discards everything, e.g. when tracking is switched off.
func (t *Tracker) Reset() {
	t.mu.Lock()
	t.data = map[key]*counter{}
	t.lastP = map[string]time.Time{}
	t.mu.Unlock()
}
