package conn

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Moving to a new server address: the server announces the address in its
// policy. The agent checks the address answers, reconnects to it, and saves
// it only once that connection succeeds. If it fails, the agent goes back to
// the address it had and doesn't retry that one for retryMoveAfter, so a
// wrong or unreachable address can never strand a device.

// moveTarget returns the normalized address to switch to, or false when the
// announced address is empty, the current one, invalid, or failed recently.
func moveTarget(current, announced string, failed map[string]time.Time, now time.Time) (string, bool) {
	target := strings.TrimRight(strings.TrimSpace(announced), "/")
	if target == "" || strings.EqualFold(target, strings.TrimRight(current, "/")) {
		return "", false
	}
	u, err := url.Parse(target)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", false
	}
	if at, ok := failed[target]; ok && now.Sub(at) < retryMoveAfter {
		return "", false
	}
	return target, true
}

// checkHealth confirms a server answers on its health endpoint.
func checkHealth(ctx context.Context, server string) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server+"/healthz", nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check returned %s", resp.Status)
	}
	return nil
}

// maybeMove starts a switch to the announced address when appropriate.
func (a *Agent) maybeMove(ctx context.Context, announced string) {
	if a.SaveServerURL == nil {
		return
	}
	a.mu.Lock()
	current := a.cfg.ServerURL
	target, ok := moveTarget(current, announced, a.failedMoves, time.Now())
	if !ok || a.checking || a.trialURL != "" {
		a.mu.Unlock()
		return
	}
	a.checking = true
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		a.checking = false
		a.mu.Unlock()
	}()

	if err := checkHealth(ctx, target); err != nil {
		a.moveFailed(target, err)
		return
	}
	a.log.Info("server announced a new address; reconnecting to it", "from", current, "to", target)
	a.mu.Lock()
	a.trialURL = target
	end := a.endSession
	a.mu.Unlock()
	if end != nil {
		end()
	}
}

// serverTarget returns the address for the next connection and whether it
// is a new, not yet confirmed one.
func (a *Agent) serverTarget() (string, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.trialURL != "" {
		return a.trialURL, true
	}
	if a.fallbackURL != "" {
		return a.fallbackURL, false
	}
	return a.cfg.ServerURL, false
}

func (a *Agent) hasTrial() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.trialURL != ""
}

func (a *Agent) moveFailed(target string, err error) {
	a.mu.Lock()
	a.failedMoves[target] = time.Now()
	if a.trialURL == target {
		a.trialURL = ""
	}
	current := a.cfg.ServerURL
	a.mu.Unlock()
	a.log.Warn("new server address isn't usable; keeping the current one",
		"current", current, "new", target, "retry_in", retryMoveAfter, "err", err)
}

func (a *Agent) moveSucceeded(target string) {
	a.mu.Lock()
	from := a.cfg.ServerURL
	a.cfg.ServerURL = target
	a.trialURL = ""
	a.mu.Unlock()
	if err := a.SaveServerURL(target); err != nil {
		// Still connected; after a restart the agent uses the old address
		// and is moved again by the next policy.
		a.log.Error("connected to the new server address but couldn't save it", "server", target, "err", err)
		return
	}
	a.log.Info("server address changed", "from", from, "to", target)
}
