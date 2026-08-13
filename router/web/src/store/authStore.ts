// Client-only auth state — replaces the old React Context (AuthProvider in
// App.tsx). `isAuthenticated`/`authChecked` are driven by the boot-time
// session probe and the global 401 handler (see App.tsx's useSessionProbe).
import { create } from "zustand";

interface AuthStoreState {
  isAuthenticated: boolean;
  /** False until the initial session probe (GET /api/config) resolves. */
  authChecked: boolean;
  setAuthenticated: (v: boolean) => void;
  setAuthChecked: (v: boolean) => void;
}

export const useAuthStore = create<AuthStoreState>((set) => ({
  isAuthenticated: false,
  authChecked: false,
  setAuthenticated: (v) => set({ isAuthenticated: v }),
  setAuthChecked: (v) => set({ authChecked: v }),
}));

// Convenience hook matching the old `useAuth()` Context API, so call sites
// (LoginPage, App.tsx's ProtectedLayout) don't need to change their shape.
export function useAuth(): {
  isAuthenticated: boolean;
  authChecked: boolean;
  setAuth: (v: boolean) => void;
} {
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated);
  const authChecked = useAuthStore((s) => s.authChecked);
  const setAuth = useAuthStore((s) => s.setAuthenticated);
  return { isAuthenticated, authChecked, setAuth };
}
