// Command rui is the luban router UI backend.
// It listens on a unix socket and serves the /api/* endpoints.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"luban/internal/auth"
	"luban/internal/config"
	"luban/internal/server"
)

func main() {
	sockPath := flag.String("socket", "/run/router/api.sock", "unix socket path")
	baseDir := flag.String("basedir", "/opt/router", "base directory (contains config.json, templates/, boards/)")
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	store, err := config.NewStore(*baseDir)
	if err != nil {
		slog.Error("load config", "err", err)
		os.Exit(1)
	}

	// On first boot, the default config has an empty password hash.
	// Generate the bcrypt hash of "password" so the first login works.
	ensureDefaultPassword(store)

	sessions := auth.NewManager()
	srv := server.New(store, sessions, *baseDir)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	slog.Info("starting", "socket", *sockPath, "basedir", *baseDir)
	if err := srv.ListenAndServe(ctx, *sockPath); err != nil {
		slog.Error("serve error", "err", err)
		os.Exit(1)
	}
}

func ensureDefaultPassword(store *config.Store) {
	cfg := store.Get()
	if cfg.System.Admin.PasswordHash != "" {
		return
	}
	hash, err := auth.HashPassword("password")
	if err != nil {
		slog.Error("hash default password", "err", err)
		return
	}
	if err := store.SetField(func(c *config.Config) {
		c.System.Admin.PasswordHash = hash
		c.System.Admin.MustChange = true
	}); err != nil {
		slog.Error("save default password", "err", err)
	}
	slog.Info("first-boot: default admin password set; user must change on first login")
}
