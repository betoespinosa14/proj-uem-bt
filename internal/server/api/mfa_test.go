package api_test

import (
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/uem/uem/internal/server/auth"
)

// newBrowser is a fresh cookie jar, like a new browser or private window.
func (e *env) newBrowser(t *testing.T) *client {
	jar, _ := cookiejar.New(nil)
	return &client{t: t, base: e.srv.URL, http: &http.Client{Jar: jar}}
}

func (c *client) syncCSRF() {
	u, _ := url.Parse(c.base)
	for _, ck := range c.http.Jar.Cookies(u) {
		if ck.Name == "uem_csrf" {
			c.csrf = ck.Value
		}
	}
}

// login signs in with a password and reports whether a two-step code is needed.
func (c *client) login(email, password string) (mfaRequired bool) {
	var out struct {
		MFARequired bool `json:"mfa_required"`
		User        *struct {
			MFAEnabled bool `json:"mfa_enabled"`
		} `json:"user"`
	}
	c.do("POST", "/api/auth/login", map[string]string{"email": email, "password": password}, 200, &out)
	if out.MFARequired == (out.User != nil) {
		c.t.Fatalf("login: want either mfa_required or a user, got %+v", out)
	}
	c.syncCSRF()
	return out.MFARequired
}

func TestTwoStepSignIn(t *testing.T) {
	e := setup(t)
	owner := e.signup(t, "Mfa")
	email := fmt.Sprintf("mfa-%d@test.example", time.Now().UnixNano())
	var created struct{ ID string }
	owner.do("POST", "/api/users", map[string]any{"email": email, "name": "M", "role": "admin", "password": "correct-horse-1"}, 201, &created)

	// Off by default: the password alone signs in.
	u := e.newBrowser(t)
	if u.login(email, "correct-horse-1") {
		t.Fatal("code asked for before two-step sign-in was turned on")
	}
	var st struct {
		Enabled           bool `json:"enabled"`
		RecoveryCodesLeft int  `json:"recovery_codes_left"`
		TrustedBrowsers   int  `json:"trusted_browsers"`
	}
	u.do("GET", "/api/auth/mfa", nil, 200, &st)
	if st.Enabled {
		t.Fatal("enabled before setup")
	}

	// Turn it on.
	var setupResp struct{ Secret, URI string }
	u.do("POST", "/api/auth/mfa/setup", nil, 200, &setupResp)
	if setupResp.Secret == "" || setupResp.URI == "" {
		t.Fatalf("setup returned %+v", setupResp)
	}
	step := auth.TOTPStep(time.Now())
	code := func(s int64) string { c, _ := auth.TOTPCode(setupResp.Secret, s); return c }
	wrong := func(s int64) string {
		n, _ := strconv.Atoi(code(s))
		return fmt.Sprintf("%06d", (n+1)%1_000_000)
	}
	u.do("POST", "/api/auth/mfa/enable", map[string]string{"code": wrong(step)}, 400, nil)
	var enabled struct {
		RecoveryCodes []string `json:"recovery_codes"`
	}
	u.do("POST", "/api/auth/mfa/enable", map[string]string{"code": code(step)}, 200, &enabled)
	if len(enabled.RecoveryCodes) != 10 {
		t.Fatalf("got %d recovery codes", len(enabled.RecoveryCodes))
	}
	u.do("POST", "/api/auth/mfa/setup", nil, 409, nil) // already on
	var me struct {
		User struct {
			MFAEnabled bool `json:"mfa_enabled"`
		} `json:"user"`
	}
	u.do("GET", "/api/auth/me", nil, 200, &me)
	if !me.User.MFAEnabled {
		t.Fatal("me doesn't report two-step sign-in")
	}

	// A new browser: the password alone no longer opens a session.
	b1 := e.newBrowser(t)
	if !b1.login(email, "correct-horse-1") {
		t.Fatal("no code asked for")
	}
	b1.do("GET", "/api/devices", nil, 401, nil)
	// The code used to turn it on can't be reused.
	b1.do("POST", "/api/auth/login/mfa", map[string]any{"code": code(step)}, 401, nil)
	// After 5 attempts the challenge is gone, even for a right code.
	for i := 0; i < 4; i++ {
		b1.do("POST", "/api/auth/login/mfa", map[string]any{"code": wrong(step + 1)}, 401, nil)
	}
	b1.do("POST", "/api/auth/login/mfa", map[string]any{"code": code(step + 1)}, 401, nil)
	b1.do("GET", "/api/devices", nil, 401, nil)
	// Without a password first there's no challenge at all.
	e.newBrowser(t).do("POST", "/api/auth/login/mfa", map[string]any{"code": code(step + 1)}, 401, nil)

	// The right code, remembering the browser.
	b2 := e.newBrowser(t)
	b2.login(email, "correct-horse-1")
	b2.do("POST", "/api/auth/login/mfa", map[string]any{"code": code(step + 1), "remember": true}, 200, nil)
	b2.syncCSRF()
	b2.do("GET", "/api/devices", nil, 200, nil)
	b2.do("POST", "/api/auth/logout", nil, 200, nil)
	if b2.login(email, "correct-horse-1") {
		t.Fatal("remembered browser was asked for a code")
	}
	b2.do("GET", "/api/devices", nil, 200, nil)

	// Recovery codes work once, typed in any case.
	b3 := e.newBrowser(t)
	b3.login(email, "correct-horse-1")
	b3.do("POST", "/api/auth/login/mfa", map[string]any{"code": " " + enabled.RecoveryCodes[0] + " "}, 200, nil)
	b4 := e.newBrowser(t)
	b4.login(email, "correct-horse-1")
	b4.do("POST", "/api/auth/login/mfa", map[string]any{"code": enabled.RecoveryCodes[0]}, 401, nil)
	b4.do("POST", "/api/auth/login/mfa", map[string]any{"code": enabled.RecoveryCodes[1]}, 200, nil)
	u.do("GET", "/api/auth/mfa", nil, 200, &st)
	if !st.Enabled || st.RecoveryCodesLeft != 8 || st.TrustedBrowsers != 1 {
		t.Fatalf("status %+v, want on, 8 codes left, 1 remembered browser", st)
	}

	// Sensitive changes need the password.
	u.do("POST", "/api/auth/mfa/disable", map[string]string{"password": "wrong-password"}, 400, nil)
	u.do("POST", "/api/auth/mfa/recovery-codes", map[string]string{"password": "wrong-password"}, 400, nil)
	var fresh struct {
		RecoveryCodes []string `json:"recovery_codes"`
	}
	u.do("POST", "/api/auth/mfa/recovery-codes", map[string]string{"password": "correct-horse-1"}, 200, &fresh)
	b5 := e.newBrowser(t)
	b5.login(email, "correct-horse-1")
	b5.do("POST", "/api/auth/login/mfa", map[string]any{"code": enabled.RecoveryCodes[2]}, 401, nil) // old set replaced
	b5.do("POST", "/api/auth/login/mfa", map[string]any{"code": fresh.RecoveryCodes[0]}, 200, nil)

	// Another organisation can't reset it; an owner of this one can, which
	// also signs the user out everywhere.
	other := e.signup(t, "MfaOther")
	other.do("POST", "/api/users/"+created.ID+"/reset-mfa", nil, 404, nil)
	owner.do("POST", "/api/users/"+created.ID+"/reset-mfa", nil, 200, nil)
	b2.do("GET", "/api/devices", nil, 401, nil)
	if e.newBrowser(t).login(email, "correct-horse-1") {
		t.Fatal("code still asked for after the reset")
	}

	// A password change makes remembered browsers ask for a code again.
	u = e.newBrowser(t)
	u.login(email, "correct-horse-1")
	u.do("POST", "/api/auth/mfa/setup", nil, 200, &setupResp)
	step = auth.TOTPStep(time.Now())
	var again struct {
		RecoveryCodes []string `json:"recovery_codes"`
	}
	u.do("POST", "/api/auth/mfa/enable", map[string]string{"code": code(step)}, 200, &again)
	b6 := e.newBrowser(t)
	b6.login(email, "correct-horse-1")
	b6.do("POST", "/api/auth/login/mfa", map[string]any{"code": code(step + 1), "remember": true}, 200, nil)
	owner.do("PATCH", "/api/users/"+created.ID, map[string]any{"password": "another-horse-2"}, 200, nil)
	if !b6.login(email, "another-horse-2") {
		t.Fatal("remembered browser skipped the code after a password change")
	}

	// Turning it off needs the password; afterwards the password alone works.
	b6.do("POST", "/api/auth/login/mfa", map[string]any{"code": again.RecoveryCodes[0]}, 200, nil)
	b6.syncCSRF()
	b6.do("POST", "/api/auth/mfa/disable", map[string]string{"password": "another-horse-2"}, 200, nil)
	if e.newBrowser(t).login(email, "another-horse-2") {
		t.Fatal("code still asked for after turning it off")
	}

	// An owner can't reset their own from the Users page.
	var ownerMe struct{ User struct{ ID string } }
	owner.do("GET", "/api/auth/me", nil, 200, &ownerMe)
	owner.do("POST", "/api/users/"+ownerMe.User.ID+"/reset-mfa", nil, 400, nil)
}
