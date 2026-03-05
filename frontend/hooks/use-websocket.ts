"use client";

import { useEffect, useRef, useCallback, useState } from "react";
import { SocketClient, getSocket } from "@/lib/socket";
import { WSEventType, WSNewEmailPayload } from "@/lib/types";
import { useAuth } from "@/lib/auth-context";
import { toast } from "sonner";

// Check if WebSocket is enabled via environment variable
const isWebSocketEnabled = typeof window !== "undefined"
    ? process.env.NEXT_PUBLIC_WEBSOCKET_ENABLED !== "false"
    : true;

export function useWebSocket() {
    const { isAuthenticated } = useAuth();
    const socketRef = useRef<SocketClient | null>(null);
    const [connected, setConnected] = useState(false);

    useEffect(() => {
        // Skip if WebSocket is disabled
        if (!isWebSocketEnabled) {
            console.log("WebSocket is disabled via NEXT_PUBLIC_WEBSOCKET_ENABLED");
            return;
        }

        if (!isAuthenticated) return;

        const socket = getSocket();
        socketRef.current = socket;

        const unsubConnect = socket.on("connect", () => {
            setConnected(true);
        });

        const unsubDisconnect = socket.on("disconnect", () => {
            setConnected(false);
        });

        const unsubNewEmail = socket.on("new_email", (payload) => {
            const data = payload as WSNewEmailPayload;
            toast.info(`New email from ${data.from}`, {
                description: data.subject,
            });
        });

        // Connect if not already
        if (!socket.connected) {
            socket.connect();
        }

        return () => {
            unsubConnect();
            unsubDisconnect();
            unsubNewEmail();
        };
    }, [isAuthenticated]);

    const on = useCallback((event: string, handler: (payload: unknown) => void) => {
        return socketRef.current?.on(event, handler) || (() => { });
    }, []);

    const emit = useCallback((type: WSEventType, payload?: unknown) => {
        socketRef.current?.emit(type, payload);
    }, []);

    return { connected, on, emit };
}
