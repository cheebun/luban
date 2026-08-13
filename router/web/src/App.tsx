import { useQueryClient } from "@tanstack/react-query";
import { useEffect } from "react";
import { BrowserRouter, Navigate, Route, Routes } from "react-router-dom";
import { twc } from "react-twc";
import { getConfig, setUnauthorizedHandler } from "./api/index.ts";
import { queryKeys } from "./api/queries.ts";
import { Layout } from "./components/layout/Layout.tsx";
import { DashboardPage } from "./pages/DashboardPage.tsx";
import { DnsPage } from "./pages/DnsPage.tsx";
import { HealthPage } from "./pages/HealthPage.tsx";
import { LogPage } from "./pages/LogPage.tsx";
import { LoginPage } from "./pages/LoginPage.tsx";
import { NetworkPage } from "./pages/NetworkPage.tsx";
import { useAuth, useAuthStore } from "./store/authStore.ts";

const BootScreen = twc.div`min-h-screen flex items-center justify-center text-sm text-gray-400`;

// Registers the global 401 handler and runs the boot-time session probe.
// Replaces the old AuthProvider effect pair — state now lives in
// useAuthStore (zustand) instead of a Context, but the two effects and their
// semantics (401 anywhere -> logged out; probe GET /api/config once on load
// before deciding whether to show the login screen) are unchanged.
function useSessionProbe() {
  const queryClient = useQueryClient();
  const setAuthenticated = useAuthStore((s) => s.setAuthenticated);
  const setAuthChecked = useAuthStore((s) => s.setAuthChecked);

  useEffect(() => {
    setUnauthorizedHandler(() => setAuthenticated(false));
    return () => setUnauthorizedHandler(null);
  }, [setAuthenticated]);

  useEffect(() => {
    let cancelled = false;
    void queryClient
      .fetchQuery({ queryKey: queryKeys.config, queryFn: getConfig })
      .then(() => {
        if (!cancelled) setAuthenticated(true);
      })
      .catch(() => {
        if (!cancelled) setAuthenticated(false);
      })
      .finally(() => {
        if (!cancelled) setAuthChecked(true);
      });
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
}

function ProtectedLayout() {
  const { isAuthenticated, authChecked } = useAuth();
  if (!authChecked) return <BootScreen>正在检查登录状态…</BootScreen>;
  if (!isAuthenticated) return <Navigate to="/login" replace />;
  return <Layout />;
}

function AppRoutes() {
  useSessionProbe();
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route element={<ProtectedLayout />}>
        <Route index element={<DashboardPage />} />
        <Route path="network" element={<NetworkPage />} />
        <Route path="dns" element={<DnsPage />} />
        <Route path="health" element={<HealthPage />} />
        <Route path="log" element={<LogPage />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Route>
    </Routes>
  );
}

export function App() {
  return (
    <BrowserRouter>
      <AppRoutes />
    </BrowserRouter>
  );
}
