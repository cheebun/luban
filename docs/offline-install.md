# Offline Installation

A router is often set up in a location where the internet is not yet working —
the router itself is what provides that connection. The offline bundle packages
every apt dependency, the SmartDNS binary, and the luban payload into a single
archive that can be transferred from any machine that does have internet access
and installed without any network calls.

## Overview

The process has two stages:

1. **Build the bundle** on a machine that has internet and matches the target's
   OS release and CPU architecture.
2. **Transfer and install** on the router using the bundle.

---

## Step 1: Build the bundle

### Builder requirements

The builder must match the target router on two dimensions:

- **Debian release**: the bundle embeds `.deb` files; a Debian 13 bundle
  installs cleanly on a Debian 13 router but will likely fail on Debian 12
  due to differing library versions.
- **CPU architecture**: an `amd64` bundle cannot be used on an `arm64` router.

For amd64 targets the Minisforum MS-A2 test VM (Debian 13, amd64) is suitable.
For arm64 targets use an arm64 Debian 13 system.

The build script must run as root (it needs `apt-get` to resolve and download
the package dependency closure).

### Option A: bundle a pre-built release tarball

If you already have a luban release tarball (e.g. downloaded from GitHub
Releases), pass it with `--release`:

```bash
sudo bash scripts/make-offline-bundle.sh \
    --release luban-v1.2.3-linux-amd64.tar.gz \
    --version v1.2.3 \
    --output /tmp/bundles
```

This produces `/tmp/bundles/luban-offline-v1.2.3-amd64.tar.gz`.

### Option B: build the luban payload from the repository

If you have the source tree and want to build the payload on the fly, use
`--from-repo`. Go and pnpm must be installed on the builder:

```bash
# From the repo root:
sudo bash scripts/make-offline-bundle.sh \
    --from-repo \
    --version v1.2.3 \
    --output /tmp/bundles
```

The script runs `go build` in `router/api/` and `pnpm --filter @router/web
build` to produce the `luban` binary and the `www/` frontend assets.

### Dependency-closure caveat

`make-offline-bundle.sh` uses `apt-get install --download-only --reinstall`
to download all packages and their full transitive dependency closure. The
`--reinstall` flag forces download of every package regardless of whether the
builder already has it installed. However, apt's dep solver still uses the
builder's installed-package database to determine which transitive deps are
already satisfied, and on a heavily-loaded desktop builder some deps of deps
may be skipped. Mitigation: run on a minimal Debian install (the PVE VM
works; avoid full desktop installs) to keep the builder close to the target.

---

## Step 2: Transfer the bundle to the router

Use the `tar | ssh` pattern described in the main deployment docs, or plain
`scp` if the bundle is small enough:

```bash
# tar + ssh (preferred — streams directly, no intermediate file on router)
scp luban-offline-v1.2.3-amd64.tar.gz root@<router-ip>:/tmp/
```

Or if the router is behind another host and you need a jump:

```bash
scp -o ProxyJump=<jumphost> luban-offline-v1.2.3-amd64.tar.gz root@<router-ip>:/tmp/
```

---

## Step 3: Install on the router

On the router (as root), extract the bundle and run install.sh with
`--offline` pointing at the extracted directory:

```bash
cd /tmp
tar xzf luban-offline-v1.2.3-amd64.tar.gz
bash luban-offline-v1.2.3-amd64/install.sh --offline luban-offline-v1.2.3-amd64
```

`install.sh` skips all network operations in offline mode:
- Caddy apt repo setup is skipped.
- All apt packages are installed from the bundle's `debs/` directory in a
  single transaction (apt handles dependency ordering for local `.deb` files).
- The SmartDNS binary is extracted from the bundle's `smartdns/` directory.
- The luban payload (`luban`, `www/`, `templates/`, `boards/`, `systemd/`) is
  copied from the bundle's `luban/` directory.

Everything after package installation — systemd unit setup, Caddyfile
configuration, resolved masking, networkd enablement, and idempotency rules —
is identical to the online installer.

---

## Bundle layout

```
luban-offline-<version>-<arch>.tar.gz
└── luban-offline-<version>-<arch>/
    ├── debs/                   # all apt packages + full dep closure (.deb files)
    ├── smartdns/               # SmartDNS GitHub release tarball (one file)
    ├── luban/                  # luban payload
    │   ├── luban               # Go backend binary
    │   ├── www/                # pre-built frontend (Vite output)
    │   ├── templates/          # service config templates
    │   ├── boards/             # board profiles
    │   └── systemd/            # systemd units + router-rollback.sh
    ├── manifest.txt            # versions + sha256sums
    ├── INSTALL-NOTE.txt        # quick-start reminder
    └── install.sh              # copy of install.sh for convenience
```

---

## Upgrading an offline-installed router

To upgrade a router that was installed from an offline bundle:

1. Build a new bundle with the new luban release.
2. Transfer the new bundle to the router.
3. Run `install.sh --offline <new-bundle-dir>` again.

The upgrade is safe and idempotent:
- `config.json` is never overwritten by install.sh — your settings are preserved.
- `boards.d/` (user-added board profiles) is never overwritten.
- The `luban` binary, `www/`, `templates/`, and `boards/` are replaced with the
  new version.
- All systemd units are reinstalled and reloaded.

If the new release contains breaking config-schema changes, the `luban` binary
handles migration on first start (see the main upgrade documentation when
that feature lands).

---

## Troubleshooting

**apt fails with "dependency is not satisfiable"**

The bundle was built on a different Debian release than the target. Rebuild
the bundle on a system matching the target's `lsb_release -cs` output.

**SmartDNS tarball not found in bundle**

The bundle's `smartdns/` directory is empty or contains a tarball for a
different architecture. Rebuild the bundle on a machine with the matching
architecture.

**install.sh exits with "Release missing: luban"**

The bundle's `luban/` directory is missing the binary. This can happen if
`--from-repo` was used and `go build` failed silently, or if the release
tarball passed to `--release` was for a different architecture. Check
`luban/luban` exists in the extracted bundle before installing.
