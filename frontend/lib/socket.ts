"use client";

import { WSEvent, WSEventType } from "./types";
import { getAccessToken } from "./api";

// Check if WebSocket is enabled via environment variable
const isWebSocketEnabled = process.env.NEXT_PUBLIC_WEBSOCKET_ENABLED !== "false";

// ─── Socket.IO-like adapter over native WebSocket ────────────────────────────
// The backend uses plain WebSocket with JSON events: { type, payload, ts }
// This adapter provides a familiar .on() / .emit() / .connect() API.

type EventHandler = (payload: unknown) => void;

export interface SocketOptions {
    url?: string;
    autoConnect?: boolean;
    reconnect?: boolean;
    reconnectInterval?: number;
    maxReconnectInterval?: number;
    reconnectAttempts?: number;
    pingInterval?: number;
}

const DEFAULT_OPTIONS: Required<SocketOptions> = {
    url: process.env.NEXT_PUBLIC_WS_URL || "ws://localhost:8080/api/v1/ws",
    autoConnect: true,
    reconnect: true,
    reconnectInterval: 1000,
    maxReconnectInterval: 30000,
    reconnectAttempts: Infinity,
    pingInterval: 25000,
};

export class SocketClient {
    private ws: WebSocket | null = null;
    private options: Required<SocketOptions>;
    private listeners: Map<string, Set<EventHandler>> = new Map();
    private reconnectCount = 0;
    private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
    private pingTimer: ReturnType<typeof setInterval> | null = null;
    private _connected = false;
    private _destroyed = false;

    constructor(options?: SocketOptions) {
        // If WebSocket is disabled via environment, set autoConnect to false
        const wsEnabled = options?.autoConnect ?? isWebSocketEnabled;
        this.options = { ...DEFAULT_OPTIONS, ...options, autoConnect: wsEnabled && isWebSocketEnabled };
        if (this.options.autoConnect) {
            this.connect();
        }
    }

    get connected(): boolean {
        return this._connected;
    }

    connect(): void {
        // Skip connection if WebSocket is disabled via environment
        if (!isWebSocketEnabled) {
            console.log("WebSocket is disabled via NEXT_PUBLIC_WEBSOCKET_ENABLED");
            return;
        }
        if (this._destroyed) return;

        const token = getAccessToken();
        if (!token) {
            // Wait for token then retry
            setTimeout(() => this.connect(), 2000);
            return;
        }

        try {
            const url = `${this.options.url}?token=${encodeURIComponent(token)}`;
            this.ws = new WebSocket(url);

            this.ws.onopen = () => {
                this._connected = true;
                this.reconnectCount = 0;
                this.startPing();
                this.dispatch("connect", null);
            };

            this.ws.onmessage = (event) => {
                try {
                    const data: WSEvent = JSON.parse(event.data);
                    this.dispatch(data.type, data.payload);
                } catch {
                    // ignore malformed messages
                }
            };

            this.ws.onclose = () => {
                this._connected = false;
                this.stopPing();
                this.dispatch("disconnect", null);

                if (this.options.reconnect && !this._destroyed) {
                    this.scheduleReconnect();
                }
            };

            this.ws.onerror = () => {
                // onerror is always followed by onclose
                this.dispatch("error", null);
            };
        } catch {
            if (this.options.reconnect && !this._destroyed) {
                this.scheduleReconnect();
            }
        }
    }

    disconnect(): void {
        this._destroyed = true;
        this.stopPing();
        if (this.reconnectTimer) {
            clearTimeout(this.reconnectTimer);
            this.reconnectTimer = null;
        }
        if (this.ws) {
            this.ws.close();
            this.ws = null;
        }
        this._connected = false;
    }

    // Subscribe to events
    on(event: string, handler: EventHandler): () => void {
        if (!this.listeners.has(event)) {
            this.listeners.set(event, new Set());
        }
        this.listeners.get(event)!.add(handler);

        // Return unsubscribe function
        return () => {
            this.listeners.get(event)?.delete(handler);
        };
    }

    // Emit event to server
    emit(type: WSEventType, payload?: unknown): void {
        if (!this.ws || this.ws.readyState !== WebSocket.OPEN) return;

        const event: WSEvent = {
            type,
            payload,
            ts: new Date().toISOString(),
        };
        this.ws.send(JSON.stringify(event));
    }

    private dispatch(event: string, payload: unknown): void {
        const handlers = this.listeners.get(event);
        if (handlers) {
            handlers.forEach((handler) => {
                try {
                    handler(payload);
                } catch (err) {
                    console.error(`Socket event handler error for "${event}":`, err);
                }
            });
        }

        // Also dispatch to wildcard listeners
        const wildcardHandlers = this.listeners.get("*");
        if (wildcardHandlers) {
            wildcardHandlers.forEach((handler) => {
                try {
                    handler({ event, payload });
                } catch (err) {
                    console.error("Socket wildcard handler error:", err);
                }
            });
        }
    }

    private scheduleReconnect(): void {
        if (this.reconnectCount >= this.options.reconnectAttempts) {
            this.dispatch("reconnect_failed", null);
            return;
        }

        const delay = Math.min(
            this.options.reconnectInterval * Math.pow(2, this.reconnectCount),
            this.options.maxReconnectInterval
        );

        this.reconnectTimer = setTimeout(() => {
            this.reconnectCount++;
            this.dispatch("reconnecting", { attempt: this.reconnectCount });
            this.connect();
        }, delay);
    }

    private startPing(): void {
        this.stopPing();
        this.pingTimer = setInterval(() => {
            this.emit("ping");
        }, this.options.pingInterval);
    }

    private stopPing(): void {
        if (this.pingTimer) {
            clearInterval(this.pingTimer);
            this.pingTimer = null;
        }
    }
}

// Singleton instance
let socketInstance: SocketClient | null = null;

export function getSocket(options?: SocketOptions): SocketClient {
    if (!socketInstance) {
        socketInstance = new SocketClient({ ...options, autoConnect: false });
    }
    return socketInstance;
}

export function destroySocket(): void {
    if (socketInstance) {
        socketInstance.disconnect();
        socketInstance = null;
    }
}
