// TanStack Query hooks for every GET endpoint. Query keys live here so
// mutations.ts (invalidate/optimistic-update targets) and App.tsx (boot
// session probe, which warms the config cache via queryClient.fetchQuery)
// can reference the same keys without re-declaring them.
import { useQuery } from "@tanstack/react-query";
import { getConfig, getHealth, getLogs, getStatus } from "./index.ts";

export const queryKeys = {
  config: ["config"] as const,
  status: ["status"] as const,
  health: ["health"] as const,
  log: ["log"] as const,
};

export function useConfigQuery() {
  return useQuery({ queryKey: queryKeys.config, queryFn: getConfig });
}

// Dashboard polls status every 10s; ApplyDialog's own polling loop calls
// getStatus() directly (not through this hook) since it needs a plain
// one-shot promise per poll tick, not query-cache semantics.
export function useStatusQuery() {
  return useQuery({ queryKey: queryKeys.status, queryFn: getStatus, refetchInterval: 10_000 });
}

export function useHealthQuery() {
  return useQuery({ queryKey: queryKeys.health, queryFn: getHealth, refetchInterval: 15_000 });
}

// LogPage fetches once on mount and otherwise only via its explicit 刷新
// button — no background refetch, matching the old useState/useEffect version.
export function useLogQuery() {
  return useQuery({ queryKey: queryKeys.log, queryFn: getLogs, staleTime: Infinity });
}
