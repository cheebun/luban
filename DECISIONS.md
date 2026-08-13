# Architecture Decisions

## Platform

Debian / Armbian, stock apt packages only.
OpenWrt-inspired: single config source, generated service files, apply/confirm/rollback.

## Web Stack

- **Caddy** — static files (`www/`) + reverse proxy `/api/*` → `unix:/run/router/api.sock`, `tls internal`.
  Field-tested revision: a single catch-all `:443` site (not per-hostname named
  sites), with `tls internal { on_demand }` for hostless cert issuance and a
  SPA fallback (`try_files {path} /index.html`) on every non-`/api` path.
  Named sites broke the repair-tool use case — the admin UI must work when
  reached via any Host header, raw IP, or an SSH tunnel with no matching
  hostname, and `on_demand` TLS is required because the cert can't be
  pre-provisioned for an arbitrary Host. Without the SPA fallback, React
  Router paths (e.g. `/network`, `/dns`) 404 on a hard refresh because Caddy
  looks for a matching static file instead of serving `index.html`.
- **Go** — single binary, root, listens only on unix socket (no TCP)
- **React** — via Vite + TypeScript
- **Styling** — Tailwind CSS + tailwind-variants (`tv`) + react-twc (`twc`), mandatory.
  This is a design system rule, not just a library choice:
  - `tv()` defines every component's variants (size, color, state) in one place
  - `twc` wraps them into typed React components
  - **No bare Tailwind classes in JSX** — all styling goes through tv/twc components.
    Pages compose components; they never assemble raw utility classes inline.
- **Build** — Vite + TypeScript on the dev machine; `vite build` output goes to `www/`.
  Device only receives static files.
- **Lint/format** — oxlint + oxfmt

Both Caddy and the Go backend run as root (OpenWrt precedent: uhttpd/rpcd/LuCI
all run as root). This trades away process-level privilege separation for a
simpler deployment model: no socket permission management between a `caddy`
user and the root-owned `/run/router/api.sock`, no capability tuning. The unix
socket still keeps the Go backend off TCP entirely — its only client is Caddy.

Note: the Debian caddy package defaults to `User=caddy`; `install.sh` installs
a systemd drop-in (`User=root`) to override.

### Web Stack addendum (2026-08-12)

Adopted for the frontend data layer: `@tanstack/react-query` 5.101.4 (server
state — optimistic cache updates for `saveConfig` per the official guide, but
apply/confirm/rollback stay deliberately non-optimistic since they mutate
live network state the UI can't safely predict), `@tanstack/react-form`
1.33.5 with zod validators, `zod` 4.4.3 (runtime schema for every API
payload, with types derived via `z.infer` instead of hand-written), and
`zustand` 5.0.14 (client-only state: auth, the ApplyDialog state machine).
A 300-LOC/file lint cap (`.oxlintrc.json` `max-lines`) and a
one-component-per-file rule keep the query/form/schema/store split
enforceable rather than aspirational.

Go was initially rejected as "hard to modify", but accepted once templates and board
profiles were externalized to plain files on disk (NOT go:embed).
Frontend changes require `vite build` + deploy, but no Go recompilation.

Frontend build output must be fully self-contained — admin UI on a broken WAN is the
primary repair tool. All JS/CSS bundled, zero runtime CDN.

## Dependencies

Runtime (Go backend calls these):

```
iproute2  udev  dnsmasq  smartdns  wpa_supplicant  iw  nftables  caddy
systemd-timesyncd  ppp
```

`ppp` provides pppd + the kernel-mode PPPoE plugin (`rp-pppoe.so`) for PPPoE WAN.
Kernel modules (`ppp_generic`, `pppox`, `pppoe`) ship with stock Debian/Armbian
kernels; `install.sh` verifies with `modinfo pppoe` and warns if missing.

Address management (WAN DHCP client, static IP, IPv6 RA/DHCPv6-PD, LAN RA) is
**systemd-networkd** — part of systemd, zero extra packages. Declarative
`.network` files fit the template-generation model. `isc-dhcp-client` is
rejected (upstream EOL).

Base tooling (SSH maintenance, debugging, deployment):

```
curl  tar  wget  unzip  neovim
```

Network diagnostics:

```
mtr  tcpdump  nmap  iperf3  ethtool  httping  inetutils-traceroute  telnet
bind9-dnsutils  conntrack  socat  netcat-openbsd  net-tools  ebtables  ipset
pppoe
```

`pppoe` (Roaring Penguin) is diagnostics-only: `pppoe-discovery` probes for a
PPPoE access concentrator on the wire. The user-mode client is never used for
the data path — dialing always goes through pppd + kernel-mode `rp-pppoe.so`.

System maintenance:

```
htop  iotop  tmux  rsync  jq  lsof  psmisc  tree  git
openssl  bash-completion  wireguard
libarchive-tools  xz-utils  zip  axel
```

All from `apt install`, no third-party repos except SmartDNS and Caddy.

## Services

| Concern | Component | Notes |
|---|---|---|
| DHCP | dnsmasq | `port=0`, DNS disabled |
| DNS | SmartDNS | upstream servers, routing rules, cache |
| Wi-Fi | wpa_supplicant + iw | STA (WAN uplink); hostapd if AP needed later |
| Firewall/NAT | nftables | dry-run `nft -c -f` before apply |
| NTP | systemd-timesyncd | ARM boards often have no RTC battery; required for scheduled tasks |
| Addressing | systemd-networkd | WAN DHCP client / static IP, DHCPv6-PD, LAN RA; templates generate `.network` files |

dnsmasq only does DHCP; DNS is entirely SmartDNS's job.

Considered and rejected: dnsmasq on :53 forwarding to SmartDNS on :5335
(the previous Debian 13 router ran that after a deliberate switch — but the
motivation was proxy/tproxy traffic splitting, which this project doesn't do).
Known trade-off of SmartDNS-front: DHCP client hostnames don't auto-resolve
as `<hostname>.lan`; if wanted later, the Go backend can generate SmartDNS
address records from dnsmasq leases.

dnsmasq DHCP runs with `dhcp-authoritative` (sole DHCP server on the segment).

`/etc/default/dnsmasq`'s `CONFIG_DIR` must exclude the apply pipeline's
`.bak` backups (`CONFIG_DIR=/etc/dnsmasq.d,.dpkg-dist,.dpkg-old,.dpkg-new,.bak` —
the `.bak` suffix appended to the shipped exclude list). Debian's default
`CONFIG_DIR` line only excludes `.dpkg-*` variants; without adding `.bak`,
dnsmasq loads the pre-apply backup config alongside the live one, since
`installRendered()` (see below) writes `router.conf.bak` in the same
directory. This means the shipped Debian line must be **replaced**, not
appended to — a second `CONFIG_DIR=` line doesn't merge, it overrides.

`install.sh` writes a bootstrap `/etc/dnsmasq.d/router.conf` (`port=0` only,
so dnsmasq starts safely with DHCP disabled) at the exact path the apply
pipeline later renders to — never a second, differently-named file. The
first real `apply` overwrites this bootstrap file in place, so there is only
ever one dnsmasq config the Go pipeline needs to track for backup/rollback.

dnsmasq and SmartDNS bind `br0`'s address. To avoid a boot race where they start before
`br0` has an address, prefer dnsmasq `bind-dynamic` + `After=network.target` — this is
interface-granular and doesn't block on PPPoE/WAN. The alternative is
`After=systemd-networkd-wait-online@br0.service` (waits only for br0, not all interfaces);
avoid the bare `systemd-networkd-wait-online.service` — it waits for ALL managed
interfaces, so a slow PPPoE dial or WAN DHCP timeout delays or blocks the whole LAN.

SmartDNS handles local zones; dnsmasq DHCP pushes the router as DNS server
to all clients. Built-in static DNS records:

```
router.lan   → LAN IP        # admin UI entry point
router       → LAN IP        # short name (via search domain)
br0.lan      → LAN IP        # bridge interface name
modem.lan    → modem IP      # optical modem access
```

dnsmasq DHCP pushes `search lan` so bare `router` and `br0` resolve.
Caddy serves all of the above as server names (+ raw LAN IP as fallback).

`.local` is deliberately NOT used: RFC 6762 reserves it for mDNS (multicast).
Answering `.local` names over unicast DNS conflicts with Apple devices and
Avahi — clients resolve those names via multicast and bypass SmartDNS entirely,
giving inconsistent results. Real mDNS support (avahi-daemon advertising
`router.local`) is a future feature, not a unicast DNS record.
Custom static DNS entries are a future feature (see below).

LAN clients must be able to reach the modem (e.g. 192.168.1.1) behind the WAN port.
The router adds a SNAT/masquerade rule so LAN→modem traffic exits the WAN interface
with the router's WAN IP as source; the modem sees a local request and replies normally.
Optionally `modem.lan` resolves to the modem's IP for convenience.
Modem IP defaults to the WAN DHCP gateway (auto-detected); overridable in Web UI.

## WAN Types

Three WAN connection modes, selectable in Web UI:

- **DHCP** — default. Router is a DHCP client on WAN port (modem handles PPPoE)
- **Static IP** — manual address/gateway/DNS
- **PPPoE** — router dials directly; requires username/password. MTU auto-set to 1492

PPPoE implementation: the WAN bearer `.network` sets
`LinkLocalAddressing=no` (bare L2 carrier, no IP — the public IP lives on
`ppp0`); templates generate `/etc/ppp/peers/wan` (kernel-mode
`plugin rp-pppoe.so`, `mtu 1492`, `persist`, `maxfail 0`) and
`/etc/ppp/chap-secrets` (mode 600 — plaintext credentials live here; exclude or
warn on config export). pppd runs under its own systemd unit with `nodetach`
and `Restart=`, not via `pon`/`poff`.
When WAN is PPPoE, LAN clients are told MTU 1492 via the RA MTU option and DHCPv4 option 26.
MSS clamp only rescues TCP; without the LAN MTU sync, large UDP (QUIC, WireGuard) is silently
dropped.
The pppd unit sets `After=systemd-networkd.service` so networkd has its config loaded before
`ppp0` appears.

## IPv6

WAN: accept RA + DHCPv6-PD from upstream (most ISPs use prefix delegation).
LAN: send RA with delegated prefix; clients use SLAAC.
nftables templates include ip6 rules mirroring the v4 policy.

The whole IPv6 chain is systemd-networkd, one component end to end:
WAN `.network` accepts RA + requests PD → `DHCPPrefixDelegation=yes` on the
br0 `.network` assigns a /64 from the delegated prefix → `IPv6SendRA=yes`
announces it to LAN with RDNSS = router (→ SmartDNS). Works on `ppp0` for
PPPoE WAN too. dnsmasq does NOT do RA or DHCPv6 — DHCPv4 only.

Field-tested ISP quirks (from the previous Debian 13 router deployment):
- Some ISPs assign the ppp0 address via RA, NOT IPv6CP — never gate the PD
  request on IPv6CP completion (pppd's `ipv6-up` hook may never fire).
  networkd's `IPv6AcceptRA=` + `DHCP=ipv6` on ppp0 handles this natively.
- The delegated prefix length varies (/56, /60, /64) — the /64 carved for
  br0 must be derived from it, never hardcoded. Web UI exposes an
  auto/manual prefix-length setting.
- The WAN `.network` sets `WithoutRA=solicit` in `[DHCPv6]` so the DHCPv6/PD
  client sends a solicit regardless of the RA M-flag. Some ISPs support PD
  without setting M-flag; the default behavior silently never sends a solicit.
  No side effect on ISPs that do set M-flag.

## Bridge Model

Multiple LAN ports (when present) are bridged into `br0` automatically.
WAN port is never bridged. config.json defines which interfaces belong to the bridge.

The br0 `.network` sets `ConfigureWithoutCarrier=yes` — an empty bridge has no
carrier at boot, and without this br0 never comes up, dnsmasq can't bind, and
the whole LAN gets no DHCP (field-tested failure mode).
Single-port boards skip bridging — the lone ethernet port is either WAN or LAN
(determined by board profile / detection / wizard).

## Firewall Default Policy

```
WAN inbound   → DROP  (except established/related)
LAN inbound   → ACCEPT
Forward        → DROP  (except LAN→WAN: ACCEPT + masquerade)
```

This is the nftables template baseline. User-added rules (port forwards, etc.)
are layered on top via config.json.

Baseline must also include (field-tested; omitting these breaks things silently):

- ICMPv6 ND set (`nd-neighbor-solicit`, `nd-neighbor-advert`,
  `nd-router-advert`, `mld-listener-query`) — SLAAC dies without it
- `icmpv6 type packet-too-big` rate-limited accept — PPPoE MTU is 1492;
  without PMTUD, large IPv6 packets stall silently
- WAN ICMP/ICMPv6 echo-request, rate-limited accept (debuggability)
- `ct state invalid drop`
- Forward chain: `ct state established,related accept` before the LAN→WAN accept rule —
  IPv4 return traffic survives via masquerade conntrack, but IPv6 has no NAT; without this
  rule IPv6 return packets are dropped in forward and IPv6 breaks entirely.
- TCP MSS clamping in the forward chain (`tcp flags syn tcp option maxseg size set rt mtu`) —
  mandatory for PPPoE (MTU 1492). PMTUD alone is unreliable: many servers/CDNs drop
  ICMP(v6) too-big, black-holing large TCP packets.
- WAN input accept for the DHCPv6 client (`udp dport 546`). Solicit goes
  to multicast `ff02::1:2` but the server replies from unicast, so conntrack never sees the
  reply as established — default WAN DROP silently kills prefix delegation. Source is NOT
  restricted to `fe80::/10`: most ISPs reply from link-local, but some reply from a GUA;
  the source filter silently kills PD in that case.
- The ICMPv6 ND set must apply to the WAN input chain too — the router itself needs upstream
  RAs for its own IPv6 addressing.

## Kernel (sysctl)

Templates generate `/etc/sysctl.d/99-router.conf`:

```
net.ipv4.ip_forward=1                    # without this it's not a router
net.ipv6.conf.all.forwarding=1
net.ipv4.conf.all.rp_filter=1            # drop spoofed sources
net.ipv6.conf.<wan>.accept_ra=2          # forwarding=1 defaults accept_ra to 0; explicit, not implied
net.ipv4.conf.<wan>.accept_redirects=0
net.ipv4.conf.<wan>.send_redirects=0
net.core.default_qdisc=fq
net.ipv4.tcp_congestion_control=bbr
```

`forwarding=1` makes the kernel default every interface's `accept_ra` to 0. networkd's
`IPv6AcceptRA=yes` writes `accept_ra=2` itself, but the dependency is declared explicitly
so manual interface work doesn't silently lose IPv6.
For PPPoE WAN, `ppp0` gets `rp_filter=2` (loose) — strict mode can drop legitimate replies
under asymmetric ISP routing. Strict stays on for DHCP/static WAN.

## DHCP Defaults

- LAN subnet: `192.168.20.1/24`
- DHCP pool: `192.168.20.100` – `192.168.20.254`
- Lease time: 12h
- Gateway: `192.168.20.1` (self)
- DNS: `192.168.20.1` (self → SmartDNS)

## Authentication

- Password stored as bcrypt hash in config.json
- Login returns a session token via `Set-Cookie` (HttpOnly, Secure, SameSite=Strict)
- Session timeout: 30 minutes of inactivity, true sliding window — the
  cookie is re-issued with a fresh `Max-Age` on every authenticated request,
  not just refreshed server-side. A server-side-only refresh leaves the
  browser holding the original (shorter) cookie lifetime, so the browser
  drops the cookie and logs the user out mid-session even though the server
  still considers it valid.
- Default credentials on first boot: admin / password, forced password change on first login
- `/api/login`, `/api/logout`, `/api/password` (change password)

## Logging

All logs go to journald (no separate log files — avoids SD card wear).
Logged events: config changes, auth failures, service restarts, apply/rollback actions.
Log retention managed by journald's built-in size cap (default 50MB).

## Config Model

`/opt/router/config.json` → `templates/*.tpl` generators → per-service configs.
Apply: backup → validate → write + reload → 90s confirm timer → auto-rollback if unconfirmed.

The web UI only reads/writes `config.json`; generated service files are marked
"auto-generated, do not edit". Dry-run validation (`nft -c -f`, `dnsmasq --test`)
runs before any service reload — bad configs are rejected, not applied.

The 90s confirm timer prevents lockout: if you break the network config and can't
reach the UI to confirm, the old config is automatically restored.
(OpenWrt precedent: LuCI "Save & Apply with rollback" via rpcd, default 30s.
90s is deliberately longer — PPPoE redial / DHCP renew can exceed 30s and
cause false rollbacks.)

Confirmation, LuCI-style with a manual fallback:
- After apply, the frontend polls the router; as soon as it is reachable again
  it calls `/api/confirm` automatically — no user action in the common case.
- A manual Confirm button remains as fallback (works even if polling breaks).
- An "apply unchecked" option skips the rollback window for changes known to
  interrupt connectivity.

The rollback watchdog is a **systemd timer, independent of the Go process** —
never an in-process Go timer, and never a transient `systemd-run` timer; use a
persistent `.timer` unit (`router-rollback.timer`). A started-but-not-enabled
timer still vanishes on reboot, so the power-loss case needs a flag file:
apply writes an unconfirmed-apply flag and starts the timer; `/api/confirm`
removes the flag and stops it; a boot-time check (the same unit, also enabled
with `OnBootSec=`) rolls back if the flag is present. On expiry the timer unit
restores `config.json.bak`, regenerates service configs, and restarts services
without involving the (possibly dead) backend.

After regenerating `.network` files, apply must run `networkctl reload` before (re)starting
pppd or other services — networkd does not re-match interfaces that already exist (notably
`ppp0`); skipping the reload leaves ppp0 unconfigured and IPv6 PD never triggers.

The rollback path never re-renders templates (a broken template may be the very thing being
rolled back) — it restores backed-up copies of the generated config files directly, and
restores `.network` files before restarting networkd (a bad `.network` can take networkd
down with it).

## WAN/LAN Detection (first boot only)

1. `boards/*.json` profile lookup (by device-tree model / DMI, pinned by udev device path)
2. DHCP offer probe (an offer = upstream router on that wire → WAN candidate)
3. Setup wizard fallback (user picks; always available as manual override)

Board profiles use udev device path (bus position), not interface names or MACs —
the path is identical across all units of the same board model.

Profile format (`boards/*.json`):

```json
{
  "id": "machenike-mini-2",
  "name": "human-readable model name",
  "match": {
    "dmi": { "sys_vendor": "...", "product_name": "...", "product_family": "..." }
  },
  "interfaces": [
    { "role": "wan|lan|wifi", "path": "pci-0000:2c:00.0",
      "model": "...", "label": "physical port location", "notes": "..." }
  ]
}
```

x86 boards match on DMI fields (`/sys/class/dmi/id/*`); ARM boards match on
device-tree model (`match.dt_model`, `/proc/device-tree/model`). `path` is
udev `ID_PATH`. Multiple `lan` entries → bridged into br0.

Shipped profiles (data collected from live hardware, not datasheets):

| Profile | WAN | LAN | WiFi |
|---|---|---|---|
| `machenike-mini-2` | I225-V | I225-V ×1 | Intel AX211 (AP: 2.4GHz only stable) |
| `minisforum-ms-a2` | RTL8125 | I226-V + X710 SFP+ ×2 → br0 | MT7922 |

Machenike's DMI strings are generic ("Machenike DT Computer") and may collide
with other Machenike models; MS-A2 pins the precise `board_name` (F1WSA).
WAN role on both was inferred from which NIC is vfio-passthrough'd to the
current router VM — the wizard's manual override covers any mismatch.

Unconfigured: all ports on 192.168.20.1/24 + DHCP so wizard is reachable from any port.
Once the DHCP-offer probe identifies a WAN candidate, the DHCP server is disabled on that
port (avoids leaking authoritative DHCP offers into the upstream LAN); the wizard remains
reachable from all other ports.

## API

`/api/login` `/api/logout` `/api/password` `/api/status` `/api/config` `/api/apply` `/api/confirm` `/api/rollback` `/api/log` `/api/reboot`

Status queries use `ip -j`, `nft -j` (native JSON); `iw` and dnsmasq leases
need small parsers. All subprocess calls use argument lists with timeouts.

## Installation (`install.sh`)

One-shot script that turns a clean Debian/Armbian into a working router:

1. Detect arch (`uname -m` → amd64 / arm64)
2. Add third-party repos (Caddy official, SmartDNS GitHub release)
3. `apt install` all dependencies (runtime + tooling + diagnostics + maintenance)
4. Download latest release tarball from GitHub Releases (Go binary + www/ + templates/ + boards/)
5. Deploy to `/opt/router/`, create systemd unit `router-ui.service`
6. Write Caddyfile, install systemd drop-in (`User=root`), enable caddy service
7. Configure dnsmasq (`port=0`, `IGNORE_RESOLVCONF=yes`); **stop then mask**
   systemd-resolved (`systemctl stop systemd-resolved` before mask — mask alone
   does not stop a running instance, and resolved briefly fights SmartDNS for :53
   on first install); remove the resolv.conf symlink and write a static
   `nameserver 127.0.0.1` — all three, or :53 self-collides / dnsmasq fights resolvconf
8. Enable systemd-networkd; disable NetworkManager / ifupdown management of
   router interfaces (they fight networkd for the same NICs). Same step also
   disables cloud-init/netplan network management (move `/etc/netplan/*.yaml`
   aside, write `/etc/cloud/cloud.cfg.d/99-luban-disable-network-config.cfg`
   with `network: {config: disabled}`, remove any
   `/run/systemd/network/10-netplan-*` netplan already wrote) — see Field-Test
   Log entry below for why this is load-bearing, not cosmetic.
9. Print status + first-login URL

The script must be idempotent — safe to re-run on an already-installed system.
`install.sh` never writes `config.json` at all — `luban` seeds a valid default
config on first boot if none exists. This avoids two independent producers of
the config schema (a bash-templated default vs. the Go struct defaults)
silently drifting apart; the Go loader is the only thing that ever creates a
default `config.json`.

**Offline installation**: `scripts/make-offline-bundle.sh` (run on the builder)
produces a self-contained `luban-offline-<version>-<arch>.tar.gz` containing
all apt `.deb` files with full dependency closure (via `apt-get install
--download-only --reinstall`), the SmartDNS GitHub release tarball, and the
luban payload. `install.sh --offline <bundle-dir>` installs from that bundle
with no network access: apt repo setup and all GitHub/curl fetches are
skipped; apt packages are installed from `debs/` in a single transaction so
dpkg ordering is handled automatically. All post-install steps (config
generation, resolved masking, systemd setup, idempotency rules) are identical
to the online path. Builder-must-match-target caveat: the bundle must be built
on a Debian system with the same release and CPU architecture as the target
router, because `.deb` files are not cross-version compatible and apt's dep
solver uses the builder's installed-package state. The Minisforum MS-A2 PVE
test VM (Debian 13, amd64) is the recommended builder for amd64 bundles. See
[docs/offline-install.md](docs/offline-install.md) for the full workflow.

## Future Features

- **Upgrade** — remote upgrade (check + download + stage, reboot at 3:00 AM);
  auto upgrade (daily 3:00 AM check, disabled by default); local file upload.
  Updates: `luban`, `www/`, `templates/`, `boards/`. Never touched: `config.json`,
  `boards.d/`, Caddyfile. Previous version in `.prev/`, auto-rollback on crash.
- **Static DNS** — user-defined custom records via Web UI (e.g. `nas.lan → 192.168.x.x`);
  restricted to `.lan` (never `.local`, reserved for mDNS)
- **mDNS** — avahi-daemon advertising `router.local` over multicast (the correct
  way to provide `.local` names)
- **Static leases** — DHCP address reservation by MAC via Web UI
- **Loop detection** — probe frames on LAN ports, alert + optional auto-disable on loop
- **DDNS** — dynamic DNS client, selectable per domain: IPv4 / IPv6 / IPv4+IPv6
- **Port forwarding** — DNAT rules via Web UI
- **UPnP / NAT-PMP** — automatic port mapping for LAN applications
- **Backup / restore** — config.json export download + upload restore
- **Factory reset** — delete config.json, reboot into setup wizard
- **Log viewer** — Web UI for browsing journald logs with filtering
- **i18n** — Chinese + English UI
- **hostapd** — Wi-Fi AP mode (LAN-side wireless); `bridge=br0` in
  hostapd.conf joins the bridge by itself — no `.network` file for the
  wireless interface (field-tested)
- **WireGuard** — VPN tunnel management
- **MosDNS** — alternative DNS engine (rule-based DNS routing/splitting)
- **AdGuard Home** — DNS-level ad blocking + parental controls

## Repository Layout (monorepo)

One repo holds everything that ships: Go backend, web frontend, service templates, board
profiles, install script. A release is one artifact built from one commit — no cross-repo
version skew.

Module naming: directory = package name under the `@router/*` scope. `router/web` is the JS
workspace package `@router/web`; `router/api` is the Go backend (same naming convention, not
an npm package — Go modules don't enter the workspace).

```
linux-router/            # monorepo
├── router/
│   ├── web/             # @router/web — Vite + React + TS; `vite build` → www/ in the release
│   ├── api/             # @router/api — Go source → builds the `luban` binary
│   ├── templates/       # service config templates (shipped as-is)
│   └── boards/          # board profiles (shipped as-is)
├── scripts/
│   └── install.sh
└── pnpm-workspace.yaml  # JS packages only (router/web)
```

Release tarball (GitHub Releases) is assembled from the monorepo: `luban` (per-arch) + `www/` +
`templates/` + `boards/`. The device never sees the repo — it only receives the release
artifact (matches the existing "device only receives static files" rule).

## Deployed Layout

```
/opt/router/
├── luban            # Go binary
├── www/             # frontend (Caddy serves)
├── templates/       # service config templates
├── boards/          # system board profiles (updated with releases)
├── boards.d/        # user-added board profiles (never touched by updates)
├── config.json      # single source of truth
└── config.json.bak  # rollback target
```

## Naming

Project name: **Luban**, chosen 2026-08-12.

| Artifact | Name |
|---|---|
| Go binary | `luban` (renamed from `rui` on 2026-08-13; product name = binary name) |
| Go module | `luban` |
| JS workspace package | `@router/web` (unchanged) |
| Web UI title | "Luban" |

The binary was originally named `rui` and renamed to `luban` on 2026-08-13 so
the product name and binary name are identical. The Go module was already
`luban` before this rename. The `install.sh` deploy step removes any stale
`/opt/router/rui` left by older installs. The JS package `@router/web` retains
its existing scope (`@router`) — renaming would require pnpm-workspace changes
with no functional benefit.

The PPPoE dialer unit is named plainly `pppd.service` (not `luban-pppd.service`):
it just runs stock `pppd` for the WAN dial and Debian's `ppp` package ships no
conflicting systemd unit of its own, so the plain name was free and deliberately
chosen over a project-prefixed one.

## Field-Test Log

2026-08-12 — first end-to-end validation, on a PVE VM (Debian 13). See
[docs/test-environment.md](docs/test-environment.md) for the environment.
Bug classes found, one line each — treat these as future-proofing rules, not
one-off fixes:

- **Config schema mismatch** between `install.sh`'s fallback default config
  and the Go loader's expected shape — resolved by removing the fallback
  entirely (see Installation section above); the Go struct is the only
  source of a default `config.json`.
- **Wrong apt package name** for a runtime dependency — verify exact Debian
  13 package names against `apt-cache search`/official repo before adding to
  `install.sh`, not from memory of a prior distro version.
- **Stale GitHub release asset names** — the download step in `install.sh`
  assumed asset filenames that didn't match what the release workflow
  actually produced; keep the two in lockstep or derive the name
  programmatically.
- **`:53` startup race** between dnsmasq and SmartDNS/systemd-resolved before
  the resolved-masking sequence was correct (see Installation section: stop
  before mask, remove the resolv.conf symlink, static `nameserver 127.0.0.1`).
- **Debian's `caddy` package postinst stub** silently satisfied an
  "is caddy installed" existence check without actually running a usable
  caddy — existence guards in install scripts must check for a working
  service, not just a binary/package record.
- **`.bak` files loaded as live dnsmasq config** — root cause of the
  `CONFIG_DIR` fix documented above under Services.
- **tmpfs flag file** — the unconfirmed-apply flag was initially placed
  under a tmpfs-backed path, defeating the boot-time rollback check (the
  flag vanishes on reboot exactly when it's needed); it must live on
  persistent storage (`/opt/router/unconfirmed-apply`).
- **systemd unit-name drift between Go and systemd** — the Go apply pipeline
  referenced a service unit name that didn't match the actual `.service`
  file on disk; unit names referenced from Go code must be checked against
  `router/systemd/*.service` literally, not assumed from convention.
- **Backup-scheme divergence between Go and shell** — the Go writer and
  `router-rollback.sh` briefly disagreed on the backup file naming/location
  contract; this is exactly what `rollback_contract_test.go` now guards
  against.
- **Frontend types written against an unimplemented API shape** — 
  `router/web/src/api/types.ts` had fields the Go backend didn't yet return,
  causing silent `undefined`s in the UI instead of a build/type error;
  frontend types must track what the API actually returns, verified against
  a real response, not the planned schema.
- **Null-array marshaling** — see the "Go nil slices marshal to JSON null"
  rule in AGENTS.md; found when a DHCP lease list rendered as empty in the UI
  because the frontend didn't handle `null`.
- **SPA fallback missing** — see the Caddy revision documented above under
  Web Stack; React Router paths 404'd on direct navigation/refresh before
  `try_files {path} /index.html` was added.
- **cloud-init/netplan silently governed the WAN NIC instead of luban's
  networkd config** (2026-08-13, found while debugging bridge mode). Symptom:
  bridge mode never brought up an uplink — the WAN NIC never joined `br0`, so
  no DHCP lease. Root cause: cloud-init's netplan writes
  `/run/systemd/network/10-netplan-<iface>.network` on every boot, and
  systemd-networkd applies only the *first* matching `.network` file by
  filename sort across `/usr/lib`, `/run`, `/etc` combined —
  `10-netplan-eth0` sorts before luban's `10-wan`, so netplan's file won,
  silently. Invisible in routed mode (both configs happened to do plain DHCP
  on the same interface, so the wrong one working looked identical to the
  right one working); fatal in bridge mode, where only luban's `.network`
  knows to join the NIC to `br0`. Fix: `install.sh` Step 10 now disables
  cloud-init/netplan network management entirely (see Installation section
  above). Residual hardening idea, not implemented: luban's own `.network`
  files could additionally be named to sort ahead of netplan's
  (e.g. `05-*` instead of `10-*`) as defense-in-depth in case something
  re-enables netplan later; the install.sh disable step is the primary fix
  and this is not currently needed on top of it.
