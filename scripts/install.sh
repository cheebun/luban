#!/usr/bin/env bash
# install.sh — Turn a clean Debian/Armbian into a luban router.
# https://github.com/<GITHUB_REPO>/releases
#
# Usage:
#   sudo ./install.sh                        # download latest release from GitHub
#   sudo ./install.sh --local FILE.tgz       # install from a local tarball
#   sudo ./install.sh --offline BUNDLE_DIR   # offline: use pre-built bundle directory
#                                            #   (pass the extracted bundle dir, not the tarball)
#
# In --offline mode all network operations are skipped. Apt packages are installed
# from debs/ in the bundle, SmartDNS from smartdns/, and the luban payload from
# luban/ — no internet connection is required. All post-install configuration,
# systemd setup, and idempotency rules are identical across modes.
#
set -euo pipefail

##############################################################################
# PLACEHOLDER — update these before publishing a release
GITHUB_REPO="cheebun/luban"
SMARTDNS_REPO="pymumu/smartdns" # SmartDNS upstream GitHub repo (do not change)
##############################################################################

INSTALL_DIR="/opt/router"
CADDY_DROPIN_DIR="/etc/systemd/system/caddy.service.d"

LOCAL_TARBALL=""
OFFLINE_DIR=""   # path to extracted offline bundle directory

# ── helpers ───────────────────────────────────────────────────────────────────
info() { printf '\n[luban] %s\n' "$*"; }
ok()   { printf '[luban]   ok  %s\n' "$*"; }
warn() { printf '[luban] WARN  %s\n' "$*" >&2; }
die()  { printf '[luban] ERROR %s\n' "$*" >&2; exit 1; }

# ── argument parsing ──────────────────────────────────────────────────────────
usage() {
    echo "Usage: $0 [--local <tarball>] [--offline <bundle-dir>]"
    echo "  --local <tarball>      Install from a local tarball (skip GitHub download)"
    echo "  --offline <bundle-dir> Offline install from a pre-built bundle directory"
    exit 1
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --local)
            [[ -z "${2:-}" ]] && { echo "ERROR: --local requires a path argument"; usage; }
            LOCAL_TARBALL="$2"
            shift 2
            ;;
        --offline)
            [[ -z "${2:-}" ]] && { echo "ERROR: --offline requires a directory path"; usage; }
            OFFLINE_DIR="$2"
            shift 2
            ;;
        -h|--help) usage ;;
        *) echo "ERROR: Unknown argument: $1"; usage ;;
    esac
done

# Validate mutual exclusion: --offline and --local are incompatible.
if [[ -n "$OFFLINE_DIR" && -n "$LOCAL_TARBALL" ]]; then
    die "--offline and --local cannot be used together"
fi

if [[ -n "$OFFLINE_DIR" ]]; then
    [[ -d "$OFFLINE_DIR" ]]             || die "Offline bundle directory not found: $OFFLINE_DIR"
    [[ -d "$OFFLINE_DIR/debs" ]]        || die "Bundle missing debs/ directory: $OFFLINE_DIR/debs"
    [[ -d "$OFFLINE_DIR/smartdns" ]]    || die "Bundle missing smartdns/ directory: $OFFLINE_DIR/smartdns"
    [[ -d "$OFFLINE_DIR/luban" ]]       || die "Bundle missing luban/ directory: $OFFLINE_DIR/luban"
fi

# ── privilege check ───────────────────────────────────────────────────────────
[[ $EUID -eq 0 ]] || die "This script must be run as root."

# ── arch detection ────────────────────────────────────────────────────────────
ARCH_RAW="$(uname -m)"
case "$ARCH_RAW" in
    x86_64)  ARCH="amd64"  ;;
    aarch64) ARCH="arm64"  ;;
    *) die "Unsupported architecture: $ARCH_RAW" ;;
esac
info "Architecture: $ARCH ($ARCH_RAW)"

if [[ -n "$OFFLINE_DIR" ]]; then
    info "Mode: offline (bundle: $OFFLINE_DIR)"
else
    info "Mode: online"
fi

# ── temp workspace (cleaned up on exit) ──────────────────────────────────────
TMP_WORK="$(mktemp -d)"
trap 'rm -rf "$TMP_WORK"' EXIT

########################################################################
# STEP 0 — Ensure DNS resolution works before any curl/apt step
#
# Skipped in offline mode: no network calls are made during offline install,
# so DNS resolution is not required.
########################################################################
if [[ -z "$OFFLINE_DIR" ]]; then
    info "Step 0: Ensuring DNS resolution is available"

    if getent hosts deb.debian.org >/dev/null 2>&1; then
        ok "DNS resolution already working"
    else
        GATEWAY_IP="$(ip route show default 2>/dev/null | awk '/default/ {print $3; exit}')"
        TEMP_DNS="${GATEWAY_IP:-223.5.5.5}"
        printf 'nameserver %s\n' "$TEMP_DNS" > /etc/resolv.conf
        ok "Temporary resolver set to ${TEMP_DNS} for install-time DNS"
    fi
else
    info "Step 0: Skipped (offline mode — no DNS required)"
fi

########################################################################
# STEP 1 — Third-party apt repo: Caddy official
#
# Skipped in offline mode: the bundle's debs/ already contains the Caddy
# .deb and all of its dependencies. No repo configuration is needed.
########################################################################
if [[ -z "$OFFLINE_DIR" ]]; then
    info "Step 1: Third-party repos"

    if [[ ! -f /usr/share/keyrings/caddy-stable-archive-keyring.gpg ]]; then
        curl -fsSL https://dl.cloudsmith.io/public/caddy/stable/gpg.key \
            | gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
        echo "deb [signed-by=/usr/share/keyrings/caddy-stable-archive-keyring.gpg] \
https://dl.cloudsmith.io/public/caddy/stable/deb/debian any-version main" \
            > /etc/apt/sources.list.d/caddy-stable.list
        ok "Caddy apt repo added"
    else
        ok "Caddy apt repo already configured"
    fi
else
    info "Step 1: Skipped (offline mode — apt repos not needed)"
fi

########################################################################
# STEP 2 — apt install: runtime + base tooling + diagnostics + maintenance
#
# Online mode: fetch from apt repositories.
# Offline mode: install all packages from debs/ in the bundle in a single
# apt-get transaction. apt handles dpkg ordering automatically when given
# local .deb paths, avoiding the "dependency not satisfied" errors that
# plain `dpkg -i` produces on an out-of-order install.
########################################################################
info "Step 2: ${OFFLINE_DIR:+[offline] }apt install all dependencies"

# dnsmasq's postinst autostarts it immediately after install, with DNS
# listening on :53 by default. It would race smartdns for that port before
# our own config ever lands. Write a port=0 (DNS off) bootstrap config
# BEFORE installing the package so postinst picks it up on its first start.
# (mkdir -p /etc/dnsmasq.d works pre-install.)
#
# This bootstrap config is written directly at /etc/dnsmasq.d/router.conf —
# the SAME path the apply pipeline renders dnsmasq.conf.tpl to (see
# router/api/internal/apply/template.go renderTable). An earlier version of
# this script wrote a separate /etc/dnsmasq.d/00-luban.conf with its own
# `port=0`/DHCP directives; dnsmasq loads all of conf-dir, so that file and
# the apply-rendered router.conf both defined the same keywords and dnsmasq
# refused to start ("illegal repeated keyword") after every apply. Clean up
# that stale file from older installs.
rm -f /etc/dnsmasq.d/00-luban.conf

mkdir -p /etc/dnsmasq.d
DNSMASQ_BOOTSTRAP_HEADER="# luban bootstrap config, replaced by luban apply"
DNSMASQ_ROUTER_CONF="/etc/dnsmasq.d/router.conf"
if [[ -f "$DNSMASQ_ROUTER_CONF" ]] && ! grep -qF "$DNSMASQ_BOOTSTRAP_HEADER" "$DNSMASQ_ROUTER_CONF"; then
    # File exists and is NOT our bootstrap marker — apply already rendered
    # the real config here. Never clobber it on re-install.
    ok "dnsmasq config already rendered by apply — left untouched"
else
    cat > "$DNSMASQ_ROUTER_CONF" <<EOF
$DNSMASQ_BOOTSTRAP_HEADER
# An unconfigured system should not serve DHCP; this disables the DNS
# listener only. Full DHCP settings are rendered from
# /opt/router/config.json at apply time, overwriting this file.
port=0
EOF
    ok "dnsmasq bootstrap config written (pre-install)"
fi

DNSMASQ_DEFAULT="/etc/default/dnsmasq"
if [[ -f "$DNSMASQ_DEFAULT" ]]; then
    if ! grep -q "^IGNORE_RESOLVCONF=yes" "$DNSMASQ_DEFAULT"; then
        if grep -q "^#*IGNORE_RESOLVCONF" "$DNSMASQ_DEFAULT"; then
            sed -i 's/^#*IGNORE_RESOLVCONF=.*/IGNORE_RESOLVCONF=yes/' "$DNSMASQ_DEFAULT"
        else
            echo "IGNORE_RESOLVCONF=yes" >> "$DNSMASQ_DEFAULT"
        fi
    fi
else
    echo "IGNORE_RESOLVCONF=yes" > "$DNSMASQ_DEFAULT"
fi
ok "dnsmasq IGNORE_RESOLVCONF=yes set (pre-install)"

# The apply pipeline backs up router.conf in-place as router.conf.bak inside
# conf-dir; Debian's default conf-dir exclusion suffixes (.dpkg-dist,
# .dpkg-old, .dpkg-new) don't cover .bak, so dnsmasq loads the stale backup
# too and fails with "illegal repeated keyword". Append .bak to the
# exclusion list so the backup stays invisible to dnsmasq.
if [[ -f "$DNSMASQ_DEFAULT" ]]; then
    if grep -q "^CONFIG_DIR=" "$DNSMASQ_DEFAULT"; then
        if ! grep -q "^CONFIG_DIR=.*\.bak" "$DNSMASQ_DEFAULT"; then
            sed -i 's|^CONFIG_DIR=.*|CONFIG_DIR=/etc/dnsmasq.d,.dpkg-dist,.dpkg-old,.dpkg-new,.bak|' "$DNSMASQ_DEFAULT"
        fi
    else
        echo "CONFIG_DIR=/etc/dnsmasq.d,.dpkg-dist,.dpkg-old,.dpkg-new,.bak" >> "$DNSMASQ_DEFAULT"
    fi
else
    echo "CONFIG_DIR=/etc/dnsmasq.d,.dpkg-dist,.dpkg-old,.dpkg-new,.bak" > "$DNSMASQ_DEFAULT"
fi
ok "dnsmasq CONFIG_DIR set to exclude .bak backups (pre-install)"

if [[ -z "$OFFLINE_DIR" ]]; then
    # Online: fetch from apt repositories.
    apt-get update -qq

    RUNTIME_PKGS=(
        iproute2 udev dnsmasq smartdns wpasupplicant iw nftables caddy
        systemd-timesyncd ppp
    )
    BASE_PKGS=(
        curl tar wget unzip neovim
    )
    DIAG_PKGS=(
        mtr tcpdump nmap iperf3 ethtool httping inetutils-traceroute telnet
        bind9-dnsutils conntrack socat netcat-openbsd net-tools ebtables ipset
        pppoe
    )
    MAINT_PKGS=(
        htop iotop tmux rsync jq lsof psmisc tree git
        openssl bash-completion wireguard
        libarchive-tools xz-utils zip axel
    )

    apt-get install -y --no-install-recommends \
        "${RUNTIME_PKGS[@]}" \
        "${BASE_PKGS[@]}" \
        "${DIAG_PKGS[@]}" \
        "${MAINT_PKGS[@]}"
else
    # Offline: install every .deb in the bundle's debs/ directory.
    # apt-get handles dpkg ordering for local .deb files when given explicit
    # paths — a single transaction so pre/post-install hooks fire in the
    # correct dependency order, unlike plain `dpkg -i` which errors on
    # unsatisfied deps if packages arrive out of order.
    #
    # Fallback note: if apt-get refuses local .deb installs (very old apt),
    # use: dpkg -i "$OFFLINE_DIR"/debs/*.deb && apt-get install -f -y
    DEB_PATHS=("$OFFLINE_DIR"/debs/*.deb)
    [[ ${#DEB_PATHS[@]} -gt 0 && -f "${DEB_PATHS[0]}" ]] \
        || die "No .deb files found in bundle: $OFFLINE_DIR/debs/"

    apt-get install -y --no-install-recommends "${DEB_PATHS[@]}"
fi

ok "All apt packages installed"

########################################################################
# STEP 3 — SmartDNS: GitHub release binary → /usr/local/bin/smartdns
#
# Online mode: fetch tarball from GitHub Releases API.
# Offline mode: use the tarball pre-downloaded into the bundle's smartdns/.
#
# In both modes, the actual binary extraction and install are identical —
# handled by install_smartdns_binary() below.
########################################################################
info "Step 3: SmartDNS binary"

# Shared function: extract the smartdns binary from a local tarball and
# install it to /usr/local/bin/smartdns.
install_smartdns_binary() {
    local tarball="$1"
    local extract_dir="$TMP_WORK/sd-extract"
    mkdir -p "$extract_dir"
    tar -xzf "$tarball" -C "$extract_dir"
    local SD_BIN
    SD_BIN="$(find "$extract_dir" -name smartdns -type f | head -1)"
    if [[ -n "$SD_BIN" ]]; then
        install -m 755 "$SD_BIN" /usr/local/bin/smartdns
        ok "SmartDNS binary installed to /usr/local/bin/smartdns"
    else
        warn "smartdns binary not found in tarball; apt-installed version will be used"
    fi
}

case "$ARCH" in
    amd64) SD_ARCH="x86_64"  ;;
    arm64) SD_ARCH="aarch64" ;;
    *)     die "Unexpected ARCH: $ARCH" ;;
esac

if [[ -z "$OFFLINE_DIR" ]]; then
    # Online: query GitHub Releases API for the latest tarball URL.
    SD_RELEASE_JSON="$(curl -fsSL "https://api.github.com/repos/${SMARTDNS_REPO}/releases/latest")"

    SD_TAG="$(printf '%s' "$SD_RELEASE_JSON" | jq -r '.tag_name')"
    [[ -n "$SD_TAG" && "$SD_TAG" != "null" ]] || die "Could not determine SmartDNS latest release tag"
    info "  SmartDNS latest tag: ${SD_TAG}"

    SD_ASSET_PATTERN="${SD_ARCH}-linux-all.tar.gz"
    SD_URL="$(printf '%s' "$SD_RELEASE_JSON" \
        | jq -r --arg pat "$SD_ASSET_PATTERN" \
            '.assets[]? | select(.name | endswith($pat)) | .browser_download_url' \
        | head -1)"
    [[ -n "$SD_URL" ]] || die "No SmartDNS release asset matching *${SD_ASSET_PATTERN} found for tag ${SD_TAG}"
    SD_PKG="$(basename "$SD_URL")"
    info "  SmartDNS asset: ${SD_PKG}"

    curl -fsSL -L -o "${TMP_WORK}/${SD_PKG}" "$SD_URL"
    install_smartdns_binary "${TMP_WORK}/${SD_PKG}"
else
    # Offline: use the pre-downloaded tarball from the bundle.
    SD_TARBALL="$(find "$OFFLINE_DIR/smartdns" -name "*${SD_ARCH}*.tar.gz" | head -1)"
    [[ -n "$SD_TARBALL" ]] \
        || die "No SmartDNS tarball matching *${SD_ARCH}*.tar.gz in bundle: $OFFLINE_DIR/smartdns/"
    info "  SmartDNS tarball (bundle): $(basename "$SD_TARBALL")"
    install_smartdns_binary "$SD_TARBALL"
fi

# Create a systemd unit only if the release tarball did not ship one.
# The apt package (if installed) provides /lib/systemd/system/smartdns.service;
# the GitHub tarball typically does not.
if [[ ! -f /lib/systemd/system/smartdns.service ]] && \
   [[ ! -f /etc/systemd/system/smartdns.service ]]; then
    cat > /etc/systemd/system/smartdns.service <<'UNIT'
[Unit]
Description=SmartDNS DNS Server
Documentation=https://github.com/pymumu/smartdns
After=network.target
Wants=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/smartdns -f -c /etc/smartdns/smartdns.conf
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
UNIT
    ok "SmartDNS systemd unit created"
fi

mkdir -p /etc/smartdns
if [[ ! -f /etc/smartdns/smartdns.conf ]]; then
    cat > /etc/smartdns/smartdns.conf <<'EOF'
# SmartDNS configuration — initial skeleton written by luban install.sh.
# This file is regenerated from /opt/router/config.json at every apply; manual
# edits are overwritten.
bind [::]:53
cache-size 4096
EOF
    ok "SmartDNS default config written"
fi

systemctl daemon-reload
systemctl enable smartdns
# apt's smartdns package (RUNTIME_PKGS) may already be running an older
# binary/config from its postinst; restart so the GitHub binary and our
# config actually take effect now instead of at next reboot.
systemctl restart smartdns
ok "SmartDNS enabled and restarted"

########################################################################
# STEP 4 — PPPoE kernel module check
########################################################################
info "Step 4: PPPoE kernel module check"

if modinfo pppoe &>/dev/null; then
    ok "pppoe kernel module present"
else
    warn "Kernel module 'pppoe' not found."
    warn "PPPoE WAN mode will NOT work on this kernel."
    warn "Remedy: install a kernel with CONFIG_PPPOE, or use DHCP/Static WAN."
fi

########################################################################
# STEP 5 — Obtain release tarball (GitHub download, --local, or --offline)
########################################################################
info "Step 5: Obtaining luban release"

TARBALL=""
RELEASE_DIR="${TMP_WORK}/release"
mkdir -p "$RELEASE_DIR"

if [[ -n "$OFFLINE_DIR" ]]; then
    # Offline: the bundle's luban/ directory IS the release directory.
    # Copy (not move — the bundle dir may be read-only) to RELEASE_DIR.
    cp -r "$OFFLINE_DIR/luban/." "$RELEASE_DIR/"
    ok "Luban payload loaded from bundle: $OFFLINE_DIR/luban/"
elif [[ -n "$LOCAL_TARBALL" ]]; then
    [[ -f "$LOCAL_TARBALL" ]] || die "Local tarball not found: $LOCAL_TARBALL"
    TARBALL="$LOCAL_TARBALL"
    ok "Using local tarball: $TARBALL"
    tar -xzf "$TARBALL" -C "$RELEASE_DIR" --strip-components=1
    ok "Release tarball extracted"
else
    LUBAN_TAG="$(curl -fsSL \
        "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" \
        | grep '"tag_name"' | head -1 \
        | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')"
    LUBAN_PKG="luban-${LUBAN_TAG}-linux-${ARCH}.tar.gz"
    LUBAN_URL="https://github.com/${GITHUB_REPO}/releases/download/${LUBAN_TAG}/${LUBAN_PKG}"
    info "  Downloading luban ${LUBAN_TAG}..."
    TARBALL="${TMP_WORK}/luban.tar.gz"
    curl -fsSL -L -o "$TARBALL" "$LUBAN_URL"
    ok "Downloaded $LUBAN_PKG"
    tar -xzf "$TARBALL" -C "$RELEASE_DIR" --strip-components=1
    ok "Release tarball extracted"
fi

########################################################################
# STEP 6 — Deploy to /opt/router/
#
# Idempotency rules:
#   luban, www/, templates/, boards/  — always updated from release
#   boards.d/                       — created once, never touched again
#   config.json                     — never overwritten (preserves user settings)
########################################################################
info "Step 6: Deploy to $INSTALL_DIR"

mkdir -p "$INSTALL_DIR"

[[ -f "${RELEASE_DIR}/luban" ]] || die "Release missing: luban"
install -m 755 "${RELEASE_DIR}/luban" "${INSTALL_DIR}/luban"
# Migration: remove stale binary from previous installs that used the old name.
rm -f "${INSTALL_DIR}/rui"
ok "luban binary deployed"

for asset in www templates boards; do
    if [[ -d "${RELEASE_DIR}/${asset}" ]]; then
        rm -rf "${INSTALL_DIR:?}/${asset}"
        cp -r "${RELEASE_DIR}/${asset}" "${INSTALL_DIR}/${asset}"
        ok "${asset}/ updated"
    fi
done

# boards.d/ — user-added board profiles; create once, never overwrite
if [[ ! -d "${INSTALL_DIR}/boards.d" ]]; then
    mkdir -p "${INSTALL_DIR}/boards.d"
    ok "Created $INSTALL_DIR/boards.d/"
else
    ok "$INSTALL_DIR/boards.d/ already exists — left untouched"
fi

# config.json — install.sh never writes this file. The luban binary seeds a
# valid default (internal/config.NewStore) on first start if it is missing.
# We only need INSTALL_DIR to exist, which the mkdir -p above already ensured.
if [[ -f "${INSTALL_DIR}/config.json" ]]; then
    ok "config.json already exists — left untouched"
else
    ok "config.json absent — luban will seed a default config on first start"
fi

########################################################################
# STEP 7 — Systemd units from tarball's systemd/ directory
#
# Enabled  : router-ui.service, router-rollback.timer
# Installed but NOT enabled: pppd.service (activated by Go backend)
########################################################################
info "Step 7: Installing systemd units"

SYSTEMD_SRC="${RELEASE_DIR}/systemd"
[[ -d "$SYSTEMD_SRC" ]] || die "Release missing systemd/ directory"

install_unit() {
    local name="$1"
    local src="${SYSTEMD_SRC}/${name}"
    [[ -f "$src" ]] || die "Unit file missing from release: systemd/${name}"
    install -m 644 "$src" "/etc/systemd/system/${name}"
    ok "Installed /etc/systemd/system/${name}"
}

install_unit "router-ui.service"
install_unit "router-rollback.service"
install_unit "router-rollback.timer"
install_unit "pppd.service"

# router-rollback.service's ExecStart points at /opt/router/systemd/router-rollback.sh
# (not /etc/systemd/system/) — deploy the script itself alongside the units.
[[ -f "${SYSTEMD_SRC}/router-rollback.sh" ]] || die "Release missing: systemd/router-rollback.sh"
mkdir -p "${INSTALL_DIR}/systemd"
install -m 755 "${SYSTEMD_SRC}/router-rollback.sh" "${INSTALL_DIR}/systemd/router-rollback.sh"
ok "Installed ${INSTALL_DIR}/systemd/router-rollback.sh"

systemctl daemon-reload
systemctl enable router-ui.service
systemctl enable router-rollback.timer
# pppd.service: installed but NOT enabled.
# The Go backend starts/stops it on demand when WAN type is PPPoE.
ok "Systemd units installed; pppd.service installed but not enabled"

########################################################################
# STEP 8 — Caddy: drop-in (User=root) + Caddyfile
#
# Debian's caddy package defaults to User=caddy; we need root so Caddy
# can serve the unix socket owned by root and read /opt/router/www.
########################################################################
info "Step 8: Caddy configuration"

mkdir -p "$CADDY_DROPIN_DIR"
if [[ ! -f "${CADDY_DROPIN_DIR}/luban-root.conf" ]]; then
    cat > "${CADDY_DROPIN_DIR}/luban-root.conf" <<'EOF'
[Service]
User=root
EOF
    ok "Caddy drop-in (User=root) installed"
else
    ok "Caddy drop-in already present — left untouched"
fi

mkdir -p /etc/caddy
# Always (re)write: the Debian caddy package's postinst drops its own stub
# Caddyfile (":80 { ... }", no TLS) at this exact path, so a "write only if
# missing" guard here silently keeps that HTTP-only stub instead of ours —
# that's what produced "listening only on the HTTP port, no automatic HTTPS"
# in the field test. This bootstrap file is disposable anyway: the apply
# pipeline overwrites it with the rendered version from
# router/templates/Caddyfile.tpl as soon as a config is applied.
cat > /etc/caddy/Caddyfile <<'EOF'
# luban — Caddyfile (auto-generated by install.sh; do not edit)
#
# Serves:
#   /        → /opt/router/www  (self-contained; zero CDN dependencies)
#   /api/*   → unix//run/router/api.sock  (Go backend)
#
# This bootstrap file is disposable — the apply pipeline overwrites it with
# the rendered version from router/templates/Caddyfile.tpl as soon as a
# config is applied. Kept in sync with that template below.

{
    local_certs
}

# Catch-all on :443 — deliberately matches ANY Host, not just the named
# sites. This is the repair tool for when the network is broken (e.g.
# reached through an SSH tunnel where Host is "localhost"), so it must be
# reachable no matter what hostname the client used to get here. See
# DECISIONS: "raw LAN IP as fallback" — this extends that same rationale
# to arbitrary hosts.
:443 {
    # on_demand: a hostless site has no static domain list to pick a cert
    # for, so arbitrary/absent SNI needs Caddy to issue one on the fly.
    tls internal {
        on_demand
    }
    # API proxy to the Go backend on its Unix socket. Kept out of the SPA
    # fallback below so a missing API route 404s instead of returning HTML.
    handle /api/* {
        reverse_proxy unix//run/router/api.sock
    }

    # try_files falls back to index.html so React Router paths like
    # /network survive a refresh or direct load instead of 404ing.
    handle {
        root * /opt/router/www
        try_files {path} /index.html
        file_server
    }
}

# Plain-HTTP requests get redirected to HTTPS. A named-host block would
# get this automatically from Caddy; the :443 catch-all above does not,
# so it's spelled out explicitly here.
:80 {
    redir https://{host}{uri} permanent
}
EOF
ok "Caddyfile written"

mkdir -p /run/router
systemctl daemon-reload
systemctl enable caddy
systemctl restart caddy
ok "Caddy enabled and restarted"

########################################################################
# STEP 9 — DNS stack
#
# a) dnsmasq: port=0 (DNS off), IGNORE_RESOLVCONF=yes
# b) systemd-resolved: STOP then MASK
#    (mask alone does not stop a running instance; both fight SmartDNS for :53)
# c) resolv.conf: remove symlink → write static nameserver 127.0.0.1, after
#    verifying smartdns actually answers there (see Step 0)
########################################################################
info "Step 9: DNS stack configuration"

# dnsmasq's bootstrap config (port=0) and /etc/default/dnsmasq
# IGNORE_RESOLVCONF were already written pre-install in Step 2, so postinst
# never raced smartdns for :53. Restart here to make sure the running
# instance reflects that config, in case dnsmasq was already
# installed/running before this script ran.
systemctl restart dnsmasq
ok "dnsmasq restarted (DNS listener off, port=0)"

# systemd-resolved: stop THEN mask (order matters — mask alone leaves it running)
if systemctl is-active --quiet systemd-resolved 2>/dev/null; then
    systemctl stop systemd-resolved
fi
systemctl mask systemd-resolved
ok "systemd-resolved stopped and masked"

# resolv.conf: remove the symlink managed-resolved created, then restore the
# static resolver. In offline mode, Step 0 was skipped, so resolv.conf may
# still be the system default — write our static entry unconditionally so
# the final state is always correct regardless of install mode.
if [[ -L /etc/resolv.conf ]]; then
    rm -f /etc/resolv.conf
fi
if [[ -z "$OFFLINE_DIR" ]]; then
    # Online mode: wait for smartdns to answer on 127.0.0.1 before switching
    # (Step 0 may have pointed resolv.conf at the gateway temporarily).
    for _ in 1 2 3 4 5; do
        dig +time=2 +tries=1 @127.0.0.1 deb.debian.org >/dev/null 2>&1 && break
        sleep 1
    done
fi
if [[ ! -f /etc/resolv.conf ]] || ! grep -q "^nameserver 127.0.0.1" /etc/resolv.conf; then
    printf 'nameserver 127.0.0.1\n' > /etc/resolv.conf
fi
ok "resolv.conf → nameserver 127.0.0.1"

########################################################################
# STEP 10 — systemd-networkd: enable
#            NetworkManager / ifupdown: remove from NIC management
########################################################################
info "Step 10: systemd-networkd + NIC manager cleanup"

systemctl enable systemd-networkd
systemctl start systemd-networkd
ok "systemd-networkd enabled and started"

# NetworkManager: disable if installed (it fights networkd for ethernet NICs)
if systemctl list-unit-files NetworkManager.service &>/dev/null; then
    if systemctl is-enabled --quiet NetworkManager.service 2>/dev/null; then
        systemctl stop NetworkManager.service 2>/dev/null || true
        systemctl disable NetworkManager.service
        ok "NetworkManager disabled"
    else
        ok "NetworkManager already disabled"
    fi
fi

# netplan: on cloud-init-provisioned images (e.g. Ubuntu cloud/VM images),
# netplan's generator writes /run/systemd/network/10-netplan-<if>.network on
# every boot. systemd-networkd applies exactly one .network file per
# interface — the alphabetically-first match across /usr/lib, /run, /etc
# combined — and "10-netplan-eth0.network" sorts BEFORE luban's own
# "10-wan.network" ('n' < 'w'). Result: luban's rendered WAN config is
# silently ignored; netplan's own static/DHCP config governs the interface
# instead. This is invisible in routed DHCP mode (netplan's own DHCP lease
# happens to bring up a working address anyway) but fatal in bridge mode:
# Bridge=br0 in luban's 10-wan.network never takes effect, the WAN port
# never joins br0, and br0 — now missing its only uplink — never gets a
# DHCPv4 lease (field-tested failure: br0 stuck with an IPv6 link-local
# address only). Disable netplan the same way NetworkManager/ifupdown are
# disabled below, so systemd-networkd + luban's own .network files are the
# sole authority over every NIC.
if [[ -d /etc/netplan ]] && compgen -G "/etc/netplan/*.yaml" > /dev/null 2>&1; then
    mkdir -p /etc/netplan/luban-disabled
    mv /etc/netplan/*.yaml /etc/netplan/luban-disabled/ 2>/dev/null || true
    ok "netplan YAML configs moved aside (backup: /etc/netplan/luban-disabled/)"
fi
if [[ -d /etc/netplan ]]; then
    # Prevent cloud-init from regenerating netplan config on next boot.
    mkdir -p /etc/cloud/cloud.cfg.d
    cat > /etc/cloud/cloud.cfg.d/99-luban-disable-network-config.cfg <<'EOF'
# Installed by luban install.sh — cloud-init must not manage networking;
# systemd-networkd (driven by luban's own rendered .network files) owns it.
network: {config: disabled}
EOF
    ok "cloud-init network config generation disabled"
fi
# Remove any netplan output already generated for the current boot so NICs
# (in particular the WAN interface) are immediately released back to
# luban's own .network files without requiring a reboot.
rm -f /run/systemd/network/10-netplan-*.network /run/systemd/network/10-netplan-*.link
ok "netplan disabled (stale generated .network/.link files removed)"

# ifupdown: strip ethernet stanzas from /etc/network/interfaces so ifupdown
# does not race networkd for the same NICs.  Loopback stanza is kept.
INTERFACES_FILE="/etc/network/interfaces"
if [[ -f "$INTERFACES_FILE" ]]; then
    if grep -qE "^(auto|iface|allow-hotplug).+(eth|en[opsx]|eno|enx)" \
            "$INTERFACES_FILE" 2>/dev/null; then
        cp "$INTERFACES_FILE" "${INTERFACES_FILE}.luban-bak"
        cat > "$INTERFACES_FILE" <<'EOF'
# Managed by luban install.sh — ethernet stanzas removed.
# All interface configuration is handled by systemd-networkd (.network files).
source /etc/network/interfaces.d/*

auto lo
iface lo inet loopback
EOF
        ok "ifupdown interfaces: ethernet stanzas removed (backup: ${INTERFACES_FILE}.luban-bak)"
    else
        ok "ifupdown interfaces already clean of ethernet stanzas"
    fi
fi

########################################################################
# STEP 11 — Start services
########################################################################
info "Step 11: Starting luban services"

systemctl start router-ui.service
systemctl start router-rollback.timer
ok "router-ui.service and router-rollback.timer started"

########################################################################
# STATUS SUMMARY
########################################################################
_svc() { systemctl is-active "$1" 2>/dev/null || echo "inactive"; }

printf '\n'
printf '══════════════════════════════════════════════════════\n'
printf '  luban Router — Installation Complete\n'
if [[ -n "$OFFLINE_DIR" ]]; then
printf '  (offline mode)\n'
fi
printf '══════════════════════════════════════════════════════\n'
printf '\n'
printf '  First-login URL : http://192.168.20.1\n'
printf '  Default login   : admin / password\n'
printf '  NOTE: You will be forced to change the password\n'
printf '        on first login.\n'
printf '\n'
printf '  %-34s %s\n' "Service" "Status"
printf '  %-34s %s\n' "-------" "------"
printf '  %-34s %s\n' "router-ui.service"        "$(_svc router-ui.service)"
printf '  %-34s %s\n' "caddy.service"             "$(_svc caddy.service)"
printf '  %-34s %s\n' "smartdns.service"          "$(_svc smartdns.service)"
printf '  %-34s %s\n' "dnsmasq.service"           "$(_svc dnsmasq.service)"
printf '  %-34s %s\n' "systemd-networkd.service"  "$(_svc systemd-networkd.service)"
printf '  %-34s %s\n' "router-rollback.timer"     "$(_svc router-rollback.timer)"
printf '\n'
printf '  Install path : %s\n' "$INSTALL_DIR"
printf '  Config file  : %s/config.json\n' "$INSTALL_DIR"
printf '\n'
printf '══════════════════════════════════════════════════════\n'
