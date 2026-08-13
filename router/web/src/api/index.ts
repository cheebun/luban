export {
  ApiRequestError,
  isNetworkError,
  isUnauthorized,
  probeReachable,
  setUnauthorizedHandler,
} from "./client.ts";

import { get, post, put } from "./client.ts";
import {
  ApplyResponseSchema,
  HealthResponseSchema,
  LoginResponseSchema,
  LogResponseSchema,
  OkResponseSchema,
  RouterConfigSchema,
  StatusResponseSchema,
} from "./schemas.ts";
import type {
  LoginRequest,
  LoginResponse,
  PasswordRequest,
  RouterConfig,
  StatusResponse,
  ApplyRequest,
  ApplyResponse,
  HealthResponse,
  ServiceRestartRequest,
} from "./types.ts";
import {
  WizardStateSchema,
  WizardProbeResponseSchema,
  WizardCompleteResponseSchema,
} from "./wizardSchemas.ts";
import type {
  WizardState,
  WizardProbeResponse,
  WizardCompleteRequest,
  WizardCompleteResponse,
} from "./wizardSchemas.ts";

export type { WizardState, WizardInterface, WizardBoard, WizardCompleteResponse } from "./wizardSchemas.ts";

export type * from "./types.ts";

export function login(req: LoginRequest): Promise<LoginResponse> {
  return post("/api/login", LoginResponseSchema, req);
}

export function changePassword(req: PasswordRequest): Promise<{ ok: boolean }> {
  return post("/api/password", OkResponseSchema, req);
}

export function logout(): Promise<{ ok: boolean }> {
  return post("/api/logout", OkResponseSchema);
}

// Array-typed fields (addrs/routes/leases/services/system.interfaces) are
// normalized null -> [] by StatusResponseSchema itself (see schemas.ts'
// `nullableArray`), so no manual post-processing is needed here anymore.
export function getStatus(): Promise<StatusResponse> {
  return get("/api/status", StatusResponseSchema);
}

export function getHealth(): Promise<HealthResponse> {
  return get("/api/health", HealthResponseSchema);
}

// name must be one of router/api/internal/health.RestartableServices — the
// backend 400s on anything else.
export function restartService(req: ServiceRestartRequest): Promise<{ ok: boolean }> {
  return post("/api/service/restart", OkResponseSchema, req);
}

export function getConfig(): Promise<RouterConfig> {
  return get("/api/config", RouterConfigSchema);
}

// router/api's routeGuarded only accepts PUT for /api/config (GET is read,
// POST 405s) — confirmed live. Do not change back to post().
export function saveConfig(config: RouterConfig): Promise<{ ok: boolean }> {
  return put("/api/config", OkResponseSchema, config);
}

export function applyConfig(req?: ApplyRequest): Promise<ApplyResponse> {
  return post("/api/apply", ApplyResponseSchema, req);
}

export function confirmApply(): Promise<{ ok: boolean }> {
  return post("/api/confirm", OkResponseSchema);
}

export function rollbackApply(): Promise<{ ok: boolean }> {
  return post("/api/rollback", OkResponseSchema);
}

export async function getLogs(): Promise<string[]> {
  const raw = await get("/api/log", LogResponseSchema);
  return raw.log ? raw.log.split("\n").filter((line) => line.length > 0) : [];
}

export function getWizardState(): Promise<WizardState> {
  return get("/api/wizard/state", WizardStateSchema);
}

export function probeWizard(): Promise<WizardProbeResponse> {
  return post("/api/wizard/probe", WizardProbeResponseSchema);
}

export function completeWizard(req: WizardCompleteRequest): Promise<WizardCompleteResponse> {
  return post("/api/wizard/complete", WizardCompleteResponseSchema, req);
}
