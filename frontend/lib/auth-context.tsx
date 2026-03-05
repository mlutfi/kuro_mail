"use client";

import React, { createContext, useContext, useState, useEffect, useCallback, useRef } from "react";
import { useRouter } from "next/navigation";
import { api, setTokens, clearTokens, getAccessToken, getRefreshToken } from "./api";
import { LoginResponse, UserProfile, TwoFASetupResponse } from "./types";

// ─── Context Types ───────────────────────────────────────────────────────────

interface AuthState {
    user: UserProfile | null;
    isAuthenticated: boolean;
    isLoading: boolean;
    login: (email: string, password: string) => Promise<{ requiresTwoFA: boolean; tempToken?: string }>;
    verify2FA: (tempToken: string, code: string, trustDevice?: boolean) => Promise<void>;
    logout: () => Promise<void>;
    refreshUser: () => Promise<void>;
    setup2FA: () => Promise<TwoFASetupResponse | null>;
    enable2FA: (code: string) => Promise<string[] | null>;
    disable2FA: (code: string) => Promise<boolean>;
}

const AuthContext = createContext<AuthState | null>(null);

// ─── Device ID ───────────────────────────────────────────────────────────────

function generateUUID(): string {
    if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
        return crypto.randomUUID();
    }
    return "xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx".replace(/[xy]/g, (c) => {
        const r = (Math.random() * 16) | 0;
        const v = c === "x" ? r : (r & 0x3) | 0x8;
        return v.toString(16);
    });
}

function getDeviceId(): string {
    if (typeof window === "undefined") return "server";
    let id = localStorage.getItem("device_id");
    if (!id) {
        id = generateUUID();
        localStorage.setItem("device_id", id);
    }
    return id;
}

function getDeviceName(): string {
    if (typeof window === "undefined") return "Server";
    return navigator.userAgent.slice(0, 80);
}

// ─── Provider ────────────────────────────────────────────────────────────────

export function AuthProvider({ children }: { children: React.ReactNode }) {
    const [user, setUser] = useState<UserProfile | null>(null);
    const [isLoading, setIsLoading] = useState(true);
    const initialized = useRef(false);
    const retryTimeout = useRef<ReturnType<typeof setTimeout> | null>(null);
    const mountedRef = useRef(true);

    const isAuthenticated = !!user;

    // Fetch user profile — determines auth status definitively.
    // Uses a plain function declaration (hoisted) so it can self-reference for retries.
    // Only uses stable refs and state setters, so no stale closure concerns.
    async function fetchUser(): Promise<void> {
        try {
            const res = await api<UserProfile>("/auth/me");

            if (!mountedRef.current) return;

            if (res.success && res.data) {
                setUser(res.data);
                setIsLoading(false);
            } else {
                // Check if tokens were cleared by the api layer (definitive auth failure)
                const hasTokens = !!getAccessToken() || !!getRefreshToken();
                if (!hasTokens) {
                    setUser(null);
                    setIsLoading(false);
                } else {
                    // Transient error — tokens still valid, retry
                    retryTimeout.current = setTimeout(() => {
                        if (mountedRef.current) void fetchUser();
                    }, 5000);
                }
            }
        } catch {
            if (!mountedRef.current) return;
            retryTimeout.current = setTimeout(() => {
                if (mountedRef.current) void fetchUser();
            }, 5000);
        }
    }

    // On mount: check for existing token and fetch user.
    useEffect(() => {
        if (initialized.current) return;
        initialized.current = true;
        mountedRef.current = true;

        const token = getAccessToken();
        if (token) {
            void fetchUser();
        } else {
            // No token at all — definitely not authenticated, stop loading immediately.
            // This is not a cascading render: it's the definitive initial state.
            queueMicrotask(() => setIsLoading(false));
        }

        return () => {
            mountedRef.current = false;
            if (retryTimeout.current) clearTimeout(retryTimeout.current);
        };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);

    const login = useCallback(async (email: string, password: string) => {
        const res = await api<LoginResponse>("/auth/login", {
            method: "POST",
            body: {
                email,
                password,
                device_id: getDeviceId(),
                device_name: getDeviceName(),
            },
            skipAuth: true,
        });

        if (!res.success) {
            throw new Error(res.error || "Login failed");
        }

        const data = res.data!;

        if (data.requires_two_fa) {
            return { requiresTwoFA: true, tempToken: data.temp_token };
        }

        // Direct login (no 2FA)
        setTokens(data.access_token!, data.refresh_token!);
        setUser(data.user!);
        return { requiresTwoFA: false };
    }, []);

    const verify2FA = useCallback(async (tempToken: string, code: string, trustDevice = false) => {
        const res = await api<LoginResponse>("/auth/login/2fa", {
            method: "POST",
            body: { temp_token: tempToken, code, trust_device: trustDevice },
            skipAuth: true,
        });

        if (!res.success) {
            throw new Error(res.error || "2FA verification failed");
        }

        const data = res.data!;
        setTokens(data.access_token!, data.refresh_token!);
        setUser(data.user!);
    }, []);

    const logout = useCallback(async () => {
        try {
            await api("/auth/logout", { method: "POST" });
        } catch {
            // ignore
        }
        clearTokens();
        setUser(null);
    }, []);

    const refreshUser = useCallback(async () => {
        await fetchUser();
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);

    const setup2FA = useCallback(async () => {
        const res = await api<TwoFASetupResponse>("/auth/2fa/setup", { method: "POST" });
        if (res.success && res.data) return res.data;
        return null;
    }, []);

    const enable2FA = useCallback(async (code: string) => {
        const res = await api<{ backup_codes: string[] }>("/auth/2fa/enable", {
            method: "POST",
            body: { code },
        });
        if (res.success && res.data) {
            await fetchUser();
            return res.data.backup_codes;
        }
        return null;
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);

    const disable2FA = useCallback(async (code: string) => {
        const res = await api("/auth/2fa/disable", {
            method: "POST",
            body: { code },
        });
        if (res.success) {
            await fetchUser();
            return true;
        }
        return false;
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);

    return (
        <AuthContext.Provider
            value={{
                user,
                isAuthenticated,
                isLoading,
                login,
                verify2FA,
                logout,
                refreshUser,
                setup2FA,
                enable2FA,
                disable2FA,
            }}
        >
            {children}
        </AuthContext.Provider>
    );
}

// ─── Hook ────────────────────────────────────────────────────────────────────

export function useAuth() {
    const ctx = useContext(AuthContext);
    if (!ctx) throw new Error("useAuth must be used within AuthProvider");
    return ctx;
}

// ─── Auth Guard (for protected pages) ────────────────────────────────────────

export function useRequireAuth() {
    const auth = useAuth();
    const router = useRouter();

    useEffect(() => {
        if (!auth.isLoading && !auth.isAuthenticated) {
            // Only redirect when tokens are absent (definitively cleared by server rejection).
            if (!getAccessToken() && !getRefreshToken()) {
                router.replace("/login");
            }
        }
    }, [auth.isLoading, auth.isAuthenticated, router]);

    return auth;
}
