// Small typed fetch client for the AgentBoard API.

let csrfToken: string | null = null;

export function setCSRF(token: string | null) {
  csrfToken = token;
}

async function handle<T>(res: Response): Promise<T> {
  if (!res.ok) {
    let msg = `HTTP ${res.status}`;
    try {
      const body = await res.json();
      if (body?.error?.message) msg = body.error.message;
    } catch {
      // ignore
    }
    const err = new Error(msg) as Error & { status?: number };
    err.status = res.status;
    throw err;
  }
  const body = await res.json();
  return body.data as T;
}

export async function apiGet<T>(path: string): Promise<T> {
  const res = await fetch(path, { credentials: "same-origin" });
  return handle<T>(res);
}

function mutate<T>(method: string, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = { "Content-Type": "application/json" };
  if (csrfToken) headers["X-CSRF-Token"] = csrfToken;
  return fetch(path, {
    method,
    credentials: "same-origin",
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
  }).then((r) => handle<T>(r));
}

export const apiPost = <T,>(path: string, body?: unknown) => mutate<T>("POST", path, body);
export const apiPatch = <T,>(path: string, body?: unknown) => mutate<T>("PATCH", path, body);
export const apiDelete = <T,>(path: string) => mutate<T>("DELETE", path);

export interface SessionInfo {
  authenticated: boolean;
  totp_enabled?: boolean;
  expires_at?: string;
  csrf_token?: string;
}

export async function fetchSession(): Promise<SessionInfo> {
  const info = await apiGet<SessionInfo>("/auth/session");
  if (info.csrf_token) setCSRF(info.csrf_token);
  return info;
}

export async function login(password: string, totp?: string): Promise<SessionInfo> {
  const info = await apiPost<SessionInfo>("/auth/login", { password, totp_code: totp ?? "" });
  if (info.csrf_token) setCSRF(info.csrf_token);
  return info;
}

export async function logout(): Promise<void> {
  await apiPost("/auth/logout", {});
  setCSRF(null);
}
