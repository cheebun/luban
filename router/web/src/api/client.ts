import type { z } from "zod";

export class ApiRequestError extends Error {
  constructor(
    public readonly status: number,
    message: string,
  ) {
    super(message);
    this.name = "ApiRequestError";
  }
}

// /api/login and /api/password are only ever called from LoginPage, which
// already handles their 401s locally (bad credentials, wrong current
// password). A 401 from any other endpoint means the session cookie is
// missing or expired — that's a session-invalidation event the whole app
// needs to react to, not just the caller.
const SESSION_EXEMPT_PATHS = new Set(["/api/login", "/api/password"]);

type UnauthorizedHandler = () => void;
let unauthorizedHandler: UnauthorizedHandler | null = null;

/** Registered once by the top-level AuthProvider; not for ad-hoc use elsewhere. */
export function setUnauthorizedHandler(handler: UnauthorizedHandler | null): void {
  unauthorizedHandler = handler;
}

async function request<T>(
  path: string,
  schema: z.ZodType<T>,
  options?: RequestInit,
): Promise<T> {
  const resp = await fetch(path, {
    headers: { "Content-Type": "application/json" },
    ...options,
  });
  if (!resp.ok) {
    const text = await resp.text().catch(() => resp.statusText);
    if (resp.status === 401 && !SESSION_EXEMPT_PATHS.has(path)) {
      unauthorizedHandler?.();
    }
    throw new ApiRequestError(resp.status, text);
  }
  const json: unknown = await resp.json();
  return parseResponse(path, schema, json);
}

/**
 * Guard against backend/frontend contract drift. Every response is parsed
 * against its zod schema right after fetch; a mismatch is always logged
 * loudly (field path + expected/actual), naming exactly which field broke
 * the contract, instead of surfacing later as `Cannot read properties of
 * undefined` deep inside a component. In dev this also throws, so drift is
 * caught at the call site during development; production falls back to the
 * raw (unvalidated) payload so a single unexpected field doesn't hard-crash
 * the admin UI on a live router.
 */
function parseResponse<T>(path: string, schema: z.ZodType<T>, json: unknown): T {
  const result = schema.safeParse(json);
  if (!result.success) {
    const issues = result.error.issues
      .map((issue) => `${issue.path.join(".") || "<root>"}: ${issue.message}`)
      .join("; ");
    // eslint-disable-next-line no-console -- intentional contract-drift alarm, not debug leftover
    console.error(`${path}: response failed schema validation — ${issues}`, json);
    if (import.meta.env.DEV) {
      throw new Error(`${path}: response does not match the expected schema — ${issues}`);
    }
    return json as T;
  }
  return result.data;
}

export async function get<T>(path: string, schema: z.ZodType<T>): Promise<T> {
  return request<T>(path, schema);
}

export async function post<T>(
  path: string,
  schema: z.ZodType<T>,
  body?: unknown,
): Promise<T> {
  return request<T>(path, schema, {
    method: "POST",
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
}

export async function put<T>(
  path: string,
  schema: z.ZodType<T>,
  body?: unknown,
): Promise<T> {
  return request<T>(path, schema, {
    method: "PUT",
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
}

export function isUnauthorized(err: unknown): boolean {
  return err instanceof ApiRequestError && err.status === 401;
}

/**
 * True when `err` never got an HTTP response at all — `fetch` itself threw
 * (offline, DNS failure, connection reset, timeout). False for
 * `ApiRequestError`, which means a response *was* received, even a 4xx/5xx
 * one. Callers on the apply/confirm path use this to distinguish "the router
 * legitimately restarted mid-request" (expected, not an error) from a real
 * validation/render failure the backend reported.
 */
export function isNetworkError(err: unknown): boolean {
  return !(err instanceof ApiRequestError);
}

/**
 * Reachability probe for the apply-dialog poll loop: a bare, unvalidated
 * fetch (no zod parsing, no 401 -> unauthorizedHandler side effect) that
 * treats ANY HTTP response — including 401, since the router may still be
 * mid-restart with a fresh session — as "reachable". Only a network-level
 * throw counts as unreachable. `cache: "no-store"` guards against a stale
 * cached response reporting reachability before the service is actually
 * back up.
 */
export async function probeReachable(path: string): Promise<boolean> {
  try {
    await fetch(path, { cache: "no-store" });
    return true;
  } catch {
    return false;
  }
}
