package api

import (
	"net/http"
	"net/mail"
	"strings"
	"sync"
	"time"

	"github.com/uem/uem/internal/server/audit"
	"github.com/uem/uem/internal/server/auth"
	"github.com/uem/uem/internal/server/store"
)

const minPasswordLen = 10

type meResponse struct {
	User        store.User `json:"user"`
	Org         store.Org  `json:"org"`
	Permissions []string   `json:"permissions"`
}

func validEmail(e string) bool {
	a, err := mail.ParseAddress(e)
	return err == nil && a.Address == e
}

func validatePassword(pw string) error {
	if len(pw) < minPasswordLen {
		return badRequest("password must be at least 10 characters")
	}
	if len(pw) > 256 {
		return badRequest("password is too long")
	}
	return nil
}

func (s *Server) signup(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.AllowSignup {
		writeErr(w, http.StatusForbidden, "sign-up is disabled on this server")
		return
	}
	var req struct {
		OrgName  string `json:"org_name"`
		Name     string `json:"name"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decode(r, &req); err != nil {
		fail(w, err)
		return
	}
	req.Email = strings.TrimSpace(req.Email)
	req.OrgName = strings.TrimSpace(req.OrgName)
	if req.OrgName == "" || !validEmail(req.Email) {
		fail(w, badRequest("organization name and a valid email are required"))
		return
	}
	if err := validatePassword(req.Password); err != nil {
		fail(w, err)
		return
	}
	if !s.loginLimiter.allow("signup:" + clientIP(r)) {
		writeErr(w, http.StatusTooManyRequests, "too many attempts, try again later")
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		fail(w, err)
		return
	}
	org, user, err := s.store.CreateOrgWithOwner(r.Context(), req.OrgName, strings.TrimSpace(req.Name), req.Email, hash)
	if err != nil {
		if err == store.ErrConflict {
			writeErr(w, http.StatusConflict, "an account with that email already exists")
			return
		}
		fail(w, err)
		return
	}
	s.audit.Record(r.Context(), audit.Event{OrgID: org.ID,
		Actor:  audit.Actor{Type: "user", ID: user.ID, Email: user.Email, IP: clientIP(r)},
		Action: "org.create", TargetType: "org", TargetID: org.ID, Details: map[string]any{"name": org.Name}})
	s.startSession(w, r, user, org)
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decode(r, &req); err != nil {
		fail(w, err)
		return
	}
	key := clientIP(r) + "|" + strings.ToLower(strings.TrimSpace(req.Email))
	if !s.loginLimiter.allow(key) {
		writeErr(w, http.StatusTooManyRequests, "too many failed attempts, try again later")
		return
	}
	user, err := s.store.GetUserByEmail(r.Context(), strings.TrimSpace(req.Email))
	if err != nil {
		auth.DummyCheck(req.Password)
		writeErr(w, http.StatusUnauthorized, "invalid email or password")
		return
	}
	if err := auth.CheckPassword(user.PasswordHash, req.Password); err != nil {
		s.audit.Record(r.Context(), audit.Event{OrgID: user.OrgID,
			Actor:  audit.Actor{Type: "user", ID: user.ID, Email: user.Email, IP: clientIP(r)},
			Action: "auth.login_failed", TargetType: "user", TargetID: user.ID})
		writeErr(w, http.StatusUnauthorized, "invalid email or password")
		return
	}
	s.loginLimiter.reset(key)
	org, err := s.store.GetOrg(r.Context(), user.OrgID)
	if err != nil {
		fail(w, err)
		return
	}
	_ = s.store.TouchLogin(r.Context(), user.ID)
	s.audit.Record(r.Context(), audit.Event{OrgID: org.ID,
		Actor:  audit.Actor{Type: "user", ID: user.ID, Email: user.Email, IP: clientIP(r)},
		Action: "auth.login", TargetType: "user", TargetID: user.ID})
	s.startSession(w, r, user, org)
}

func (s *Server) startSession(w http.ResponseWriter, r *http.Request, user store.User, org store.Org) {
	token := auth.RandomToken(32)
	sess := store.Session{
		ID: auth.HashToken(token), UserID: user.ID, OrgID: org.ID,
		CSRFToken: auth.RandomToken(24), ExpiresAt: time.Now().Add(s.cfg.SessionTTL),
	}
	if err := s.store.CreateSession(r.Context(), sess, clientIP(r), r.UserAgent()); err != nil {
		fail(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: token, Path: "/", Expires: sess.ExpiresAt,
		HttpOnly: true, Secure: s.cfg.SecureCookies, SameSite: http.SameSiteStrictMode})
	// Readable by the SPA, which echoes it in X-CSRF-Token.
	http.SetCookie(w, &http.Cookie{Name: csrfCookie, Value: sess.CSRFToken, Path: "/", Expires: sess.ExpiresAt,
		Secure: s.cfg.SecureCookies, SameSite: http.SameSiteStrictMode})
	writeJSON(w, http.StatusOK, meResponse{User: user, Org: org, Permissions: auth.PermissionsFor(user.Role)})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	_ = s.store.DeleteSession(r.Context(), who(r).Session.ID)
	s.record(r, "auth.logout", "user", who(r).User.ID, nil)
	for _, name := range []string{sessionCookie, csrfCookie} {
		http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: "/", MaxAge: -1,
			Secure: s.cfg.SecureCookies, SameSite: http.SameSiteStrictMode})
	}
	ok(w)
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	p := who(r)
	org, err := s.store.GetOrg(r.Context(), p.Session.OrgID)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, meResponse{User: p.User, Org: org, Permissions: auth.PermissionsFor(p.User.Role)})
}

// limiter is a fixed-window attempt counter keyed by caller. In-memory, so
// limits are per server instance.
type limiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	hits   map[string]*window
}

type window struct {
	start time.Time
	n     int
}

func newLimiter(max int, w time.Duration) *limiter {
	return &limiter{max: max, window: w, hits: map[string]*window{}}
}

func (l *limiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if len(l.hits) > 100_000 { // bound memory under a flood of distinct keys
		l.hits = map[string]*window{}
	}
	h := l.hits[key]
	if h == nil || now.Sub(h.start) > l.window {
		h = &window{start: now}
		l.hits[key] = h
	}
	h.n++
	return h.n <= l.max
}

func (l *limiter) reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.hits, key)
}
