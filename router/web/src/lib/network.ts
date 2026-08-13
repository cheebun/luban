// Pure IPv4/CIDR helpers shared by the WAN/LAN zod validators (src/lib/formSchemas.ts)
// and the network forms themselves. Extracted from the old NetworkPage.tsx so
// the same logic backs both the zod schema (superRefine cross-field checks)
// and the ip/mask <-> CIDR conversion done on submit.

export function ipToInt(ip: string): number | null {
  const m = /^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$/.exec(ip);
  if (!m) return null;
  const parts = m.slice(1).map(Number);
  if (parts.some((p) => p > 255)) return null;
  return ((parts[0] << 24) | (parts[1] << 16) | (parts[2] << 8) | parts[3]) >>> 0;
}

export function isValidIPv4(ip: string): boolean {
  return ipToInt(ip) !== null;
}

export function prefixToMask(prefix: number): string {
  const bits = prefix === 0 ? 0 : (0xffffffff << (32 - prefix)) >>> 0;
  return [(bits >>> 24) & 255, (bits >>> 16) & 255, (bits >>> 8) & 255, bits & 255].join(".");
}

// Returns the CIDR prefix length for a dotted-decimal mask, or null if the
// mask is not a syntactically valid, left-contiguous IPv4 netmask.
export function maskToPrefix(mask: string): number | null {
  const bits = ipToInt(mask);
  if (bits === null) return null;
  let prefix = 0;
  let sawZero = false;
  for (let i = 31; i >= 0; i--) {
    const bit = (bits >>> i) & 1;
    if (bit === 1) {
      if (sawZero) return null;
      prefix++;
    } else {
      sawZero = true;
    }
  }
  return prefix;
}

// One-way split used only to seed local ip/mask edit state from a config
// CIDR string (prefix -> mask is unambiguous, unlike the reverse).
export function splitCidrForEdit(cidr: string): { ip: string; mask: string } {
  const idx = cidr.indexOf("/");
  if (idx === -1) return { ip: cidr, mask: "" };
  const ip = cidr.slice(0, idx);
  const prefix = Number(cidr.slice(idx + 1));
  return {
    ip,
    mask: Number.isInteger(prefix) && prefix >= 0 && prefix <= 32 ? prefixToMask(prefix) : "",
  };
}

export function networkAddress(ipInt: number, prefix: number): number {
  const maskBits = prefix === 0 ? 0 : (0xffffffff << (32 - prefix)) >>> 0;
  return (ipInt & maskBits) >>> 0;
}

export function leaseToHours(lease: string): number | null {
  const m = /^(\d+)h$/.exec(lease);
  return m ? Number(m[1]) : null;
}
