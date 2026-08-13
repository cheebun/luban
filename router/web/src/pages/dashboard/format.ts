// Pure formatting helpers + label maps shared by the dashboard card
// components. No JSX/components here — kept separate so the card files stay
// under the one-component-per-file rule.
import type { InterfaceType, StatusResponse, SystemInfo } from "../../api/index.ts";

export const WAN_MODES: Record<string, string> = {
  dhcp: "DHCP",
  static: "静态 IP",
  pppoe: "PPPoE",
  bridge: "有线桥接",
};

export const IFACE_TYPE_LABELS: Record<InterfaceType, string> = {
  bridge: "桥接",
  wireless: "无线",
  ppp: "拨号",
  ethernet: "有线",
};

export const IFACE_STATE_BADGE: Record<string, "online" | "offline" | "warning"> = {
  up: "online",
  down: "offline",
  unknown: "warning",
};

export const SERVICE_STATUS: Record<string, "online" | "offline" | "warning"> = {
  active: "online",
  inactive: "offline",
  failed: "warning",
};

export const UNKNOWN = "未知";

export function orUnknown(v: string | null | undefined): string {
  return v && v.length > 0 ? v : UNKNOWN;
}

// Formats a byte count as a human-readable size (GiB when large enough,
// otherwise MiB). Returns 未知 for null (no data source available, e.g.
// /proc/meminfo missing on macOS dev).
export function formatBytes(bytes: number | null | undefined): string {
  if (bytes === null || bytes === undefined || !Number.isFinite(bytes)) return UNKNOWN;
  const gib = bytes / 1024 ** 3;
  if (gib >= 1) return `${gib.toFixed(1)} GiB`;
  const mib = bytes / 1024 ** 2;
  return `${mib.toFixed(0)} MiB`;
}

export function formatTemperature(c: number | null | undefined): string {
  if (c === null || c === undefined || !Number.isFinite(c)) return UNKNOWN;
  return `${c.toFixed(1)} °C`;
}

export function formatLoadAvg(loadavg: SystemInfo["loadavg"]): string {
  if (!loadavg) return UNKNOWN;
  return `${loadavg.load1.toFixed(2)} / ${loadavg.load5.toFixed(2)} / ${loadavg.load15.toFixed(2)}`;
}

// link_speed_mbps is null whenever the link is down, virtual, or the driver
// doesn't report speed (most bridge/PPP/wireless interfaces) — render 未知
// rather than a misleading "0 Mb/s".
export function formatLinkSpeed(mbps: number | null): string {
  return mbps === null ? UNKNOWN : `${mbps} Mb/s`;
}

// status.uptime is a string like "3526.09s" (raw /proc/uptime field), not a number.
export function formatUptime(uptime: string): string {
  const secs = parseFloat(uptime);
  if (!Number.isFinite(secs) || secs <= 0) return "—";
  const h = Math.floor(secs / 3600);
  const m = Math.floor((secs % 3600) / 60);
  return `${h}h ${m}m`;
}

// Finds the first global-scope IPv4 address on the given interface from the
// live `ip -j addr` snapshot. Returns undefined if the interface is absent
// or has no address yet (e.g. WAN still negotiating DHCP/PPPoE).
export function findWanIp(addrs: StatusResponse["addrs"], ifname: string): string | undefined {
  const iface = addrs.find((a) => a.ifname === ifname);
  return iface?.addr_info.find((a) => a.family === "inet" && a.scope === "global")?.local;
}
