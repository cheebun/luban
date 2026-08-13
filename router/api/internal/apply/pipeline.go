package apply

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	// rollbackFlagName is created by Apply and removed by Confirm, inside baseDir.
	// If present at boot the rollback timer unit restores the backup. It must live
	// on persistent storage (not /run, which is tmpfs) so a power-loss before the
	// confirm window closes is still caught by the boot-time rollback check —
	// see router/systemd/router-rollback.sh.
	rollbackFlagName = "unconfirmed-apply"
	rollbackTimer    = "router-rollback.timer"
	cmdTimeout       = 15 * time.Second
)

// rollbackFlagPath returns the flag file path for the given base directory.
func rollbackFlagPath(baseDir string) string {
	return filepath.Join(baseDir, rollbackFlagName)
}

// Pipeline executes the apply sequence: backup → render → dry-run → write → reload services.
// baseDir is /opt/router (or the flag-overridden value).
func Pipeline(ctx context.Context, baseDir string, data TemplateData) error {
	cfgPath := filepath.Join(baseDir, "config.json")

	// Step 1: backup config.json
	if err := copyFile(cfgPath, cfgPath+".bak"); err != nil {
		return fmt.Errorf("backup config.json: %w", err)
	}

	// Step 2: render templates to temp dir
	tmpDir, entries, err := RenderAll(baseDir, data)
	if err != nil {
		return fmt.Errorf("render templates: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Step 3: dry-run validation
	if err := dryRunValidate(ctx, tmpDir); err != nil {
		return fmt.Errorf("dry-run validation: %w", err)
	}

	// Step 4: backup current generated files, then write new ones
	if err := installRendered(tmpDir, entries); err != nil {
		return fmt.Errorf("install rendered files: %w", err)
	}

	// Step 5: reload services (networkctl before pppd)
	if err := reloadServices(ctx, data.IsPPPoE); err != nil {
		// non-fatal log — the files are written; partial reload is better than a hard error
		slog.Error("service reload after apply", "err", err)
	}

	// Step 6: write unconfirmed-apply flag and arm the rollback timer
	flagPath := rollbackFlagPath(baseDir)
	_ = os.MkdirAll(baseDir, 0755)
	if err := os.WriteFile(flagPath, []byte("1"), 0644); err != nil {
		slog.Warn("write rollback flag", "err", err)
	}
	if err := runCmd(ctx, "systemctl", "start", rollbackTimer); err != nil {
		slog.Warn("start rollback timer", "err", err)
	}

	slog.Info("apply complete; waiting for confirm")
	return nil
}

// Confirm removes the rollback flag and stops the timer.
func Confirm(ctx context.Context, baseDir string) error {
	_ = os.Remove(rollbackFlagPath(baseDir))
	if err := runCmd(ctx, "systemctl", "stop", rollbackTimer); err != nil {
		slog.Warn("stop rollback timer", "err", err)
	}
	slog.Info("apply confirmed")
	return nil
}

// Rollback restores backed-up config files WITHOUT re-rendering templates.
// It restores .network files before restarting networkd.
func Rollback(ctx context.Context, baseDir string) error {
	cfgPath := filepath.Join(baseDir, "config.json")
	bakPath := cfgPath + ".bak"

	if _, err := os.Stat(bakPath); err != nil {
		return fmt.Errorf("no backup found at %s: %w", bakPath, err)
	}

	// Restore config.json
	if err := copyFile(bakPath, cfgPath); err != nil {
		return fmt.Errorf("restore config.json: %w", err)
	}

	// Restore generated files from their .bak counterparts, preserving the
	// backup's own mode (copyFile()'s 0644 default would strip chap-secrets
	// back down from 0600 on every rollback).
	for _, e := range renderTable {
		bak := e.OutputPath + ".bak"
		if _, err := os.Stat(bak); err == nil {
			if cpErr := copyFilePreserveMode(bak, e.OutputPath); cpErr != nil {
				slog.Warn("restore file", "path", e.OutputPath, "err", cpErr)
			}
		}
	}
	// Per-LAN-port files have dynamic names (see expandRenderTable) and aren't in
	// renderTable; discover their backups by glob, matching router-rollback.sh.
	if lanBaks, globErr := filepath.Glob("/etc/systemd/network/[0-9]*-lan-*.network.bak"); globErr == nil {
		for _, bak := range lanBaks {
			dst := strings.TrimSuffix(bak, ".bak")
			if cpErr := copyFilePreserveMode(bak, dst); cpErr != nil {
				slog.Warn("restore file", "path", dst, "err", cpErr)
			}
		}
	}

	// Reload networkd before restarting other services
	if err := runCmd(ctx, "networkctl", "reload"); err != nil {
		slog.Warn("networkctl reload on rollback", "err", err)
	}
	if err := runCmd(ctx, "systemctl", "restart", "systemd-networkd"); err != nil {
		slog.Warn("restart networkd on rollback", "err", err)
	}
	if err := runCmd(ctx, "systemctl", "restart", "dnsmasq", "smartdns", "nftables"); err != nil {
		slog.Warn("restart services on rollback", "err", err)
	}
	if err := runCmd(ctx, "systemctl", "restart", "caddy"); err != nil {
		slog.Warn("restart caddy on rollback", "err", err)
	}

	_ = os.Remove(rollbackFlagPath(baseDir))
	if err := runCmd(ctx, "systemctl", "stop", rollbackTimer); err != nil {
		slog.Warn("stop rollback timer", "err", err)
	}

	slog.Info("rollback complete")
	return nil
}

func dryRunValidate(ctx context.Context, tmpDir string) error {
	nftConf := filepath.Join(tmpDir, "etc/nftables.conf")
	if _, err := os.Stat(nftConf); err == nil {
		if err := runCmd(ctx, "nft", "-c", "-f", nftConf); err != nil {
			return fmt.Errorf("nft dry-run: %w", err)
		}
	}

	dnsmasqConf := filepath.Join(tmpDir, "etc/dnsmasq.d/router.conf")
	if _, err := os.Stat(dnsmasqConf); err == nil {
		if err := runCmd(ctx, "dnsmasq", "--test", "--conf-file="+dnsmasqConf); err != nil {
			return fmt.Errorf("dnsmasq dry-run: %w", err)
		}
	}
	return nil
}

func installRendered(tmpDir string, entries []renderEntry) error {
	for _, e := range entries {
		src := filepath.Join(tmpDir, filepath.FromSlash(e.OutputPath[1:]))
		if _, err := os.Stat(src); err != nil {
			continue // template not rendered (e.g. ppp0.network when not PPPoE)
		}
		// Backup the current live file before overwriting, preserving its mode
		// (e.g. chap-secrets is 0600 — a .bak with the copyFile() default of
		// 0644 would leak PPPoE credentials to any local user).
		if _, err := os.Stat(e.OutputPath); err == nil {
			if bakErr := copyFilePreserveMode(e.OutputPath, e.OutputPath+".bak"); bakErr != nil {
				slog.Warn("backup file", "path", e.OutputPath, "err", bakErr)
			}
		}
		if err := os.MkdirAll(filepath.Dir(e.OutputPath), 0755); err != nil {
			return fmt.Errorf("mkdir %s: %w", e.OutputPath, err)
		}
		if err := copyFileMode(src, e.OutputPath, e.Mode); err != nil {
			return fmt.Errorf("install %s: %w", e.OutputPath, err)
		}
	}
	return nil
}

func reloadServices(ctx context.Context, isPPPoE bool) error {
	// networkctl reload must come first so networkd re-matches ppp0 when it appears.
	if err := runCmd(ctx, "networkctl", "reload"); err != nil {
		return err
	}
	cmds := [][]string{
		{"systemctl", "restart", "systemd-networkd"},
		{"systemctl", "restart", "nftables"},
		{"systemctl", "restart", "dnsmasq"},
		{"systemctl", "restart", "smartdns"},
		{"systemctl", "restart", "caddy"},
		{"sysctl", "--system"},
	}
	// pppd is only enabled in PPPoE mode; restarting a disabled unit
	// every apply just logs spurious errors.
	if isPPPoE {
		cmds = append(cmds, []string{"systemctl", "restart", "pppd"})
	}
	for _, args := range cmds {
		if err := runCmd(ctx, args[0], args[1:]...); err != nil {
			slog.Warn("service reload", "cmd", args, "err", err)
		}
	}
	return nil
}

func runCmd(ctx context.Context, name string, args ...string) error {
	c, cancel := context.WithTimeout(ctx, cmdTimeout)
	defer cancel()
	cmd := exec.CommandContext(c, name, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run()
}

func copyFile(src, dst string) error {
	return copyFileMode(src, dst, 0644)
}

// copyFilePreserveMode copies src to dst using src's own current permission
// bits rather than a hardcoded default. Used for backup/restore pairs (e.g.
// chap-secrets) where the file's declared Mode (0600) must survive both the
// live→.bak backup and the .bak→live rollback restore.
func copyFilePreserveMode(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	return copyFileMode(src, dst, info.Mode().Perm())
}

func copyFileMode(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if mode == 0 {
		mode = 0644
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	_, cpErr := io.Copy(out, in)
	closeErr := out.Close()
	if cpErr != nil {
		return cpErr
	}
	return closeErr
}
