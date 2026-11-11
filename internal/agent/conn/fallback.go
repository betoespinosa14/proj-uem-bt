package conn

import (
	"context"
	"net/url"
	"slices"
	"strings"
	"time"
)

// Fallback addresses: the server announces other addresses of itself (e.g.
// its IP next to its DNS name). After fallbackAfter failed connections in a
// row to the main address, the agent connects through the first fallback
// that answers its health check. It never saves a fallback as its address:
// while on one it re-checks the main address every primaryCheckEvery and
// returns as soon as it answers.

const (
	fallbackAfter     = 3
	primaryCheckEvery = 5 * time.Minute
)

// fallbackCandidates returns the usable fallback addresses, normalized, in
// order, without duplicates or the main address itself.
func fallbackCandidates(primary string, fallbacks []string) []string {
	primary = strings.TrimRight(primary, "/")
	var out []string
	for _, f := range fallbacks {
		f = strings.TrimRight(strings.TrimSpace(f), "/")
		u, err := url.Parse(f)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			continue
		}
		if strings.EqualFold(f, primary) || slices.ContainsFunc(out, func(o string) bool { return strings.EqualFold(o, f) }) {
			continue
		}
		out = append(out, f)
	}
	return out
}

// setFallbacks records the fallback list from a policy and saves it when it
// changed, so the agent knows it after a restart during an outage.
func (a *Agent) setFallbacks(list []string) {
	a.mu.Lock()
	list = fallbackCandidates(a.cfg.ServerURL, list)
	if slices.Equal(list, a.cfg.FallbackURLs) {
		a.mu.Unlock()
		return
	}
	a.cfg.FallbackURLs = list
	save := a.SaveFallbacks
	a.mu.Unlock()
	if save != nil {
		if err := save(list); err != nil {
			a.log.Error("couldn't save fallback server addresses", "err", err)
		}
	}
}

// primaryFailed counts a failed connection to the main address.
func (a *Agent) primaryFailed() {
	a.mu.Lock()
	a.primaryFails++
	a.mu.Unlock()
}

// pickFallback switches to the first healthy fallback once the main address
// has failed often enough. It reports whether it switched.
func (a *Agent) pickFallback(ctx context.Context) bool {
	a.mu.Lock()
	if a.fallbackURL != "" || a.trialURL != "" || a.primaryFails < fallbackAfter {
		a.mu.Unlock()
		return false
	}
	primary := a.cfg.ServerURL
	candidates := fallbackCandidates(primary, a.cfg.FallbackURLs)
	a.mu.Unlock()
	for _, f := range candidates {
		if err := a.health(ctx, f); err != nil {
			a.log.Warn("fallback server address isn't answering", "fallback", f, "err", err)
			continue
		}
		a.mu.Lock()
		a.fallbackURL = f
		a.mu.Unlock()
		a.log.Warn("main server address unreachable; using a fallback", "main", primary, "fallback", f)
		return true
	}
	return false
}

// fallbackFailed drops the current fallback after it stopped working, so
// the next attempt goes to the main address again.
func (a *Agent) fallbackFailed() {
	a.mu.Lock()
	a.fallbackURL = ""
	a.mu.Unlock()
}

// connected resets the failure count after connecting to the main address.
func (a *Agent) connectedToPrimary() {
	a.mu.Lock()
	a.primaryFails = 0
	a.mu.Unlock()
}

// watchPrimary runs while connected through a fallback and ends that session
// once the main address answers again.
func (a *Agent) watchPrimary(ctx context.Context, end context.CancelFunc) {
	t := time.NewTicker(a.primaryEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		a.mu.Lock()
		primary := a.cfg.ServerURL
		a.mu.Unlock()
		if a.health(ctx, primary) != nil {
			continue
		}
		a.mu.Lock()
		a.fallbackURL = ""
		a.primaryFails = 0
		a.reconnectNow = true
		a.mu.Unlock()
		a.log.Info("main server address is back; reconnecting to it", "main", primary)
		end()
		return
	}
}

// takeReconnectNow reports, once, that the next connection should start
// without waiting.
func (a *Agent) takeReconnectNow() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	r := a.reconnectNow
	a.reconnectNow = false
	return r
}
