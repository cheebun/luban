#!/usr/bin/env bash
# make-offline-bundle.sh — Build a luban offline installation bundle.
#
# Run on a Debian system that MATCHES the target router's OS release and CPU
# architecture. For amd64 routers the Minisforum MS-A2 test VM (Debian 13,
# amd64) is a suitable builder. Arm64 routers require an arm64 Debian builder.
#
# Builder-must-match-target caveat
# ---------------------------------
# apt's dependency solver uses the BUILDER's installed package database to
# determine which transitive dependencies are already satisfied and therefore
# don't need to be downloaded. On a builder where many packages are already
# installed, apt may skip downloading a dependency that satisfies the
# constraint locally, leaving that package absent from the bundle — and the
# offline target would then fail to install.
#
# Mitigation applied: --reinstall forces apt to download every explicitly
# requested package (and re-evaluate their transitive deps) even when already
# installed on the builder. Run this script on a MINIMAL Debian install
# (the PVE VM works; avoid a full desktop system) to maximise the overlap
# between builder and target installed-package sets.
#
# Dependency-closure approach: apt-get install --download-only --reinstall
# -------------------------------------------------------------------------
# Two approaches were considered:
#
#   A) apt-get install --download-only --reinstall -y \
#          -o Dir::Cache::archives=<dir> <pkgs>
#
#   B) apt-get download $(apt-rdepends <pkgs> | grep -v '^  ' | ...)
#
# Option A is used here. Reasons:
#   - Invokes apt's full dep solver, which handles Conflicts, Replaces,
#     virtual packages, and package alternatives correctly. apt-rdepends
#     (option B) is a simple recursive graph traversal that cannot resolve
#     virtual packages or choose among alternatives.
#   - apt-rdepends must itself be installed on the builder (it is not in
#     the Debian base system), adding an extra prerequisite.
#   - Option B requires non-trivial shell parsing (deduplication, filtering
#     "PreDepends" indented lines, excluding unavailable packages) and still
#     cannot handle virtual packages.
#   - --reinstall ensures all selected packages are downloaded even if the
#     builder already has them installed; option B has no equivalent flag.
#
# Usage:
#   sudo ./make-offline-bundle.sh [OPTIONS]
#
# Options:
#   -v, --version <ver>    Bundle version string (default: git describe or timestamp)
#   --release <tarball>    Use a pre-built luban release tarball as the luban payload
#   --from-repo            Build luban payload from the current repository tree
#                          (requires Go and pnpm on the builder)
#   -o, --output <dir>     Write the final bundle tarball here (default: .)
#   -h, --help             Show this help and exit
#
# Exactly one of --release or --from-repo must be given.
#
set -euo pipefail

##############################################################################
GITHUB_REPO="cheebun/luban"
SMARTDNS_REPO="pymumu/smartdns"
##############################################################################

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

info() { printf '\n[bundle] %s\n' "$*"; }
ok()   { printf '[bundle]   ok  %s\n' "$*"; }
warn() { printf '[bundle] WARN  %s\n' "$*" >&2; }
die()  { printf '[bundle] ERROR %s\n' "$*" >&2; exit 1; }

# ── argument parsing ──────────────────────────────────────────────────────────
usage() {
    grep '^#' "$0" | sed 's/^# \{0,1\}//' | grep -A999 'make-offline-bundle'
    exit 1
}

BUNDLE_VER=""
RELEASE_TARBALL=""
FROM_REPO=0
OUTPUT_DIR="."

while [[ $# -gt 0 ]]; do
    case "$1" in
        -v|--version)
            [[ -z "${2:-}" ]] && die "--version requires a value"
            BUNDLE_VER="$2"; shift 2 ;;
        --release)
            [[ -z "${2:-}" ]] && die "--release requires a path"
            RELEASE_TARBALL="$2"; shift 2 ;;
        --from-repo)
            FROM_REPO=1; shift ;;
        -o|--output)
            [[ -z "${2:-}" ]] && die "--output requires a directory"
            OUTPUT_DIR="$2"; shift 2 ;;
        -h|--help) usage ;;
        *) die "Unknown argument: $1" ;;
    esac
done

# Exactly one luban payload source must be specified.
if [[ -n "$RELEASE_TARBALL" && "$FROM_REPO" -eq 1 ]]; then
    die "Specify exactly one of --release or --from-repo, not both."
fi
if [[ -z "$RELEASE_TARBALL" && "$FROM_REPO" -eq 0 ]]; then
    die "Specify one of --release <tarball> or --from-repo."
fi

[[ $EUID -eq 0 ]] || die "This script must be run as root (apt operations require root)."

# ── arch detection ────────────────────────────────────────────────────────────
ARCH_RAW="$(uname -m)"
case "$ARCH_RAW" in
    x86_64)  ARCH="amd64"; SD_ARCH="x86_64"  ;;
    aarch64) ARCH="arm64"; SD_ARCH="aarch64" ;;
    *) die "Unsupported architecture: $ARCH_RAW" ;;
esac
info "Architecture: $ARCH ($ARCH_RAW)"

# ── version string ────────────────────────────────────────────────────────────
if [[ -z "$BUNDLE_VER" ]]; then
    if git -C "$REPO_ROOT" describe --tags --exact-match 2>/dev/null; then
        BUNDLE_VER="$(git -C "$REPO_ROOT" describe --tags --exact-match 2>/dev/null)"
    elif git -C "$REPO_ROOT" rev-parse --short HEAD 2>/dev/null; then
        BUNDLE_VER="git-$(git -C "$REPO_ROOT" rev-parse --short HEAD 2>/dev/null)"
    else
        BUNDLE_VER="$(date +%Y%m%d%H%M%S)"
    fi
fi
info "Bundle version: $BUNDLE_VER"

# ── working directory (cleaned up on exit) ────────────────────────────────────
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

mkdir -p "$WORK/debs" "$WORK/smartdns" "$WORK/luban"

########################################################################
# STEP 1 — Caddy apt repo (required so apt can download caddy.deb)
########################################################################
info "Step 1: Ensuring Caddy apt repo is configured"

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

apt-get update -qq
ok "apt index updated"

########################################################################
# STEP 2 — Download all apt packages with full dependency closure
#
# The four package lists mirror install.sh exactly. If install.sh adds or
# removes packages, this list must be updated in lockstep.
########################################################################
info "Step 2: Downloading apt packages with full dependency closure"

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

ALL_PKGS=(
    "${RUNTIME_PKGS[@]}"
    "${BASE_PKGS[@]}"
    "${DIAG_PKGS[@]}"
    "${MAINT_PKGS[@]}"
)

# Download to WORK/debs/ using apt's own dep solver (see header for approach
# justification). The custom Dir::Cache::archives directs downloaded .deb
# files to our work dir; partial/ subdir is created automatically by apt.
# --reinstall: download every listed package even if already present on the
# builder; apt would otherwise skip already-satisfied packages.
apt-get install -y --download-only --reinstall --no-install-recommends \
    -o Dir::Cache::archives="$WORK/debs/" \
    "${ALL_PKGS[@]}"

# Remove the partial/ subdir apt creates inside the cache dir; it holds
# incomplete downloads and is not needed in the bundle.
rm -rf "$WORK/debs/partial"

DEB_COUNT="$(find "$WORK/debs" -name '*.deb' | wc -l)"
ok "Downloaded $DEB_COUNT .deb files (including full dependency closure)"

########################################################################
# STEP 3 — SmartDNS GitHub release tarball
########################################################################
info "Step 3: SmartDNS GitHub release tarball"

SD_RELEASE_JSON="$(curl -fsSL "https://api.github.com/repos/${SMARTDNS_REPO}/releases/latest")"
SD_TAG="$(printf '%s' "$SD_RELEASE_JSON" | jq -r '.tag_name')"
[[ -n "$SD_TAG" && "$SD_TAG" != "null" ]] || die "Could not determine SmartDNS latest release tag"
info "  SmartDNS latest tag: ${SD_TAG}"

SD_ASSET_PATTERN="${SD_ARCH}-linux-all.tar.gz"
SD_URL="$(printf '%s' "$SD_RELEASE_JSON" \
    | jq -r --arg pat "$SD_ASSET_PATTERN" \
        '.assets[]? | select(.name | endswith($pat)) | .browser_download_url' \
    | head -1)"
[[ -n "$SD_URL" ]] || die "No SmartDNS asset matching *${SD_ASSET_PATTERN} for tag ${SD_TAG}"

SD_PKG="$(basename "$SD_URL")"
info "  SmartDNS asset: ${SD_PKG}"
curl -fsSL -L -o "$WORK/smartdns/${SD_PKG}" "$SD_URL"
ok "SmartDNS tarball downloaded: smartdns/${SD_PKG}"

########################################################################
# STEP 4 — Luban payload: --release tarball or --from-repo build
########################################################################
info "Step 4: Luban payload"

if [[ -n "$RELEASE_TARBALL" ]]; then
    [[ -f "$RELEASE_TARBALL" ]] || die "Release tarball not found: $RELEASE_TARBALL"
    info "  Extracting release tarball: $RELEASE_TARBALL"
    tar -xzf "$RELEASE_TARBALL" -C "$WORK/luban" --strip-components=1
    ok "Release tarball extracted into luban/"
else
    # --from-repo: build from the current repo tree.
    info "  Building from repository: $REPO_ROOT"

    command -v go   >/dev/null 2>&1 || die "Go toolchain not found on builder (required for --from-repo)"
    command -v pnpm >/dev/null 2>&1 || die "pnpm not found on builder (required for --from-repo)"

    # Build Go binary (luban).
    # go build ./... in router/api produces all binaries; we capture 'luban'
    # by building the main package directly.
    info "  Building Go binary (luban)"
    (cd "$REPO_ROOT/router/api" && go build -o "$WORK/luban/luban" .)
    [[ -f "$WORK/luban/luban" ]] || die "go build did not produce luban binary"
    chmod 755 "$WORK/luban/luban"
    ok "luban binary built"

    # Build web frontend.
    info "  Building web frontend"
    (cd "$REPO_ROOT" && pnpm install --frozen-lockfile)
    (cd "$REPO_ROOT" && pnpm --filter @router/web build)
    [[ -d "$REPO_ROOT/router/web/dist" ]] || die "vite build did not produce router/web/dist/"
    cp -r "$REPO_ROOT/router/web/dist/." "$WORK/luban/www/"
    ok "Web frontend built and copied to luban/www/"

    # Copy runtime assets from the repo tree.
    for asset in templates boards; do
        if [[ -d "$REPO_ROOT/router/$asset" ]]; then
            cp -r "$REPO_ROOT/router/$asset" "$WORK/luban/$asset"
            ok "Copied router/$asset/"
        fi
    done

    # Copy systemd units and the rollback script.
    if [[ -d "$REPO_ROOT/router/systemd" ]]; then
        cp -r "$REPO_ROOT/router/systemd" "$WORK/luban/systemd"
        ok "Copied router/systemd/"
    fi
fi

########################################################################
# STEP 5 — manifest.txt: versions + sha256sums of key files
########################################################################
info "Step 5: Generating manifest.txt"

LUBAN_BIN="$WORK/luban/luban"
LUBAN_SHA=""
[[ -f "$LUBAN_BIN" ]] && LUBAN_SHA="$(sha256sum "$LUBAN_BIN" | awk '{print $1}')"

SD_TARBALL_PATH="$(find "$WORK/smartdns" -name '*.tar.gz' | head -1)"
SD_SHA=""
[[ -n "$SD_TARBALL_PATH" ]] && SD_SHA="$(sha256sum "$SD_TARBALL_PATH" | awk '{print $1}')"

cat > "$WORK/manifest.txt" <<EOF
# luban offline bundle manifest
# Generated: $(date -u +"%Y-%m-%dT%H:%M:%SZ")
bundle_version=${BUNDLE_VER}
arch=${ARCH}
smartdns_tag=${SD_TAG}
smartdns_asset=${SD_PKG}

# sha256sums
$(sha256sum "$WORK/luban/luban" 2>/dev/null || echo "luban/luban: not present")
$(sha256sum "$WORK/smartdns/"*.tar.gz 2>/dev/null | sed 's|.*/smartdns/|smartdns/|')

# apt package count
apt_debs=${DEB_COUNT}
EOF
ok "manifest.txt written"

# Brief human-readable note inside the bundle.
cat > "$WORK/INSTALL-NOTE.txt" <<'EOF'
luban offline installation bundle
==================================

To install on the target router:

  1. Transfer this archive to the router (e.g. via tar+ssh or scp).
  2. As root on the router, run:

       tar xzf luban-offline-<version>-<arch>.tar.gz -C /tmp/
       cd /tmp/luban-offline-<version>-<arch>
       bash install.sh --offline .

  See docs/offline-install.md in the luban source repository for full details.
EOF

########################################################################
# STEP 6 — Assemble and compress the final bundle tarball
########################################################################
info "Step 6: Creating bundle tarball"

BUNDLE_NAME="luban-offline-${BUNDLE_VER}-${ARCH}"
BUNDLE_DIR="$WORK/$BUNDLE_NAME"
mkdir -p "$BUNDLE_DIR"

cp -r "$WORK/debs"        "$BUNDLE_DIR/debs"
cp -r "$WORK/smartdns"    "$BUNDLE_DIR/smartdns"
cp -r "$WORK/luban"       "$BUNDLE_DIR/luban"
cp    "$WORK/manifest.txt" "$BUNDLE_DIR/manifest.txt"
cp    "$WORK/INSTALL-NOTE.txt" "$BUNDLE_DIR/INSTALL-NOTE.txt"

# Copy install.sh into the bundle for convenience — the operator can run it
# directly from the extracted directory with --offline <bundle-path>.
cp "$SCRIPT_DIR/install.sh" "$BUNDLE_DIR/install.sh"

BUNDLE_TARBALL="${OUTPUT_DIR}/${BUNDLE_NAME}.tar.gz"
mkdir -p "$OUTPUT_DIR"
tar -czf "$BUNDLE_TARBALL" -C "$WORK" "$BUNDLE_NAME"

BUNDLE_SIZE="$(du -sh "$BUNDLE_TARBALL" | awk '{print $1}')"
BUNDLE_SHA="$(sha256sum "$BUNDLE_TARBALL" | awk '{print $1}')"

printf '\n'
printf '══════════════════════════════════════════════════════\n'
printf '  luban offline bundle created\n'
printf '══════════════════════════════════════════════════════\n'
printf '\n'
printf '  Bundle   : %s\n' "$BUNDLE_TARBALL"
printf '  Size     : %s\n' "$BUNDLE_SIZE"
printf '  SHA-256  : %s\n' "$BUNDLE_SHA"
printf '  Version  : %s (%s)\n' "$BUNDLE_VER" "$ARCH"
printf '  SmartDNS : %s\n' "$SD_TAG"
printf '  apt debs : %s\n' "$DEB_COUNT"
printf '\n'
printf '  Transfer and install:\n'
printf '    scp %s root@<router>:/tmp/\n' "$(basename "$BUNDLE_TARBALL")"
printf '    ssh root@<router> "tar xzf /tmp/%s -C /tmp && bash /tmp/%s/install.sh --offline /tmp/%s"\n' \
    "$(basename "$BUNDLE_TARBALL")" "$BUNDLE_NAME" "$BUNDLE_NAME"
printf '\n'
printf '══════════════════════════════════════════════════════\n'
