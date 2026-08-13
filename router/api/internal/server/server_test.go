package server_test

import (
	"context"
	"fmt"
	"luban/internal/auth"
	"luban/internal/config"
	"luban/internal/server"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestServer_StartsWithNoConfigPresent reproduces the fresh-install scenario:
// baseDir exists but has no config.json (install.sh must never write one).
// NewStore should seed a valid default, and the server should come up and
// serve requests over the unix socket without error.
func TestServer_StartsWithNoConfigPresent(t *testing.T) {
	baseDir := t.TempDir()
	if _, err := os.Stat(filepath.Join(baseDir, "config.json")); !os.IsNotExist(err) {
		t.Fatalf("test setup: config.json unexpectedly present")
	}

	store, err := config.NewStore(baseDir)
	if err != nil {
		t.Fatalf("NewStore on empty baseDir: %v", err)
	}

	sessions := auth.NewManager()
	srv := server.New(store, sessions, baseDir)

	// Use a short path directly under the system temp root: t.TempDir() nests
	// several directories deep, and on macOS/BSD sockaddr_un caps sun_path at
	// ~104 bytes, so a nested path can silently fail to bind.
	sockPath := filepath.Join(os.TempDir(), fmt.Sprintf("luban-test-%d.sock", os.Getpid()))
	_ = os.Remove(sockPath)
	defer func() { _ = os.Remove(sockPath) }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe(ctx, sockPath) }()

	// Wait for the socket to appear (or bail out immediately if ListenAndServe failed).
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(sockPath); err == nil {
			break
		}
		select {
		case err := <-errCh:
			t.Fatalf("ListenAndServe exited early: %v", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("socket %s never appeared", sockPath)
		}
		time.Sleep(20 * time.Millisecond)
	}

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", sockPath)
			},
		},
		Timeout: 5 * time.Second,
	}

	resp, err := client.Post("http://unix/api/login", "application/json",
		strings.NewReader(`{"username":"admin","password":"wrong"}`))
	if err != nil {
		t.Fatalf("request over unix socket failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("login with wrong password: status = %d; want %d", resp.StatusCode, http.StatusUnauthorized)
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("ListenAndServe returned error after shutdown: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not shut down in time")
	}
}

// TestServer_LoginRefreshesCookieAndLogoutRevokes exercises the full sliding
// session lifecycle: a successful login gets a session cookie, every
// subsequent authenticated request re-issues that cookie with a fresh
// Max-Age (so activity — not a fixed post-login timer — keeps the user
// logged in), and /api/logout revokes the session so the old cookie is
// rejected afterward.
func TestServer_LoginRefreshesCookieAndLogoutRevokes(t *testing.T) {
	baseDir := t.TempDir()
	store, err := config.NewStore(baseDir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	hash, err := auth.HashPassword("test-pass")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if err := store.SetField(func(c *config.Config) {
		c.System.Admin.PasswordHash = hash
		c.System.Admin.MustChange = false
	}); err != nil {
		t.Fatalf("SetField: %v", err)
	}

	sessions := auth.NewManager()
	srv := server.New(store, sessions, baseDir)

	sockPath := filepath.Join(os.TempDir(), fmt.Sprintf("luban-test-logout-%d.sock", os.Getpid()))
	_ = os.Remove(sockPath)
	defer func() { _ = os.Remove(sockPath) }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe(ctx, sockPath) }()

	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(sockPath); err == nil {
			break
		}
		select {
		case err := <-errCh:
			t.Fatalf("ListenAndServe exited early: %v", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("socket %s never appeared", sockPath)
		}
		time.Sleep(20 * time.Millisecond)
	}

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", sockPath)
			},
		},
		Timeout: 5 * time.Second,
	}

	loginResp, err := client.Post("http://unix/api/login", "application/json",
		strings.NewReader(`{"username":"admin","password":"test-pass"}`))
	if err != nil {
		t.Fatalf("login request: %v", err)
	}
	defer func() { _ = loginResp.Body.Close() }()
	if loginResp.StatusCode != http.StatusOK {
		t.Fatalf("login: status = %d; want 200", loginResp.StatusCode)
	}
	loginCookies := loginResp.Cookies()
	if len(loginCookies) == 0 {
		t.Fatal("login: no Set-Cookie in response")
	}
	sessionCookie := loginCookies[0]

	statusReq, err := http.NewRequest(http.MethodGet, "http://unix/api/status", nil)
	if err != nil {
		t.Fatalf("build status request: %v", err)
	}
	statusReq.AddCookie(sessionCookie)
	statusResp, err := client.Do(statusReq)
	if err != nil {
		t.Fatalf("status request: %v", err)
	}
	defer func() { _ = statusResp.Body.Close() }()
	if statusResp.StatusCode != http.StatusOK {
		t.Fatalf("status: status = %d; want 200", statusResp.StatusCode)
	}
	refreshedCookies := statusResp.Cookies()
	if len(refreshedCookies) == 0 {
		t.Fatal("status: expected a refreshed Set-Cookie on authenticated response, got none")
	}
	refreshed := refreshedCookies[0]
	if refreshed.Value != sessionCookie.Value {
		t.Errorf("refreshed cookie token mismatch: got %q want %q", refreshed.Value, sessionCookie.Value)
	}
	if refreshed.MaxAge != 1800 {
		t.Errorf("refreshed cookie MaxAge = %d, want 1800", refreshed.MaxAge)
	}

	logoutReq, err := http.NewRequest(http.MethodPost, "http://unix/api/logout", nil)
	if err != nil {
		t.Fatalf("build logout request: %v", err)
	}
	logoutReq.AddCookie(sessionCookie)
	logoutResp, err := client.Do(logoutReq)
	if err != nil {
		t.Fatalf("logout request: %v", err)
	}
	defer func() { _ = logoutResp.Body.Close() }()
	if logoutResp.StatusCode != http.StatusOK {
		t.Fatalf("logout: status = %d; want 200", logoutResp.StatusCode)
	}

	postLogoutReq, err := http.NewRequest(http.MethodGet, "http://unix/api/status", nil)
	if err != nil {
		t.Fatalf("build post-logout status request: %v", err)
	}
	postLogoutReq.AddCookie(sessionCookie)
	postLogoutResp, err := client.Do(postLogoutReq)
	if err != nil {
		t.Fatalf("post-logout status request: %v", err)
	}
	defer func() { _ = postLogoutResp.Body.Close() }()
	if postLogoutResp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status after logout: status = %d; want 401", postLogoutResp.StatusCode)
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("ListenAndServe returned error after shutdown: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not shut down in time")
	}
}
