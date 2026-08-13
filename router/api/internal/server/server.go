// Package server wires up the HTTP mux and all API handlers.
package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"luban/internal/apply"
	"luban/internal/auth"
	"luban/internal/config"
	"luban/internal/health"
	"luban/internal/status"
	"net"
	"net/http"
	"os"
	"os/exec"
	"time"
)

// Server holds all dependencies shared across handlers.
type Server struct {
	store    *config.Store
	sessions *auth.Manager
	baseDir  string
}

// New constructs a Server. Call ListenAndServe to start it.
func New(store *config.Store, sessions *auth.Manager, baseDir string) *Server {
	return &Server{store: store, sessions: sessions, baseDir: baseDir}
}

// ListenAndServe binds the unix socket and serves HTTP until ctx is cancelled.
func (s *Server) ListenAndServe(ctx context.Context, sockPath string) error {
	_ = os.Remove(sockPath)
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		return err
	}
	if err := os.Chmod(sockPath, 0o660); err != nil { //nolint:gosec // G302: Unix socket at 0660 is intentional; Caddy (same group) must be able to connect
		slog.Warn("chmod socket", "err", err)
	}

	mux := http.NewServeMux()
	s.registerRoutes(mux)

	srv := &http.Server{
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()

	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutCtx)
	case err := <-errCh:
		return err
	}
}

func (s *Server) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/login", s.handleLogin)

	guarded := s.sessions.Middleware(http.HandlerFunc(s.routeGuarded))
	mux.Handle("/api/", guarded)
}

func (s *Server) routeGuarded(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/api/password":
		if r.Method == http.MethodPost {
			s.handlePassword(w, r)
		} else {
			methodNotAllowed(w)
		}
	case "/api/logout":
		if r.Method == http.MethodPost {
			s.handleLogout(w, r)
		} else {
			methodNotAllowed(w)
		}
	case "/api/status":
		if r.Method == http.MethodGet {
			s.handleStatus(w, r)
		} else {
			methodNotAllowed(w)
		}
	case "/api/config":
		switch r.Method {
		case http.MethodGet:
			s.handleConfigGet(w, r)
		case http.MethodPut:
			s.handleConfigPut(w, r)
		default:
			methodNotAllowed(w)
		}
	case "/api/apply":
		if r.Method == http.MethodPost {
			s.handleApply(w, r)
		} else {
			methodNotAllowed(w)
		}
	case "/api/confirm":
		if r.Method == http.MethodPost {
			s.handleConfirm(w, r)
		} else {
			methodNotAllowed(w)
		}
	case "/api/rollback":
		if r.Method == http.MethodPost {
			s.handleRollback(w, r)
		} else {
			methodNotAllowed(w)
		}
	case "/api/log":
		if r.Method == http.MethodGet {
			s.handleLog(w, r)
		} else {
			methodNotAllowed(w)
		}
	case "/api/reboot":
		if r.Method == http.MethodPost {
			s.handleReboot(w, r)
		} else {
			methodNotAllowed(w)
		}
	case "/api/health":
		if r.Method == http.MethodGet {
			s.handleHealth(w, r)
		} else {
			methodNotAllowed(w)
		}
	case "/api/service/restart":
		if r.Method == http.MethodPost {
			s.handleServiceRestart(w, r)
		} else {
			methodNotAllowed(w)
		}
	default:
		http.NotFound(w, r)
	}
}

// handleLogin accepts {"username":"admin","password":"..."} and sets a session cookie.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Username == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "username and password required")
		return
	}
	if req.Username != "admin" {
		slog.Warn("auth failure", "username", req.Username, "remote", r.RemoteAddr)
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	cfg := s.store.Get()
	if err := auth.CheckPassword(cfg.System.Admin.PasswordHash, req.Password); err != nil {
		slog.Warn("auth failure", "username", req.Username, "remote", r.RemoteAddr)
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if _, err := s.sessions.Login(w); err != nil {
		writeError(w, http.StatusInternalServerError, "session creation failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":          true,
		"must_change": cfg.System.Admin.MustChange,
	})
}

// handlePassword changes the admin password. Requires current password unless first-boot must_change.
func (s *Server) handlePassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.New == "" {
		writeError(w, http.StatusBadRequest, "new password required")
		return
	}

	cfg := s.store.Get()
	// Always verify the current password, even on first boot.
	if err := auth.CheckPassword(cfg.System.Admin.PasswordHash, req.Current); err != nil {
		slog.Warn("password change: wrong current password")
		writeError(w, http.StatusUnauthorized, "wrong current password")
		return
	}

	hash, err := auth.HashPassword(req.New)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "hash error")
		return
	}
	if err := s.store.SetField(func(c *config.Config) {
		c.System.Admin.PasswordHash = hash
		c.System.Admin.MustChange = false
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "save error")
		return
	}
	slog.Info("password changed")
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleLogout invalidates the caller's session server-side and clears the
// session cookie on the client.
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	s.sessions.Logout(w, r)
	slog.Info("logout")
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	st := status.Collect(r.Context(), "")
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) handleConfigGet(w http.ResponseWriter, _ *http.Request) {
	cfg := s.store.Get()
	writeJSON(w, http.StatusOK, cfg)
}

func (s *Server) handleConfigPut(w http.ResponseWriter, r *http.Request) {
	var cfg config.Config
	if err := readJSON(r, &cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid config JSON")
		return
	}
	if err := config.Validate(&cfg); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	// Preserve the password hash — clients never send it back.
	existing := s.store.Get()
	cfg.System.Admin.PasswordHash = existing.System.Admin.PasswordHash
	cfg.System.Admin.MustChange = existing.System.Admin.MustChange

	if err := s.store.Set(cfg); err != nil {
		writeError(w, http.StatusInternalServerError, "save error: "+err.Error())
		return
	}
	slog.Info("config updated")
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleApply(w http.ResponseWriter, r *http.Request) {
	cfg := s.store.Get()
	data, err := apply.BuildTemplateData(cfg)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "build template data: "+err.Error())
		return
	}
	if err := apply.Pipeline(r.Context(), s.baseDir, data); err != nil {
		slog.Error("apply failed", "err", err)
		writeError(w, http.StatusInternalServerError, "apply failed: "+err.Error())
		return
	}
	slog.Info("apply triggered")
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleConfirm(w http.ResponseWriter, r *http.Request) {
	if err := apply.Confirm(r.Context(), s.baseDir); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleRollback(w http.ResponseWriter, r *http.Request) {
	if err := apply.Rollback(r.Context(), s.baseDir); err != nil {
		slog.Error("rollback failed", "err", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	slog.Info("rollback complete via API")
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleLog(w http.ResponseWriter, r *http.Request) {
	lines := r.URL.Query().Get("lines")
	if lines == "" {
		lines = "200"
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "journalctl", "-u", "router-ui", "-n", lines, "--no-pager", "--output=short-iso") //nolint:gosec // G702: lines is passed as a distinct arg to exec (no shell expansion); journalctl treats it as a count, not a command
	out, err := cmd.Output()
	if err != nil {
		// journald may not be present in dev — return empty log rather than error
		writeJSON(w, http.StatusOK, map[string]string{"log": ""})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"log": string(out)})
}

func (s *Server) handleReboot(w http.ResponseWriter, _ *http.Request) {
	slog.Info("reboot requested via API")
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	// Flush the response before rebooting.
	if fl, ok := w.(http.Flusher); ok {
		fl.Flush()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = exec.CommandContext(ctx, "systemctl", "reboot").Run()
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	h := health.Collect(r.Context())
	writeJSON(w, http.StatusOK, h)
}

// handleServiceRestart restarts one service, guarded by health.RestartableServices
// (a hard allowlist — never derived from request data beyond a membership check).
// Restarting "router-ui" (this process) is a special case: we must respond to
// the caller before the process is torn down by systemctl, otherwise the
// restart kills the HTTP write mid-flight and the client sees a connection
// reset instead of a clean 200. So for that one name we write the response
// first, flush it, then fire the actual `systemctl restart router-ui` from a
// short-delayed goroutine after the handler returns.
func (s *Server) handleServiceRestart(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !health.RestartableServices[req.Name] {
		writeError(w, http.StatusBadRequest, "service not in restart allowlist")
		return
	}

	if req.Name == "router-ui" {
		slog.Info("service restart requested", "name", req.Name, "self", true)
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		if fl, ok := w.(http.Flusher); ok {
			fl.Flush()
		}
		go func() { //nolint:gosec // G118: context.Background is deliberate — the request context is cancelled before this goroutine runs
			time.Sleep(500 * time.Millisecond)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := health.RestartService(ctx, "router-ui"); err != nil {
				slog.Error("self-restart failed", "err", err)
			}
		}()
		return
	}

	slog.Info("service restart requested", "name", req.Name)
	if err := health.RestartService(r.Context(), req.Name); err != nil {
		slog.Error("service restart failed", "name", req.Name, "err", err)
		writeError(w, http.StatusInternalServerError, "restart failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func readJSON(r *http.Request, dst interface{}) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return err
	}
	return json.Unmarshal(body, dst)
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func methodNotAllowed(w http.ResponseWriter) {
	writeError(w, http.StatusMethodNotAllowed, "method not allowed")
}
