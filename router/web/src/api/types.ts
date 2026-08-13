// Re-export shim: every wire type below is now derived from the zod schemas
// in schemas.ts (single source of truth for both runtime validation and TS
// types), so component imports from "./types.ts" / "./index.ts" keep working
// unchanged. See schemas.ts for field-level docs and the Go struct each type
// mirrors.
export type {
  WanMode,
  WanStaticConfig,
  WanPppoeConfig,
  WanConfig,
  LanDnsMode,
  LanDhcpConfig,
  LanConfig,
  Ipv6Config,
  DnsConfig,
  SystemConfig,
  RouterConfig,
  DhcpLease,
  ServiceHealth,
  IpAddress,
  AddrInfo,
  RouteInfo,
  LoadAvg,
  CpuInfo,
  MemoryInfo,
  DiskInfo,
  InterfaceType,
  InterfaceInfo,
  SystemInfo,
  StatusResponse,
  HealthComponent,
  HealthService,
  HealthTakeoverCheck,
  HealthResponse,
  ServiceRestartRequest,
  LoginRequest,
  LoginResponse,
  PasswordRequest,
  ApplyRequest,
  ApplyResponse,
  LogResponse,
} from "./schemas.ts";

export interface ApiError {
  status: number;
  message: string;
}
