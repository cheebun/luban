// Zod schemas for every /api/* request and response body. Response schemas
// are the single source of truth for both runtime validation (parsed in
// client.ts right after fetch) and the TS types the rest of the app imports
// (via z.infer, re-exported from types.ts so existing imports don't churn).
//
// Go marshals a nil slice as JSON `null`, not `[]` (see AGENTS.md "Go nil
// slices marshal to JSON `null`"). Every array field below carries
// `.nullish().transform((v) => v ?? [])` so callers never need an `?? []`
// guard — this replaces the old hand-written `arrOrEmpty()` normalizer.
import { z } from "zod";

const nullableArray = <T extends z.ZodTypeAny>(item: T) =>
  z.array(item).nullish().transform((v) => v ?? []);

// ---- Config -----------------------------------------------------------

export const WanModeSchema = z.enum(["dhcp", "static", "pppoe", "bridge"]);

export const WanStaticConfigSchema = z.object({
  address: z.string(),
  gateway: z.string(),
  dns: nullableArray(z.string()),
});

export const WanPppoeConfigSchema = z.object({
  username: z.string(),
  password: z.string(),
});

export const WanConfigSchema = z.object({
  mode: WanModeSchema,
  interface: z.string(),
  static: WanStaticConfigSchema,
  pppoe: WanPppoeConfigSchema,
  modem_ip: z.string(),
  mtu: z.number(),
  mss: z.number(),
});

export const LanDnsModeSchema = z.enum(["auto", "manual"]);

export const LanDhcpConfigSchema = z.object({
  enabled: z.boolean(),
  start: z.string(),
  end: z.string(),
  lease: z.string(),
  dns_mode: LanDnsModeSchema,
  dns_servers: nullableArray(z.string()),
});

export const LanConfigSchema = z.object({
  interfaces: nullableArray(z.string()),
  address: z.string(),
  dhcp: LanDhcpConfigSchema,
});

export const Ipv6ConfigSchema = z.object({
  enabled: z.boolean(),
  lan_prefix_len: z.union([z.literal("auto"), z.number()]),
});

export const DnsConfigSchema = z.object({
  upstreams: nullableArray(z.string()),
});

export const SystemConfigSchema = z.object({
  hostname: z.string(),
});

export const RouterConfigSchema = z.object({
  system: SystemConfigSchema,
  wan: WanConfigSchema,
  lan: LanConfigSchema,
  ipv6: Ipv6ConfigSchema,
  dns: DnsConfigSchema,
});

// ---- Status -------------------------------------------------------------

export const DhcpLeaseSchema = z.object({
  mac: z.string(),
  ip: z.string(),
  hostname: z.string(),
  expiry: z.number(),
});

export const ServiceHealthSchema = z.object({
  name: z.string(),
  active: z.string(),
});

export const IpAddressSchema = z.object({
  family: z.string(),
  local: z.string(),
  prefixlen: z.number(),
  scope: z.string(),
});

export const AddrInfoSchema = z.object({
  ifname: z.string(),
  flags: nullableArray(z.string()),
  addr_info: nullableArray(IpAddressSchema),
});

export const RouteInfoSchema = z.object({
  dst: z.string(),
  gateway: z.string(),
  dev: z.string(),
  protocol: z.string(),
});

export const LoadAvgSchema = z.object({
  load1: z.number(),
  load5: z.number(),
  load15: z.number(),
});

export const CpuInfoSchema = z.object({
  model: z.string(),
  cores: z.number(),
  temperature_c: z.number().nullable(),
});

export const MemoryInfoSchema = z.object({
  total_bytes: z.number(),
  available_bytes: z.number(),
});

export const DiskInfoSchema = z.object({
  total_bytes: z.number(),
  free_bytes: z.number(),
});

export const InterfaceTypeSchema = z.enum(["bridge", "wireless", "ppp", "ethernet"]);

export const InterfaceInfoSchema = z.object({
  name: z.string(),
  type: InterfaceTypeSchema,
  mac: z.string(),
  ipv4: nullableArray(z.string()),
  ipv6: nullableArray(z.string()),
  state: z.string(),
  link_speed_mbps: z.number().nullable(),
});

export const SystemInfoSchema = z.object({
  hostname: z.string(),
  os: z.string().nullable(),
  kernel: z.string().nullable(),
  arch: z.string().nullable(),
  uptime: z.string(),
  loadavg: LoadAvgSchema.nullable(),
  cpu: CpuInfoSchema.nullable(),
  memory: MemoryInfoSchema.nullable(),
  disk: DiskInfoSchema.nullable(),
  interfaces: nullableArray(InterfaceInfoSchema),
  gateway_v4: z.string().nullable(),
  gateway_v6: z.string().nullable(),
});

export const StatusResponseSchema = z.object({
  uptime: z.string(),
  addrs: nullableArray(AddrInfoSchema),
  routes: nullableArray(RouteInfoSchema),
  leases: nullableArray(DhcpLeaseSchema),
  services: nullableArray(ServiceHealthSchema),
  system: SystemInfoSchema,
});

// ---- Health ---------------------------------------------------------------

export const HealthComponentSchema = z.object({
  name: z.string(),
  installed: z.boolean(),
  version: z.string().nullable(),
  detail: z.string(),
});

export const HealthServiceSchema = z.object({
  name: z.string(),
  active: z.string(),
  detail: z.string(),
  restartable: z.boolean(),
});

export const HealthTakeoverCheckSchema = z.object({
  name: z.string(),
  ok: z.boolean(),
  detail: z.string(),
});

export const HealthResponseSchema = z.object({
  components: nullableArray(HealthComponentSchema),
  services: nullableArray(HealthServiceSchema),
  takeover: nullableArray(HealthTakeoverCheckSchema),
});

export const ServiceRestartRequestSchema = z.object({
  name: z.string(),
});

// ---- Auth / misc ------------------------------------------------------

export const LoginRequestSchema = z.object({
  username: z.string(),
  password: z.string(),
});

export const LoginResponseSchema = z.object({
  ok: z.boolean(),
  must_change: z.boolean().optional(),
});

export const PasswordRequestSchema = z.object({
  current: z.string(),
  new: z.string(),
});

export const ApplyRequestSchema = z.object({
  unchecked: z.boolean().optional(),
});

export const ApplyResponseSchema = z.object({
  ok: z.boolean(),
  rollback_at: z.number().optional(),
});

export const LogResponseSchema = z.object({
  log: z.string(),
});

export const OkResponseSchema = z.object({
  ok: z.boolean(),
});

// ---- Inferred types -------------------------------------------------------
// These are what the rest of the app imports (re-exported from types.ts).

export type WanMode = z.infer<typeof WanModeSchema>;
export type WanStaticConfig = z.infer<typeof WanStaticConfigSchema>;
export type WanPppoeConfig = z.infer<typeof WanPppoeConfigSchema>;
export type WanConfig = z.infer<typeof WanConfigSchema>;
export type LanDnsMode = z.infer<typeof LanDnsModeSchema>;
export type LanDhcpConfig = z.infer<typeof LanDhcpConfigSchema>;
export type LanConfig = z.infer<typeof LanConfigSchema>;
export type Ipv6Config = z.infer<typeof Ipv6ConfigSchema>;
export type DnsConfig = z.infer<typeof DnsConfigSchema>;
export type SystemConfig = z.infer<typeof SystemConfigSchema>;
export type RouterConfig = z.infer<typeof RouterConfigSchema>;
export type DhcpLease = z.infer<typeof DhcpLeaseSchema>;
export type ServiceHealth = z.infer<typeof ServiceHealthSchema>;
export type IpAddress = z.infer<typeof IpAddressSchema>;
export type AddrInfo = z.infer<typeof AddrInfoSchema>;
export type RouteInfo = z.infer<typeof RouteInfoSchema>;
export type LoadAvg = z.infer<typeof LoadAvgSchema>;
export type CpuInfo = z.infer<typeof CpuInfoSchema>;
export type MemoryInfo = z.infer<typeof MemoryInfoSchema>;
export type DiskInfo = z.infer<typeof DiskInfoSchema>;
export type InterfaceType = z.infer<typeof InterfaceTypeSchema>;
export type InterfaceInfo = z.infer<typeof InterfaceInfoSchema>;
export type SystemInfo = z.infer<typeof SystemInfoSchema>;
export type StatusResponse = z.infer<typeof StatusResponseSchema>;
export type HealthComponent = z.infer<typeof HealthComponentSchema>;
export type HealthService = z.infer<typeof HealthServiceSchema>;
export type HealthTakeoverCheck = z.infer<typeof HealthTakeoverCheckSchema>;
export type HealthResponse = z.infer<typeof HealthResponseSchema>;
export type ServiceRestartRequest = z.infer<typeof ServiceRestartRequestSchema>;
export type LoginRequest = z.infer<typeof LoginRequestSchema>;
export type LoginResponse = z.infer<typeof LoginResponseSchema>;
export type PasswordRequest = z.infer<typeof PasswordRequestSchema>;
export type ApplyRequest = z.infer<typeof ApplyRequestSchema>;
export type ApplyResponse = z.infer<typeof ApplyResponseSchema>;
export type LogResponse = z.infer<typeof LogResponseSchema>;
