// Package auth handles password verification and session management.
package auth

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	sessionCookieName = "session"
	sessionTTL        = 30 * time.Minute
	bcryptCost        = 12
)

type session struct {
	createdAt  time.Time
	lastActive time.Time
}

// Manager holds the in-memory session table.
type Manager struct {
	mu       sync.Mutex
	sessions map[string]*session
}

// NewManager returns an initialized session manager.
func NewManager() *Manager {
	m := &Manager{sessions: make(map[string]*session)}
	go m.reap()
	return m
}

// HashPassword returns a bcrypt hash of the plaintext password.
func HashPassword(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcryptCost)
	return string(b), err
}

// CheckPassword returns nil if plain matches the stored hash.
func CheckPassword(hash, plain string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain))
}

// Login creates a new session and sets the session cookie on w.
func (m *Manager) Login(w http.ResponseWriter) (token string, err error) {
	token, err = newToken()
	if err != nil {
		return "", err
	}
	now := time.Now()
	m.mu.Lock()
	m.sessions[token] = &session{createdAt: now, lastActive: now}
	m.mu.Unlock()

	setSessionCookie(w, token, int(sessionTTL.Seconds()))
	return token, nil
}

// Logout invalidates the session server-side and clears the cookie so the
// browser drops it immediately.
func (m *Manager) Logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookieName); err == nil {
		m.mu.Lock()
		delete(m.sessions, c.Value)
		m.mu.Unlock()
	}
	// Replace any cookie already queued on this response (e.g. the sliding
	// refresh written by Middleware) so the client only ever sees the
	// expiring one.
	w.Header().Del("Set-Cookie")
	setSessionCookie(w, "", -1)
}

// setSessionCookie writes the session cookie with a fresh Max-Age. Passing
// maxAge -1 clears it (used by Logout).
func setSessionCookie(w http.ResponseWriter, token string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   maxAge,
	})
}

// Validate checks the session cookie and slides the activity window.
// Returns true and the token if the session is valid.
func (m *Manager) Validate(r *http.Request) (string, bool) {
	c, err := r.Cookie(sessionCookieName)
	if err != nil {
		return "", false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[c.Value]
	if !ok {
		return "", false
	}
	if time.Since(s.lastActive) > sessionTTL {
		delete(m.sessions, c.Value)
		return "", false
	}
	s.lastActive = time.Now()
	return c.Value, true
}

// Middleware returns an HTTP handler that requires a valid session.
// On success it re-issues the session cookie with a fresh Max-Age so an
// active user is kept logged in — only 30 minutes of inactivity expires the
// session, since the browser's own MaxAge countdown from Login would
// otherwise hard-expire it regardless of activity.
// On failure it returns 401 JSON so the frontend can redirect to login.
func (m *Manager) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := m.Validate(r)
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
			return
		}
		setSessionCookie(w, token, int(sessionTTL.Seconds()))
		next.ServeHTTP(w, r)
	})
}

// reap cleans up expired sessions every minute.
func (m *Manager) reap() {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for range t.C {
		m.mu.Lock()
		for token, s := range m.sessions {
			if time.Since(s.lastActive) > sessionTTL {
				delete(m.sessions, token)
			}
		}
		m.mu.Unlock()
	}
}

func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
