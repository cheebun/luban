// zod validators for every form in the app (NetworkPage's WAN/LAN/MTU
// sections, DnsPage's upstream list, LoginPage's login + password-change
// forms). Passed straight into @tanstack/react-form's `validators` option —
// TanStack Form implements the Standard Schema spec, so a zod schema works
// there with no adapter package. Field-level errors are attached via
// `ctx.addIssue({ path: [...] })` inside `.superRefine`, matching the field
// names each form's `<Field name="...">` uses, so `field.state.meta.errors`
// carries the same messages the old hand-written validators produced.
import { z } from "zod";
import { WanModeSchema } from "../api/schemas.ts";
import { validateDnsUpstream } from "./dns.ts";
import { ipToInt, isValidIPv4, maskToPrefix, networkAddress } from "./network.ts";

// ---- WAN section ----------------------------------------------------------

export const wanFormSchema = z
  .object({
    mode: WanModeSchema,
    ip: z.string(),
    mask: z.string(),
    gateway: z.string(),
    dns1: z.string(),
    dns2: z.string(),
    username: z.string(),
    password: z.string(),
  })
  .superRefine((v, ctx) => {
    if (v.mode === "static") {
      if (!v.ip) ctx.addIssue({ code: "custom", path: ["ip"], message: "请输入 IP 地址" });
      else if (!isValidIPv4(v.ip))
        ctx.addIssue({ code: "custom", path: ["ip"], message: "IP 地址格式无效" });

      if (!v.mask) ctx.addIssue({ code: "custom", path: ["mask"], message: "请输入子网掩码" });
      else if (maskToPrefix(v.mask) === null)
        ctx.addIssue({
          code: "custom",
          path: ["mask"],
          message: "子网掩码无效（必须是连续掩码，如 255.255.255.0）",
        });

      if (!v.gateway) ctx.addIssue({ code: "custom", path: ["gateway"], message: "请输入默认网关" });
      else if (!isValidIPv4(v.gateway))
        ctx.addIssue({ code: "custom", path: ["gateway"], message: "网关地址格式无效" });

      if (!v.dns1) ctx.addIssue({ code: "custom", path: ["dns1"], message: "请输入首选 DNS" });
      else if (!isValidIPv4(v.dns1))
        ctx.addIssue({ code: "custom", path: ["dns1"], message: "首选 DNS 格式无效" });
      if (v.dns2 && !isValidIPv4(v.dns2))
        ctx.addIssue({ code: "custom", path: ["dns2"], message: "备用 DNS 格式无效" });
    } else if (v.mode === "pppoe") {
      if (!v.username) ctx.addIssue({ code: "custom", path: ["username"], message: "请输入用户名" });
      if (!v.password) ctx.addIssue({ code: "custom", path: ["password"], message: "请输入密码" });
    }
  });

export type WanFormValues = z.infer<typeof wanFormSchema>;

// ---- LAN section ------------------------------------------------------

export const lanFormSchema = z
  .object({
    ip: z.string(),
    mask: z.string(),
    dhcpEnabled: z.boolean(),
    poolStart: z.string(),
    poolEnd: z.string(),
    leaseHours: z.number(),
    dnsMode: z.enum(["auto", "manual"]),
    dns1: z.string(),
    dns2: z.string(),
  })
  .superRefine((v, ctx) => {
    if (!v.ip) ctx.addIssue({ code: "custom", path: ["ip"], message: "请输入 IP 地址" });
    else if (!isValidIPv4(v.ip))
      ctx.addIssue({ code: "custom", path: ["ip"], message: "IP 地址格式无效" });

    if (!v.mask) ctx.addIssue({ code: "custom", path: ["mask"], message: "请输入子网掩码" });
    else if (maskToPrefix(v.mask) === null)
      ctx.addIssue({
        code: "custom",
        path: ["mask"],
        message: "子网掩码无效（必须是连续掩码，如 255.255.255.0）",
      });

    const lanPrefix = maskToPrefix(v.mask);
    const lanIpInt = ipToInt(v.ip);
    const lanNetwork =
      lanIpInt !== null && lanPrefix !== null ? networkAddress(lanIpInt, lanPrefix) : null;
    const poolSize = lanPrefix !== null ? 2 ** (32 - lanPrefix) : null;

    if (v.dhcpEnabled) {
      const startInt = ipToInt(v.poolStart);
      const endInt = ipToInt(v.poolEnd);
      let poolStartError = false;
      let poolEndError = false;

      if (!v.poolStart) {
        ctx.addIssue({ code: "custom", path: ["poolStart"], message: "请输入起始地址" });
        poolStartError = true;
      } else if (startInt === null) {
        ctx.addIssue({ code: "custom", path: ["poolStart"], message: "起始地址格式无效" });
        poolStartError = true;
      } else if (
        lanNetwork !== null &&
        poolSize !== null &&
        (networkAddress(startInt, lanPrefix!) !== lanNetwork ||
          startInt < lanNetwork ||
          startInt >= lanNetwork + poolSize)
      ) {
        ctx.addIssue({
          code: "custom",
          path: ["poolStart"],
          message: "起始地址不在 LAN 子网范围内",
        });
        poolStartError = true;
      }

      if (!v.poolEnd) {
        ctx.addIssue({ code: "custom", path: ["poolEnd"], message: "请输入结束地址" });
        poolEndError = true;
      } else if (endInt === null) {
        ctx.addIssue({ code: "custom", path: ["poolEnd"], message: "结束地址格式无效" });
        poolEndError = true;
      } else if (
        lanNetwork !== null &&
        poolSize !== null &&
        (networkAddress(endInt, lanPrefix!) !== lanNetwork ||
          endInt < lanNetwork ||
          endInt >= lanNetwork + poolSize)
      ) {
        ctx.addIssue({
          code: "custom",
          path: ["poolEnd"],
          message: "结束地址不在 LAN 子网范围内",
        });
        poolEndError = true;
      }

      if (!poolStartError && !poolEndError && startInt !== null && endInt !== null && startInt >= endInt) {
        ctx.addIssue({
          code: "custom",
          path: ["poolEnd"],
          message: "结束地址必须大于起始地址",
        });
      }

      if (!Number.isInteger(v.leaseHours) || v.leaseHours < 1) {
        ctx.addIssue({ code: "custom", path: ["leaseHours"], message: "租期至少为 1 小时" });
      }
    }

    if (v.dnsMode === "manual") {
      if (!v.dns1) ctx.addIssue({ code: "custom", path: ["dns1"], message: "请输入首选 DNS" });
      else if (!isValidIPv4(v.dns1))
        ctx.addIssue({ code: "custom", path: ["dns1"], message: "首选 DNS 格式无效" });
      if (v.dns2 && !isValidIPv4(v.dns2))
        ctx.addIssue({ code: "custom", path: ["dns2"], message: "备用 DNS 格式无效" });
    }
  });

export type LanFormValues = z.infer<typeof lanFormSchema>;

// ---- MTU/MSS section --------------------------------------------------

// effectiveMtu depends on the outer WAN mode (PPPoE defaults to 1492,
// everything else to 1500), which isn't itself an MTU/MSS field — the caller
// builds this schema per-render from the current `config.wan.mode`.
export function buildMtuMssFormSchema(wanMode: z.infer<typeof WanModeSchema>) {
  return z.object({ mtu: z.number(), mss: z.number() }).superRefine((v, ctx) => {
    if (v.mtu !== 0 && (v.mtu < 576 || v.mtu > 9200)) {
      ctx.addIssue({ code: "custom", path: ["mtu"], message: "MTU 需在 576–9200 之间，留空为自动" });
    }
    const effectiveMtu = v.mtu || (wanMode === "pppoe" ? 1492 : 1500);
    const mssMax = effectiveMtu - 40;
    if (v.mss !== 0 && (v.mss < 536 || v.mss > mssMax)) {
      ctx.addIssue({
        code: "custom",
        path: ["mss"],
        message: `MSS 需在 536–${mssMax} 之间，留空为自动`,
      });
    }
  });
}

export type MtuMssFormValues = z.infer<ReturnType<typeof buildMtuMssFormSchema>>;

// ---- DNS upstreams ------------------------------------------------------

export const dnsFormSchema = z.object({
  upstreams: z.array(z.string()),
});

// ---- Static DNS records -------------------------------------------------

// isValidLanDNSName mirrors the Go isValidHostname check for RFC1123 labels.
function isValidLanDNSName(name: string): boolean {
  if (!name || name.length > 253) return false;
  const labelRe = /^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?$|^[a-zA-Z0-9]$/;
  return name.split(".").every((l) => l.length > 0 && l.length <= 63 && labelRe.test(l));
}

// isValidIP accepts IPv4 or IPv6 addresses for UX feedback; the Go backend
// is the authoritative validator.
function isValidIP(ip: string): boolean {
  // IPv4
  const ipv4 = /^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$/.exec(ip);
  if (ipv4 && ipv4.slice(1).map(Number).every((n) => n <= 255)) return true;
  // IPv6: must contain colons and only valid hex chars / colons
  return ip.includes(":") && /^[0-9a-fA-F:]+$/.test(ip) && ip.split(":").length <= 9;
}

const RESERVED_LAN_NAMES = new Set(["router.lan", "br0.lan", "modem.lan"]);

// staticDNSRecordSchema mirrors the Go validation rules exactly.
const staticDNSRecordSchema = z
  .object({ name: z.string(), ip: z.string() })
  .superRefine((v, ctx) => {
    const name = v.name.trim().toLowerCase();
    if (name.endsWith(".local")) {
      ctx.addIssue({
        code: "custom",
        path: ["name"],
        message: "仅支持 .lan 域名（.local 为 mDNS 保留，RFC 6762）",
      });
    } else if (!name.endsWith(".lan")) {
      ctx.addIssue({ code: "custom", path: ["name"], message: "仅支持 .lan 域名" });
    } else if (!isValidLanDNSName(name)) {
      ctx.addIssue({ code: "custom", path: ["name"], message: "域名格式无效（须为 RFC1123 标签）" });
    } else if (RESERVED_LAN_NAMES.has(name)) {
      ctx.addIssue({ code: "custom", path: ["name"], message: "该名称为内置保留名称" });
    }
    const ip = v.ip.trim();
    if (!ip) {
      ctx.addIssue({ code: "custom", path: ["ip"], message: "请输入 IP 地址" });
    } else if (!isValidIP(ip)) {
      ctx.addIssue({ code: "custom", path: ["ip"], message: "IP 地址格式无效（支持 IPv4 和 IPv6）" });
    }
  });

export const staticRecordsFormSchema = z
  .object({ records: z.array(staticDNSRecordSchema) })
  .superRefine((v, ctx) => {
    const seen = new Set<string>();
    v.records.forEach((rec, i) => {
      const key = `${rec.name.trim().toLowerCase()}|${rec.ip.trim()}`;
      if (seen.has(key)) {
        ctx.addIssue({ code: "custom", path: ["records", i, "name"], message: "重复记录" });
      } else {
        seen.add(key);
      }
    });
  });

export type StaticRecordsFormValues = z.infer<typeof staticRecordsFormSchema>;

export type DnsFormValues = z.infer<typeof dnsFormSchema>;

// dnsFormSchema intentionally doesn't validate each upstream's syntax via
// .superRefine + addIssue (index-keyed field paths work fine with TanStack
// Form, but the per-row error list is simpler to keep as a derived array —
// see DnsPage.tsx, which maps `validateDnsUpstream` over `upstreams` directly,
// same as before).
export { validateDnsUpstream };

// ---- Login / password change -------------------------------------------

export const loginFormSchema = z.object({
  username: z.string().min(1, "请输入用户名"),
  password: z.string().min(1, "请输入密码"),
});

export type LoginFormValues = z.infer<typeof loginFormSchema>;

export const changePasswordFormSchema = z
  .object({
    newPassword: z.string(),
    confirmPassword: z.string(),
  })
  .superRefine((v, ctx) => {
    if (v.newPassword.length < 8) {
      ctx.addIssue({ code: "custom", path: ["newPassword"], message: "密码至少需要 8 位" });
    }
    if (v.newPassword !== v.confirmPassword) {
      ctx.addIssue({
        code: "custom",
        path: ["confirmPassword"],
        message: "两次输入的新密码不一致",
      });
    }
  });

export type ChangePasswordFormValues = z.infer<typeof changePasswordFormSchema>;

// ---- Wizard management step --------------------------------------------

export const wizardManagementFormSchema = z
  .object({
    lanIp: z.string(),
    lanMask: z.string(),
    password: z.string(),
    confirmPassword: z.string(),
  })
  .superRefine((v, ctx) => {
    if (!v.lanIp) ctx.addIssue({ code: "custom", path: ["lanIp"], message: "请输入 LAN IP 地址" });
    else if (!isValidIPv4(v.lanIp))
      ctx.addIssue({ code: "custom", path: ["lanIp"], message: "IP 地址格式无效" });

    if (!v.lanMask) ctx.addIssue({ code: "custom", path: ["lanMask"], message: "请输入子网掩码" });
    else if (maskToPrefix(v.lanMask) === null)
      ctx.addIssue({
        code: "custom",
        path: ["lanMask"],
        message: "子网掩码无效（必须是连续掩码，如 255.255.255.0）",
      });

    if (v.password.length < 8)
      ctx.addIssue({ code: "custom", path: ["password"], message: "密码至少需要 8 位" });
    if (v.password !== v.confirmPassword)
      ctx.addIssue({ code: "custom", path: ["confirmPassword"], message: "两次密码不一致" });
  });

export type WizardManagementFormValues = z.infer<typeof wizardManagementFormSchema>;

// TanStack Form field errors are `unknown[]` — Standard Schema (zod) issue
// objects carry `.message`, while plain function validators (used for
// MtuMssForm, whose schema depends on external wan-mode state) push bare
// strings. This normalizes either shape for display.
export function fieldErrorText(errors: ReadonlyArray<unknown>): string | undefined {
  const e = errors[0];
  if (e === undefined || e === null) return undefined;
  if (typeof e === "string") return e;
  if (typeof e === "object" && "message" in e) return String((e as { message: unknown }).message);
  return String(e);
}
