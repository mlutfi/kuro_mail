import { APIResponse, RefreshTokenRequest, TokenPair } from "./types";

const API_URL = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080/api/v1";

export { API_URL };

// ─── Token Storage ───────────────────────────────────────────────────────────

export function getAccessToken(): string | null {
    if (typeof window === "undefined") return null;
    return localStorage.getItem("access_token");
}

export function getRefreshToken(): string | null {
    if (typeof window === "undefined") return null;
    return localStorage.getItem("refresh_token");
}

export function setTokens(accessToken: string, refreshToken: string) {
    localStorage.setItem("access_token", accessToken);
    localStorage.setItem("refresh_token", refreshToken);
}

export function clearTokens() {
    localStorage.removeItem("access_token");
    localStorage.removeItem("refresh_token");
}

// ─── Refresh Logic ───────────────────────────────────────────────────────────

let isRefreshing = false;
let refreshPromise: Promise<string | null> | null = null;

async function refreshAccessToken(): Promise<string | null> {
    const refreshToken = getRefreshToken();
    if (!refreshToken) return null;

    if (isRefreshing && refreshPromise) {
        return refreshPromise;
    }

    isRefreshing = true;
    refreshPromise = (async () => {
        try {
            const body: RefreshTokenRequest = { refresh_token: refreshToken };
            const res = await fetch(`${API_URL}/auth/refresh`, {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify(body),
            });

            // Only clear tokens when the server definitively rejects the refresh token (401/403)
            // Do NOT clear on 5xx server errors or network issues
            if (!res.ok) {
                if (res.status === 401 || res.status === 403) {
                    clearTokens();
                }
                return null;
            }

            const json: APIResponse<TokenPair> = await res.json();
            if (json.success && json.data) {
                setTokens(json.data.access_token, json.data.refresh_token);
                return json.data.access_token;
            }
            // success=false from server means token is invalid
            clearTokens();
            return null;
        } catch {
            // Network error — do NOT clear tokens, user might still be logged in
            return null;
        } finally {
            isRefreshing = false;
            refreshPromise = null;
        }
    })();

    return refreshPromise;
}

// ─── API Fetch Wrapper ───────────────────────────────────────────────────────

interface FetchOptions extends Omit<RequestInit, "body"> {
    body?: unknown;
    skipAuth?: boolean;
}

export async function api<T = unknown>(
    endpoint: string,
    options: FetchOptions = {}
): Promise<APIResponse<T>> {
    const { body, skipAuth = false, headers: customHeaders, ...rest } = options;

    const headers: Record<string, string> = {
        "Content-Type": "application/json",
        ...(customHeaders as Record<string, string>),
    };

    if (!skipAuth) {
        const token = getAccessToken();
        if (token) {
            headers["Authorization"] = `Bearer ${token}`;
        }
    }

    const url = endpoint.startsWith("http") ? endpoint : `${API_URL}${endpoint}`;

    let res = await fetch(url, {
        ...rest,
        headers,
        body: body ? JSON.stringify(body) : undefined,
    });

    // Auto-refresh on 401
    if (res.status === 401 && !skipAuth) {
        const newToken = await refreshAccessToken();
        if (newToken) {
            headers["Authorization"] = `Bearer ${newToken}`;
            res = await fetch(url, {
                ...rest,
                headers,
                body: body ? JSON.stringify(body) : undefined,
            });
        } else {
            // Return error — let auth-context handle redirect via useRequireAuth
            return { success: false, error: "Session expired" };
        }
    }

    const json: APIResponse<T> = await res.json();
    return json;
}

// ─── Convenience methods ─────────────────────────────────────────────────────

export const apiGet = <T>(endpoint: string, options?: FetchOptions) =>
    api<T>(endpoint, { ...options, method: "GET" });

export const apiPost = <T>(endpoint: string, body?: unknown, options?: FetchOptions) =>
    api<T>(endpoint, { ...options, method: "POST", body });

export const apiPut = <T>(endpoint: string, body?: unknown, options?: FetchOptions) =>
    api<T>(endpoint, { ...options, method: "PUT", body });

export const apiDelete = <T>(endpoint: string, options?: FetchOptions) =>
    api<T>(endpoint, { ...options, method: "DELETE" });
