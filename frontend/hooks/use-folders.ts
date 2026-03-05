"use client";

import { useState, useEffect, useCallback } from "react";
import { apiGet, apiPost } from "@/lib/api";
import { FolderStats } from "@/lib/types";

export function useFolders() {
    const [folders, setFolders] = useState<FolderStats[]>([]);
    const [loading, setLoading] = useState(true);

    const fetchFolders = useCallback(async () => {
        const res = await apiGet<FolderStats[]>("/email/folders");
        if (res.success && res.data) {
            setFolders(res.data);
        }
        setLoading(false);
    }, []);

    useEffect(() => {
        const t = setTimeout(() => {
            fetchFolders();
        }, 0);
        return () => clearTimeout(t);
    }, [fetchFolders]);

    // Listen for optimistic unread updates
    useEffect(() => {
        const handleOptimisticUpdate = (e: Event) => {
            const customEvent = e as CustomEvent;
            if (customEvent.detail && typeof customEvent.detail.delta === "number") {
                setFolders((prev) =>
                    prev.map((f) => {
                        if (f.folder.toUpperCase() === "INBOX") {
                            return {
                                ...f,
                                unread_count: Math.max(0, f.unread_count + customEvent.detail.delta),
                            };
                        }
                        return f;
                    })
                );
            }
        };

        window.addEventListener("optimistic_unread_update", handleOptimisticUpdate);
        return () => window.removeEventListener("optimistic_unread_update", handleOptimisticUpdate);
    }, []);

    return { folders, loading, refetch: fetchFolders };
}

export function useUnreadCount() {
    const [count, setCount] = useState(0);

    const fetchCount = useCallback(async () => {
        const res = await apiGet<{ unread_count: number }>("/email/unread-count");
        if (res.success && res.data) {
            setCount(res.data.unread_count);
        }
    }, []);

    useEffect(() => {
        const t = setTimeout(() => {
            fetchCount();
        }, 0);
        const interval = setInterval(fetchCount, 30000); // Refresh every 30s
        return () => {
            clearTimeout(t);
            clearInterval(interval);
        };
    }, [fetchCount]);

    return { count, refetch: fetchCount };
}
