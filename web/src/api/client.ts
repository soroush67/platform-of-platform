import { ApiError, type LoginResponse, type ProblemDetails } from "./types";

// currentAccessToken is a plain module-level variable, not read through
// React context - apiFetch is called from TanStack Query hooks, which
// don't have component-tree access to context, and threading the token
// through every single hook call would be far noisier than this one
// small escape hatch. AuthContext is the only writer (setAccessToken).
let currentAccessToken: string | null = null;
let currentRefreshToken: string | null = null;

// onAuthFailure is set once by AuthProvider - called when a refresh
// attempt itself fails, so AuthContext can clear its own React state in
// sync with this module's tokens (apiFetch can't reach into React state
// directly, same reasoning as above).
let onAuthFailure: (() => void) | null = null;
let onTokensRotated: ((access: string, refresh: string) => void) | null = null;

export function setAccessToken(token: string | null) {
  currentAccessToken = token;
}

export function setRefreshToken(token: string | null) {
  currentRefreshToken = token;
}

export function setAuthCallbacks(handlers: {
  onAuthFailure: () => void;
  onTokensRotated: (access: string, refresh: string) => void;
}) {
  onAuthFailure = handlers.onAuthFailure;
  onTokensRotated = handlers.onTokensRotated;
}

async function parseProblem(res: Response): Promise<ProblemDetails> {
  try {
    const body = await res.json();
    return {
      type: body.type ?? "about:blank",
      title: body.title ?? res.statusText,
      status: res.status,
      detail: body.detail,
      request_id: body.request_id,
    };
  } catch {
    return { type: "about:blank", title: res.statusText || "request failed", status: res.status };
  }
}

async function rawFetch(path: string, init?: RequestInit): Promise<Response> {
  const headers = new Headers(init?.headers);
  if (currentAccessToken) {
    headers.set("Authorization", `Bearer ${currentAccessToken}`);
  }
  if (init?.body && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }
  return fetch(`/api/v1${path}`, { ...init, headers });
}

// attemptRefresh makes exactly one POST /auth/refresh call using the
// persisted refresh token, rotating it (the server single-uses every
// refresh token) - callers must not retry this themselves.
async function attemptRefresh(): Promise<boolean> {
  if (!currentRefreshToken) return false;
  const res = await fetch("/api/v1/auth/refresh", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ refresh_token: currentRefreshToken }),
  });
  if (!res.ok) return false;
  const body: LoginResponse = await res.json();
  currentAccessToken = body.access_token;
  currentRefreshToken = body.refresh_token;
  onTokensRotated?.(body.access_token, body.refresh_token);
  return true;
}

// apiFetchRaw is the SSE-consumption escape hatch (useFleet.ts's own
// useOperationLogStream) - returns the raw Response so callers can read
// its body as a ReadableStream, unlike apiFetch which always awaits and
// JSON-parses the full body. Deliberately does NOT retry on 401 (a
// streaming GET's own reconnect-with-a-fresh-token story is out of
// scope for Phase 1 - the token used to open the stream is assumed to
// outlive it, true for every real deploy operation's realistic runtime).
export async function apiFetchRaw(path: string, init?: RequestInit): Promise<Response> {
  return rawFetch(path, init);
}

export async function apiFetch<T>(path: string, init?: RequestInit): Promise<T> {
  let res = await rawFetch(path, init);

  if (res.status === 401 && currentRefreshToken) {
    const refreshed = await attemptRefresh();
    if (refreshed) {
      res = await rawFetch(path, init);
    } else {
      onAuthFailure?.();
      throw new ApiError({ type: "about:blank", title: "session expired", status: 401 });
    }
  }

  if (res.status === 401 && !currentRefreshToken) {
    onAuthFailure?.();
    throw new ApiError({ type: "about:blank", title: "authentication required", status: 401 });
  }

  if (!res.ok) {
    throw new ApiError(await parseProblem(res));
  }

  if (res.status === 204 || res.status === 202) {
    return undefined as T;
  }

  const text = await res.text();
  return text ? (JSON.parse(text) as T) : (undefined as T);
}
