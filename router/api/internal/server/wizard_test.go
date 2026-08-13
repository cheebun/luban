package server_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"luban/internal/apply"
	"luban/internal/auth"
	"luban/internal/config"
	"luban/internal/detect"
	"luban/internal/server"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// startWizardServer spins up a server with a no-op pipeline and a FakeProber,
// waits for the socket to appear, and returns a client wired to it.
// The caller owns cancelling ctx to shut the server down.
func startWizardServer(t *testing.T) (client *http.Client, sockPath string, store *config.Store) {
	t.Helper()

	baseDir := t.TempDir()
	var err error
	store, err = config.NewStore(baseDir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	// Pre-set a known password so we can log in.
	hash, err := auth.HashPassword("wizard-pass")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if err := store.SetField(func(c *config.Config) {
		c.System.Admin.PasswordHash = hash
		c.System.Admin.MustChange = false
	}); err != nil {
		t.Fatalf("SetField password: %v", err)
	}

	sessions := auth.NewManager()
	noopPipeline := func(_ context.Context, _ string, _ apply.TemplateData) error { return nil }
	srv := server.New(store, sessions, baseDir,
		server.WithProber(&detect.FakeProber{}),
		server.WithPipelineFn(noopPipeline),
	)

	sockPath = filepath.Join(os.TempDir(), fmt.Sprintf("luban-wizard-test-%d.sock", os.Getpid()))
	_ = os.Remove(sockPath)
	t.Cleanup(func() { _ = os.Remove(sockPath) })

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	go func() { _ = srv.ListenAndServe(ctx, sockPath) }()

	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(sockPath); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("socket %s never appeared", sockPath)
		}
		time.Sleep(20 * time.Millisecond)
	}

	client = &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", sockPath)
			},
		},
		Timeout: 10 * time.Second,
	}
	return client, sockPath, store
}

// loginWizard authenticates against the wizard server and returns the session cookie.
func loginWizard(t *testing.T, client *http.Client) *http.Cookie {
	t.Helper()
	resp, err := client.Post("http://unix/api/login", "application/json",
		strings.NewReader(`{"username":"admin","password":"wizard-pass"}`))
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login: status %d", resp.StatusCode)
	}
	if len(resp.Cookies()) == 0 {
		t.Fatal("login: no session cookie")
	}
	return resp.Cookies()[0]
}

// TestWizardComplete_ReturnsNewURL verifies that POST /api/wizard/complete
// returns HTTP 200 with exactly {"ok": true, "new_url": "https://<lan-ip>/"}
// where the IP is derived from the submitted lan_address.
func TestWizardComplete_ReturnsNewURL(t *testing.T) {
	client, _, _ := startWizardServer(t)
	cookie := loginWizard(t, client)

	body := `{
		"wan_interface":  "eth0",
		"lan_interfaces": ["eth1"],
		"lan_address":    "10.10.10.1/24",
		"password":       "new-secure-pass"
	}`
	req, err := http.NewRequest(http.MethodPost, "http://unix/api/wizard/complete",
		strings.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("wizard/complete: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("wizard/complete: status %d, body: %s", resp.StatusCode, raw)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal response: %v — body: %s", err, raw)
	}

	ok, _ := got["ok"].(bool)
	if !ok {
		t.Errorf(`response["ok"] = %v, want true`, got["ok"])
	}

	newURL, _ := got["new_url"].(string)
	if newURL == "" {
		t.Errorf(`response["new_url"] is missing or empty; full response: %s`, raw)
	}
	const wantURL = "https://10.10.10.1/"
	if newURL != wantURL {
		t.Errorf(`new_url = %q, want %q`, newURL, wantURL)
	}
}

// TestWizardComplete_ValidationErrors checks that missing required fields return
// 422 before the pipeline is ever reached.
func TestWizardComplete_ValidationErrors(t *testing.T) {
	client, _, _ := startWizardServer(t)
	cookie := loginWizard(t, client)

	cases := []struct {
		name string
		body string
		want int
	}{
		{
			name: "missing wan_interface",
			body: `{"lan_interfaces":["eth1"],"password":"pw"}`,
			want: http.StatusUnprocessableEntity,
		},
		{
			name: "missing lan_interfaces",
			body: `{"wan_interface":"eth0","password":"pw"}`,
			want: http.StatusUnprocessableEntity,
		},
		{
			name: "missing password",
			body: `{"wan_interface":"eth0","lan_interfaces":["eth1"]}`,
			want: http.StatusUnprocessableEntity,
		},
		{
			name: "wan in lan",
			body: `{"wan_interface":"eth0","lan_interfaces":["eth0"],"password":"pw"}`,
			want: http.StatusUnprocessableEntity,
		},
		{
			name: "invalid cidr",
			body: `{"wan_interface":"eth0","lan_interfaces":["eth1"],"lan_address":"not-a-cidr","password":"pw"}`,
			want: http.StatusUnprocessableEntity,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodPost, "http://unix/api/wizard/complete",
				strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			req.AddCookie(cookie)
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("request: %v", err)
			}
			_ = resp.Body.Close()
			if resp.StatusCode != tc.want {
				t.Errorf("status = %d, want %d", resp.StatusCode, tc.want)
			}
		})
	}
}
