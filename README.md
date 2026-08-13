# luban (鲁班)

A self-built Linux router for Debian/Armbian boards, OpenWrt-inspired: one JSON
config source, generated service configs, and an apply → confirm → auto-rollback
flow so a bad network change can never lock you out.

## Features

- **WAN modes**: DHCP, static IP, PPPoE (kernel-mode `rp-pppoe.so`, auto MTU 1492)
- **IPv6**: WAN RA + DHCPv6 prefix delegation, LAN SLAAC via RA, works over PPPoE too
- **DNS**: SmartDNS (resolution, upstream routing, cache) + dnsmasq (DHCP only, `port=0`)
- **Firewall/NAT**: nftables, dry-run validated before every apply
- **Anti-lockout rollback**: every apply starts a 90s confirm window; unconfirmed
  changes are automatically reverted by a systemd timer independent of the Go process
- **Web UI**: React admin console (Chinese UI), served by Caddy, talks to the Go
  backend over a unix socket

See [DECISIONS.md](DECISIONS.md) for the reasoning behind these choices.

## Architecture

```
Browser ──HTTPS──▶ Caddy ──┬─▶ static files (www/, React SPA)
                            └─▶ /api/* ──▶ unix:/run/router/api.sock ──▶ rui (Go, root)

config.json ──▶ templates/*.tpl ──▶ /etc/{dnsmasq,smartdns,nftables,...} + systemd-networkd
```

`config.json` is the single source of truth. The Go backend renders service
configs from Go `text/template` files read from disk, validates them
(`nft -c -f`, `dnsmasq --test`), writes them with a `.bak` backup of whatever
they replace, reloads services, and starts a 90-second confirm timer. If the
UI isn't reachable to confirm, `router-rollback.timer` restores the `.bak`
files and restarts services — no Go process required for rollback to work.

## Repository Layout

| Path | Contents |
|---|---|
| `router/api/` | Go backend (`rui` binary), Go module `luban` |
| `router/web/` | React + Vite + TypeScript frontend, package `@router/web` |
| `router/templates/` | Go `text/template` files for every generated service config |
| `router/boards/` | Board profiles (WAN/LAN/WiFi port mapping by device path) |
| `router/systemd/` | Unit files: `router-ui.service`, `router-rollback.{service,timer}`, `pppd.service` |
| `scripts/install.sh` | One-shot installer for a clean Debian/Armbian system |
| `docs/` | Supplementary docs (e.g. test environment setup) |

## Quick Start

On a clean Debian/Armbian board:

```bash
curl -fsSL https://github.com/<org>/luban/releases/latest/download/install.sh | sudo bash
```

or, from a downloaded release tarball:

```bash
sudo ./install.sh --local luban-<version>.tgz
```

The installer detects architecture, installs dependencies via `apt`, deploys
to `/opt/router/`, and enables the required systemd units. It is idempotent —
safe to re-run on an already-installed system — and never overwrites an
existing `config.json`.

After install, browse to `https://router.lan` (or the board's LAN IP). Log in
with `admin` / `password`; you will be forced to set a new password on first
login.

## Development

**Frontend** (`router/web/`):

```bash
pnpm install
pnpm --filter @router/web dev      # dev server
pnpm --filter @router/web build    # tsc -b && vite build → router/web/dist
pnpm --filter @router/web lint     # oxlint
```

Device builds are static: `vite build` output ships as `www/` in the release
tarball. The board never runs Node — see [AGENTS.md](AGENTS.md) for the
design-system rules the frontend must follow.

**Backend** (`router/api/`):

```bash
cd router/api
go build ./...
go test ./...
```

Tests include a real-template render test (`internal/apply/render_real_test.go`)
and a rollback contract test (`internal/apply/rollback_contract_test.go`) that
encode the cross-component contracts between the Go backend, the on-disk
templates, and the shell-based rollback script.

For rationale behind every architectural choice — platform, dependencies,
firewall baseline, IPv6 handling, the apply/rollback design — see
[DECISIONS.md](DECISIONS.md).
