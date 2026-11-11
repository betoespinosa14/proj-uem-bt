package api

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/uem/uem/internal/server/audit"
	"github.com/uem/uem/internal/server/auth"
	"github.com/uem/uem/internal/server/store"
)

// Two-step sign-in (optional per user). After a correct password, a user with
// it on gets a short-lived challenge instead of a session, and must send a code
// from their authenticator app (or a recovery code) to /auth/login/mfa.

const (
	mfaCookie       = "uem_mfa"     // pending sign-in challenge
	trustedCookie   = "uem_trusted" // "remember this browser"
	mfaChallengeTTL = 5 * time.Minute
	mfaMaxAttempts  = 5
	trustedTTL      = 30 * 24 * time.Hour
	mfaIssuer       = "BEACON"
)

type mfaChallenge struct {
	userID, orgID string
	expires       time.Time
	attempts      int
}

// challenges holds pending sign-ins in memory (one server replica, like the
// login limiter). Keys are hashes of the cookie token.
type challenges struct {
	mu sync.Mutex
	m  map[string]*mfaChallenge
}

func newChallenges() *challenges { return &challenges{m: map[string]*mfaChallenge{}} }

func (c *challenges) add(userID, orgID string) string {
	token := auth.RandomToken(32)
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	for k, v := range c.m { // drop expired ones; the map stays small
		if now.After(v.expires) {
			delete(c.m, k)
		}
	}
	if len(c.m) > 10_000 {
		c.m = map[string]*mfaChallenge{}
	}
	c.m[auth.HashToken(token)] = &mfaChallenge{userID: userID, orgID: orgID, expires: now.Add(mfaChallengeTTL)}
	return token
}

// attempt counts a code attempt and returns the challenge, or nil if it is
// unknown, expired or out of attempts.
func (c *challenges) attempt(token string) *mfaChallenge {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := auth.HashToken(token)
	ch := c.m[key]
	if ch == nil {
		return nil
	}
	if time.Now().After(ch.expires) || ch.attempts >= mfaMaxAttempts {
		delete(c.m, key)
		return nil
	}
	ch.attempts++
	cp := *ch
	return &cp
}

func (c *challenges) done(token string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.m, auth.HashToken(token))
}

func (s *Server) setCookie(w http.ResponseWriter, name, value, path string, expires time.Time) {
	c := &http.Cookie{Name: name, Value: value, Path: path, HttpOnly: true, Secure: s.cfg.SecureCookies,
		SameSite: http.SameSiteStrictMode}
	if value == "" {
		c.MaxAge = -1
	} else {
		c.Expires = expires
	}
	http.SetCookie(w, c)
}

// trustedBrowser reports whether the request carries a valid "remember this
// browser" cookie for the user.
func (s *Server) trustedBrowser(r *http.Request, userID string) bool {
	c, err := r.Cookie(trustedCookie)
	if err != nil || c.Value == "" {
		return false
	}
	return s.store.IsTrustedBrowser(r.Context(), auth.HashToken(c.Value), userID)
}

// beginMFA answers a correct password for a user with two-step sign-in on.
func (s *Server) beginMFA(w http.ResponseWriter, user store.User) {
	token := s.mfaPending.add(user.ID, user.OrgID)
	s.setCookie(w, mfaCookie, token, "/api/auth", time.Now().Add(mfaChallengeTTL))
	writeJSON(w, http.StatusOK, map[string]bool{"mfa_required": true})
}

func (s *Server) loginMFA(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code     string `json:"code"`
		Remember bool   `json:"remember"`
	}
	if err := decode(r, &req); err != nil {
		fail(w, err)
		return
	}
	c, err := r.Cookie(mfaCookie)
	var ch *mfaChallenge
	if err == nil && c.Value != "" {
		ch = s.mfaPending.attempt(c.Value)
	}
	if ch == nil {
		s.setCookie(w, mfaCookie, "", "/api/auth", time.Time{})
		writeErr(w, http.StatusUnauthorized, "your sign-in timed out or had too many wrong codes; enter your password again")
		return
	}
	if !s.mfaLimiter.allow(ch.userID) {
		writeErr(w, http.StatusTooManyRequests, "too many wrong codes, try again in 15 minutes")
		return
	}
	user, err := s.store.GetUser(r.Context(), ch.orgID, ch.userID)
	if err != nil {
		fail(w, err)
		return
	}
	actor := audit.Actor{Type: "user", ID: user.ID, Email: user.Email, IP: clientIP(r)}
	method, ok := s.checkSecondFactor(r, user, req.Code)
	if !ok {
		s.audit.Record(r.Context(), audit.Event{OrgID: user.OrgID, Actor: actor,
			Action: "auth.mfa_failed", TargetType: "user", TargetID: user.ID})
		writeErr(w, http.StatusUnauthorized, "that code didn't work")
		return
	}
	s.mfaPending.done(c.Value)
	s.mfaLimiter.reset(ch.userID)
	s.setCookie(w, mfaCookie, "", "/api/auth", time.Time{})
	if method == "recovery_code" {
		s.audit.Record(r.Context(), audit.Event{OrgID: user.OrgID, Actor: actor,
			Action: "auth.mfa_recovery_used", TargetType: "user", TargetID: user.ID})
	}
	if req.Remember {
		token := auth.RandomToken(32)
		exp := time.Now().Add(trustedTTL)
		if err := s.store.AddTrustedBrowser(r.Context(), auth.HashToken(token), user.ID, exp, r.UserAgent()); err == nil {
			s.setCookie(w, trustedCookie, token, "/api/auth", exp)
		}
	}
	s.finishLogin(w, r, user, method)
}

// checkSecondFactor accepts a current authenticator code or an unused
// recovery code, and returns which one it was.
func (s *Server) checkSecondFactor(r *http.Request, user store.User, code string) (string, bool) {
	code = strings.TrimSpace(code)
	if code == "" {
		return "", false
	}
	if isDigits(strings.NewReplacer(" ", "", "-", "").Replace(code)) {
		m, err := s.store.GetMFA(r.Context(), user.ID)
		if err != nil || m.Secret == "" {
			return "", false
		}
		step, ok := auth.CheckTOTP(m.Secret, code, time.Now(), m.LastStep)
		if !ok {
			return "", false
		}
		used, err := s.store.UseTOTPStep(r.Context(), user.ID, step)
		return "authenticator_app", err == nil && used
	}
	used, err := s.store.UseRecoveryCode(r.Context(), user.ID, auth.HashRecoveryCode(code))
	return "recovery_code", err == nil && used
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// ---- Account security (the signed-in user's own settings) ----

func (s *Server) mfaStatus(w http.ResponseWriter, r *http.Request) {
	sum, err := s.store.MFASummary(r.Context(), who(r).User.ID)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sum)
}

// mfaSetup starts turning two-step sign-in on: a new secret is saved as
// pending and returned once, for the QR code.
func (s *Server) mfaSetup(w http.ResponseWriter, r *http.Request) {
	u := who(r).User
	if u.MFAEnabled {
		writeErr(w, http.StatusConflict, "two-step sign-in is already on")
		return
	}
	secret := auth.NewTOTPSecret()
	if err := s.store.SetPendingTOTP(r.Context(), u.ID, secret); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"secret": secret, "uri": auth.TOTPURI(mfaIssuer, u.Email, secret)})
}

func (s *Server) mfaEnable(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code string `json:"code"`
	}
	if err := decode(r, &req); err != nil {
		fail(w, err)
		return
	}
	u := who(r).User
	if !s.mfaLimiter.allow(u.ID) {
		writeErr(w, http.StatusTooManyRequests, "too many wrong codes, try again in 15 minutes")
		return
	}
	m, err := s.store.GetMFA(r.Context(), u.ID)
	if err != nil {
		fail(w, err)
		return
	}
	if m.Secret != "" {
		writeErr(w, http.StatusConflict, "two-step sign-in is already on")
		return
	}
	if m.PendingSecret == "" {
		fail(w, badRequest("start the setup first"))
		return
	}
	step, ok := auth.CheckTOTP(m.PendingSecret, req.Code, time.Now(), 0)
	if !ok {
		// 400, not 401: the console treats 401 as "signed out".
		fail(w, badRequest("that code didn't work"))
		return
	}
	codes := auth.NewRecoveryCodes()
	if err := s.store.EnableMFA(r.Context(), u.ID, m.PendingSecret, step, hashCodes(codes)); err != nil {
		if err == store.ErrConflict {
			writeErr(w, http.StatusConflict, "the setup was restarted elsewhere; scan the new QR code")
			return
		}
		fail(w, err)
		return
	}
	s.mfaLimiter.reset(u.ID)
	s.record(r, "auth.mfa_enabled", "user", u.ID, nil)
	writeJSON(w, http.StatusOK, map[string]any{"recovery_codes": codes})
}

func hashCodes(codes []string) []string {
	out := make([]string, len(codes))
	for i, c := range codes {
		out[i] = auth.HashRecoveryCode(c)
	}
	return out
}

// confirmPassword re-checks the signed-in user's password before a sensitive
// change.
func (s *Server) confirmPassword(w http.ResponseWriter, r *http.Request, password string) bool {
	u := who(r).User
	if !s.loginLimiter.allow("reauth:" + u.ID) {
		writeErr(w, http.StatusTooManyRequests, "too many attempts, try again later")
		return false
	}
	if auth.CheckPassword(u.PasswordHash, password) != nil {
		fail(w, badRequest("that password isn't right")) // 400: a 401 would sign the console out
		return false
	}
	s.loginLimiter.reset("reauth:" + u.ID)
	return true
}

func (s *Server) mfaDisable(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
	}
	if err := decode(r, &req); err != nil {
		fail(w, err)
		return
	}
	if !s.confirmPassword(w, r, req.Password) {
		return
	}
	u := who(r).User
	if err := s.store.DisableMFA(r.Context(), u.ID); err != nil {
		fail(w, err)
		return
	}
	s.setCookie(w, trustedCookie, "", "/api/auth", time.Time{})
	s.record(r, "auth.mfa_disabled", "user", u.ID, nil)
	ok(w)
}

func (s *Server) mfaRecoveryCodes(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
	}
	if err := decode(r, &req); err != nil {
		fail(w, err)
		return
	}
	u := who(r).User
	if !u.MFAEnabled {
		fail(w, badRequest("two-step sign-in is off"))
		return
	}
	if !s.confirmPassword(w, r, req.Password) {
		return
	}
	codes := auth.NewRecoveryCodes()
	if err := s.store.ReplaceRecoveryCodes(r.Context(), u.ID, hashCodes(codes)); err != nil {
		fail(w, err)
		return
	}
	s.record(r, "auth.mfa_recovery_codes_replaced", "user", u.ID, nil)
	writeJSON(w, http.StatusOK, map[string]any{"recovery_codes": codes})
}

func (s *Server) mfaForgetBrowsers(w http.ResponseWriter, r *http.Request) {
	u := who(r).User
	if err := s.store.ForgetTrustedBrowsers(r.Context(), u.ID); err != nil {
		fail(w, err)
		return
	}
	s.setCookie(w, trustedCookie, "", "/api/auth", time.Time{})
	s.record(r, "auth.mfa_browsers_forgotten", "user", u.ID, nil)
	ok(w)
}

// resetUserMFA lets an owner or admin turn off two-step sign-in for someone
// who lost their phone and recovery codes. It also signs them out.
func (s *Server) resetUserMFA(w http.ResponseWriter, r *http.Request) {
	org, id, me := orgOf(r), chi.URLParam(r, "id"), who(r).User
	if id == me.ID {
		fail(w, badRequest("turn off your own two-step sign-in from Account security"))
		return
	}
	target, err := s.store.GetUser(r.Context(), org, id)
	if err != nil {
		fail(w, err)
		return
	}
	if !canManageRole(me.Role, target.Role) {
		writeErr(w, http.StatusForbidden, "only owners can reset an owner's two-step sign-in")
		return
	}
	if err := s.store.DisableMFA(r.Context(), id); err != nil {
		fail(w, err)
		return
	}
	_ = s.store.DeleteUserSessions(r.Context(), id)
	s.record(r, "user.mfa_reset", "user", id, map[string]any{"email": target.Email})
	ok(w)
}
