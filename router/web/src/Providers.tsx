// SSR-aware QueryClient singleton, per the official TanStack Query pattern
// (https://tanstack.com/query/latest/docs/framework/react/guides/ssr).
// This app is a client-only Vite SPA today — `isServer` is always false at
// runtime — but the user explicitly asked to keep the SSR branch for
// future-proofing (e.g. a move to a server-rendered framework later).
//
// `environmentManager.isServer()` (mentioned in some docs snippets) is not a
// public export of @tanstack/query-core 5.101.4; the real export is the
// `isServer` boolean re-exported from @tanstack/react-query. `isServer()` as
// a method exists internally on `queryCore`'s environment manager but isn't
// part of the public API surface, so we use the boolean per its own
// deprecation note ("use `environmentManager.isServer()` instead" refers to
// internal library code, not consumer code).
import { isServer, QueryClient, QueryClientProvider } from "@tanstack/react-query";

function makeQueryClient() {
  return new QueryClient({ defaultOptions: { queries: { staleTime: 60 * 1000 } } });
}

// On the server, always make a new query client — there's no shared request
// state to leak between requests. In the browser, reuse a single client
// across renders (React can suspend/remount during render, and creating a
// fresh client each time would drop the cache and any in-flight queries).
let browserQueryClient: QueryClient | undefined;

function getQueryClient() {
  if (isServer) return makeQueryClient();
  if (!browserQueryClient) browserQueryClient = makeQueryClient();
  return browserQueryClient;
}

export function Providers({ children }: { children: React.ReactNode }) {
  // NOT useState: on the client we want the same client across re-renders of
  // this component (e.g. Suspense boundaries re-rendering Providers), which
  // getQueryClient()'s module-level singleton already guarantees — wrapping
  // it in useState would still work but is redundant and, unlike this form,
  // would not survive a component remount during Suspense.
  const queryClient = getQueryClient();
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
}
