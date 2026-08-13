// Pure DNS-upstream validation helpers shared by DnsPage's zod schema.
// Mirrors config.ParseDNSUpstream's accept/reject rules on the Go side, so
// invalid entries are caught before submit — keep in sync with that function.
import { isValidIPv4 } from "./network.ts";

export const UPSTREAM_FORMATS_HINT =
  "支持格式：普通 UDP（IP 或 IP:port，如 114.114.114.114）、tcp://host[:port]、" +
  "tls://host[:port]（DoT）、https://host/path（DoH）、quic://host[:port]、h3://host/path";

const SCHEME_HOST_ONLY = new Set(["tcp", "tls", "quic"]);
const SCHEME_URL = new Set(["https", "h3"]);

function isValidIP(host: string): boolean {
  if (isValidIPv4(host)) return true;
  // Loose IPv6 literal check; the server re-validates authoritatively.
  return host.includes(":") && /^[0-9a-fA-F:]+$/.test(host);
}

function isValidHostname(host: string): boolean {
  if (!host || host.length > 253) return false;
  return host
    .split(".")
    .every((label) => /^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$/.test(label));
}

function splitHostPort(s: string): { host: string; port?: string } {
  const bracketed = /^\[([^\]]+)\](?::(\d+))?$/.exec(s);
  if (bracketed) return { host: bracketed[1], port: bracketed[2] };
  const idx = s.lastIndexOf(":");
  if (idx !== -1 && !s.slice(0, idx).includes(":")) {
    return { host: s.slice(0, idx), port: s.slice(idx + 1) };
  }
  return { host: s };
}

function isValidPort(port: string | undefined): boolean {
  if (port === undefined) return true;
  if (!/^\d+$/.test(port)) return false;
  const n = Number(port);
  return n >= 1 && n <= 65535;
}

function validateHostPort(s: string, allowHostname: boolean): string | null {
  const { host, port } = splitHostPort(s);
  if (!host) return "缺少主机名";
  if (!isValidPort(port)) return `端口 "${port ?? ""}" 无效`;
  if (isValidIP(host)) return null;
  if (allowHostname && isValidHostname(host)) return null;
  return allowHostname ? `"${host}" 不是合法的 IP 地址或域名` : `"${host}" 不是合法的 IP 地址`;
}

export function validateDnsUpstream(raw: string): string | null {
  const s = raw.trim();
  if (!s) return "不能为空";

  const schemeIdx = s.indexOf("://");
  if (schemeIdx === -1) {
    return validateHostPort(s, false);
  }
  const scheme = s.slice(0, schemeIdx);
  const rest = s.slice(schemeIdx + 3);

  if (SCHEME_HOST_ONLY.has(scheme)) {
    return validateHostPort(rest, true);
  }
  if (SCHEME_URL.has(scheme)) {
    try {
      const u = new URL(scheme === "h3" ? `https://${rest}` : s);
      if (!u.hostname) return "URL 缺少主机名";
      return null;
    } catch {
      return "URL 格式无效";
    }
  }
  return `不支持的协议 "${scheme}"`;
}
