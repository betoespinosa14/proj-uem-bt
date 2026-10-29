package session

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"time"
)

const reconcileEvery = 30 * time.Second

// blindAfter is how many consecutive samples without a front app (about a
// minute) make the supervisor warn that the probe can't see the front app.
const blindAfter = 12

var errNoFrontApp = errors.New("session probe reports no front app")

// userSession is one interactive session that should have a probe.
type userSession struct {
	key  string // stable identity of the session (uid or session ID)
	user string // as the process tracker reports it, e.g. "ana" or `CORP\ana`
	id   uint32 // platform session identifier
}

// helperProc is a running probe as seen by the supervisor.
type helperProc struct {
	stdout io.Reader
	stop   func()       // asks the probe to exit (closes its stdin), then kills it
	wait   func() error // blocks until the probe has exited
}

// Supervisor keeps one probe running per interactive user session and
// forwards its samples.
type Supervisor struct {
	Exe      string // path of the agent binary, run as "<Exe> session-probe"
	Log      *slog.Logger
	OnSample func(user string, s Sample, at time.Time)

	mu      sync.Mutex
	running map[string]*helperProc
	lastErr map[string]string
}

// Run reconciles probes every 30 s while enabled reports true, and stops
// them all when it reports false or ctx ends.
func (s *Supervisor) Run(ctx context.Context, enabled func() bool) {
	if !supported {
		return
	}
	s.running, s.lastErr = map[string]*helperProc{}, map[string]string{}
	t := time.NewTicker(reconcileEvery)
	defer t.Stop()
	for {
		if enabled() {
			s.reconcile(ctx)
		} else {
			s.stopAll(nil)
		}
		select {
		case <-ctx.Done():
			s.stopAll(nil)
			return
		case <-t.C:
		}
	}
}

func (s *Supervisor) reconcile(ctx context.Context) {
	sessions, err := listSessions(ctx)
	if err != nil {
		s.logOnce("list", "listing user sessions failed", err)
		return
	}
	want := map[string]bool{}
	for _, us := range sessions {
		want[us.key] = true
		s.mu.Lock()
		_, ok := s.running[us.key]
		s.mu.Unlock()
		if !ok {
			s.start(ctx, us)
		}
	}
	s.stopAll(want)
}

func (s *Supervisor) start(ctx context.Context, us userSession) {
	h, err := startHelper(ctx, s.Exe, us)
	if err != nil {
		s.logOnce(us.key, "starting session probe failed", err, "user", us.user)
		return
	}
	s.mu.Lock()
	s.running[us.key] = h
	delete(s.lastErr, us.key)
	s.mu.Unlock()
	s.Log.Info("session probe started", "user", us.user)
	go func() {
		blind := 0
		_ = ReadSamples(h.stdout, func(smp Sample) {
			if smp.AppPath == "" {
				if blind++; blind == blindAfter {
					s.logOnce(us.key+"/front", "active usage can't be measured", errNoFrontApp, "user", us.user)
				}
			} else {
				blind = 0
			}
			s.OnSample(us.user, smp, time.Now())
		})
		_ = h.wait()
		s.mu.Lock()
		if s.running[us.key] == h {
			delete(s.running, us.key) // restarted on the next reconcile if the session is still there
		}
		s.mu.Unlock()
	}()
}

// stopAll stops every probe whose session isn't in keep (all when keep is nil).
func (s *Supervisor) stopAll(keep map[string]bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, h := range s.running {
		if !keep[k] {
			h.stop()
			delete(s.running, k)
		}
	}
}

// logOnce logs a failure the first time it happens for a key, so a
// persistent problem doesn't log every 30 s.
func (s *Supervisor) logOnce(key, msg string, err error, attrs ...any) {
	s.mu.Lock()
	seen := s.lastErr[key] == err.Error()
	s.lastErr[key] = err.Error()
	s.mu.Unlock()
	if !seen {
		s.Log.Warn(msg, append(attrs, "err", err)...)
	}
}

// stopper closes stdin, then kills the process if it hasn't exited.
func stopper(stdin io.Closer, done <-chan struct{}, kill func()) func() {
	return func() {
		_ = stdin.Close()
		go func() {
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				kill()
			}
		}()
	}
}
