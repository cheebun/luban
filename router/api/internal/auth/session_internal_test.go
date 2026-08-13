package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestMiddleware_RefreshesCookieOnActivity verifies that every successful
// validation re-issues the session cookie with a fresh Max-Age, so an
// active user stays logged in past the original Login TTL window.
func TestMiddleware_RefreshesCookieOnActivity(t *testing.T) {
	m := NewManager()

	loginRec := httptest.NewRecorder()
	token, err := m.Login(loginRec)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	cookies := loginRec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("no cookie set after login")
	}

	handler := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	req.AddCookie(cookies[0])
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	refreshed := rec.Result().Cookies()
	if len(refreshed) != 1 {
		t.Fatalf("expected exactly one refreshed Set-Cookie, got %d", len(refreshed))
	}
	if refreshed[0].Value != token {
		t.Errorf("refreshed cookie token mismatch: got %q want %q", refreshed[0].Value, token)
	}
	if refreshed[0].MaxAge != int(sessionTTL.Seconds()) {
		t.Errorf("refreshed cookie MaxAge = %d, want %d", refreshed[0].MaxAge, int(sessionTTL.Seconds()))
	}
}

// TestMiddleware_ExpiresAfterInactivity verifies that a request arriving
// after more than sessionTTL of inactivity is rejected with 401, even
// though the browser-side cookie itself hasn't expired.
func TestMiddleware_ExpiresAfterInactivity(t *testing.T) {
	m := NewManager()

	loginRec := httptest.NewRecorder()
	token, err := m.Login(loginRec)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	// Simulate inactivity beyond the TTL by rewinding lastActive directly.
	m.mu.Lock()
	m.sessions[token].lastActive = time.Now().Add(-sessionTTL - time.Minute)
	m.mu.Unlock()

	handler := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token}) //nolint:gosec // G124: unit-test cookie; Secure/HttpOnly not applicable outside a real TLS connection
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 after >TTL inactivity, got %d", rec.Code)
	}
}

// TestLogout_ClearsAnyQueuedRefreshCookie verifies that Logout's expiring
// cookie wins even if a refresh cookie was already queued on the same
// response (as Middleware does before dispatching to the handler).
func TestLogout_ClearsAnyQueuedRefreshCookie(t *testing.T) {
	m := NewManager()

	loginRec := httptest.NewRecorder()
	token, err := m.Login(loginRec)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/logout", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token}) //nolint:gosec // G124: unit-test cookie; Secure/HttpOnly not applicable outside a real TLS connection

	rec := httptest.NewRecorder()
	// Simulate the middleware's refresh already having queued a Set-Cookie.
	setSessionCookie(rec, token, int(sessionTTL.Seconds()))
	m.Logout(rec, req)

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected exactly one Set-Cookie after logout, got %d", len(cookies))
	}
	if cookies[0].MaxAge >= 0 {
		t.Errorf("expected expiring cookie (MaxAge < 0), got %d", cookies[0].MaxAge)
	}

	if _, ok := m.Validate(req); ok {
		t.Error("session should be invalidated after logout")
	}
}
