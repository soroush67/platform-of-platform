import { createContext, useContext, useEffect, useState, type ReactNode } from "react";
import { jwtDecode } from "jwt-decode";

import { apiFetch, setAccessToken, setAuthCallbacks, setRefreshToken } from "../api/client";
import type { LoginResponse, User } from "../api/types";

const ACCESS_TOKEN_KEY = "pop_access_token";
const REFRESH_TOKEN_KEY = "pop_refresh_token";

type AuthStatus = "loading" | "authenticated" | "unauthenticated";

interface AuthContextValue {
  status: AuthStatus;
  user: User | null;
  login: (username: string, password: string) => Promise<void>;
  logout: () => void;
}

const AuthContext = createContext<AuthContextValue | null>(null);

function persistTokens(access: string, refresh: string) {
  localStorage.setItem(ACCESS_TOKEN_KEY, access);
  localStorage.setItem(REFRESH_TOKEN_KEY, refresh);
  setAccessToken(access);
  setRefreshToken(refresh);
}

function clearTokens() {
  localStorage.removeItem(ACCESS_TOKEN_KEY);
  localStorage.removeItem(REFRESH_TOKEN_KEY);
  setAccessToken(null);
  setRefreshToken(null);
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const [status, setStatus] = useState<AuthStatus>("loading");
  const [user, setUser] = useState<User | null>(null);

  useEffect(() => {
    setAuthCallbacks({
      onAuthFailure: () => {
        clearTokens();
        setUser(null);
        setStatus("unauthenticated");
      },
      onTokensRotated: (access, refresh) => {
        localStorage.setItem(ACCESS_TOKEN_KEY, access);
        localStorage.setItem(REFRESH_TOKEN_KEY, refresh);
      },
    });

    const access = localStorage.getItem(ACCESS_TOKEN_KEY);
    const refresh = localStorage.getItem(REFRESH_TOKEN_KEY);
    if (!access || !refresh) {
      setStatus("unauthenticated");
      return;
    }
    setAccessToken(access);
    setRefreshToken(refresh);

    // Decode `sub` for an immediate id (the JWT carries nothing else -
    // no username/email), then fetch the real profile. If the access
    // token has already expired, apiFetch's own 401-refresh-retry
    // handles it transparently; if the refresh token is also dead,
    // onAuthFailure above fires and this falls through to
    // unauthenticated.
    try {
      jwtDecode(access);
    } catch {
      clearTokens();
      setStatus("unauthenticated");
      return;
    }

    apiFetch<User>("/users/me")
      .then((u) => {
        setUser(u);
        setStatus("authenticated");
      })
      .catch(() => {
        setStatus("unauthenticated");
      });
  }, []);

  async function login(username: string, password: string) {
    const body: LoginResponse = await apiFetch("/auth/login", {
      method: "POST",
      body: JSON.stringify({ username, password }),
    });
    persistTokens(body.access_token, body.refresh_token);
    const u = await apiFetch<User>("/users/me");
    setUser(u);
    setStatus("authenticated");
  }

  function logout() {
    // Purely client-side - no server-side revoke endpoint exists for
    // either token type (deliberate, documented gap, not an oversight).
    clearTokens();
    setUser(null);
    setStatus("unauthenticated");
  }

  return <AuthContext.Provider value={{ status, user, login, logout }}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used within AuthProvider");
  return ctx;
}
