"use client";

import { useState, useCallback } from "react";
import { apiGet, apiPost } from "@/lib/api";
import {
    PaginatedThreads,
    Thread,
    ThreadListItem,
    MarkReadRequest,
    MarkStarredRequest,
    TrashRequest,
    MoveRequest,
} from "@/lib/types";

export function useThreads(folder: string = "INBOX") {
    const [threads, setThreads] = useState<ThreadListItem[]>([]);
    const [total, setTotal] = useState(0);
    const [page, setPage] = useState(1);
    const [totalPages, setTotalPages] = useState(1);
    const [hasNext, setHasNext] = useState(false);
    const [loading, setLoading] = useState(true);

    const fetchThreads = useCallback(
        async (p: number = 1) => {
            setLoading(true);
            const res = await apiGet<PaginatedThreads>(
                `/email/threads?folder=${encodeURIComponent(folder)}&page=${p}&per_page=50`
            );
            if (res.success && res.data) {
                setThreads(res.data.data || []);
                setTotal(res.data.total);
                setPage(res.data.page);
                setTotalPages(res.data.total_pages);
                setHasNext(res.data.has_next);
            }
            setLoading(false);
        },
        [folder]
    );

    const markRead = useCallback(
        async (uids: number[], read: boolean) => {
            const body: MarkReadRequest = { folder, uids, read };
            await apiPost("/email/messages/mark-read", body);
            // Optimistic update — only affect threads that contain these UIDs
            const uidSet = new Set(uids);

            let changedCount = 0;

            setThreads((prev) =>
                prev.map((t) => {
                    const hasUid = t.message_uids?.some((u) => uidSet.has(u));
                    if (!hasUid) return t;

                    const wasUnread = t.unread_count > 0;
                    if (read && wasUnread) changedCount++;
                    if (!read && !wasUnread) changedCount++;

                    return {
                        ...t,
                        unread_count: read ? 0 : t.unread_count > 0 ? t.unread_count : 1,
                    };
                })
            );

            // Dispatch optimistic event for Sidebar if in INBOX
            if (folder.toUpperCase() === "INBOX" && changedCount > 0) {
                const delta = read ? -changedCount : changedCount;
                window.dispatchEvent(new CustomEvent('optimistic_unread_update', { detail: { delta } }));
            }
        },
        [folder]
    );

    const markStarred = useCallback(
        async (uids: number[], starred: boolean) => {
            const body: MarkStarredRequest = { folder, uids, starred };
            await apiPost("/email/messages/mark-starred", body);
            // Optimistic update — only affect threads that contain these UIDs
            const uidSet = new Set(uids);
            setThreads((prev) =>
                prev.map((t) => {
                    const hasUid = t.message_uids?.some((u) => uidSet.has(u));
                    if (!hasUid) return t;
                    return { ...t, is_starred: starred };
                })
            );
        },
        [folder]
    );

    const moveToTrash = useCallback(
        async (uids: number[]) => {
            const body: TrashRequest = { folder, uids };
            // Optimistic update — remove threads containing these UIDs
            const uidSet = new Set(uids);
            setThreads((prev) => prev.filter((t) => !t.message_uids?.some((u) => uidSet.has(u))));

            await apiPost("/email/messages/trash", body);
            // Optionally, we can omit fetchThreads(page) if the optimistic update is reliable enough,
            // or keep it to sync the state with backend eventually. We will keep it but it will fetch in background.
            fetchThreads(page);
        },
        [folder, fetchThreads, page]
    );

    const moveMessages = useCallback(
        async (uids: number[], destination: string) => {
            const body: MoveRequest = { src_folder: folder, dst_folder: destination, uids };
            // Optimistic update — remove threads containing these UIDs
            const uidSet = new Set(uids);
            setThreads((prev) => prev.filter((t) => !t.message_uids?.some((u) => uidSet.has(u))));

            await apiPost("/email/messages/move", body);
            fetchThreads(page);
        },
        [folder, fetchThreads, page]
    );

    return {
        threads,
        total,
        page,
        totalPages,
        hasNext,
        loading,
        fetchThreads,
        setPage,
        markRead,
        markStarred,
        moveToTrash,
        moveMessages,
    };
}

export function useThread(threadId: string, folder: string = "INBOX") {
    const [thread, setThread] = useState<Thread | null>(null);
    const [loading, setLoading] = useState(true);

    const fetchThread = useCallback(async () => {
        setLoading(true);
        const res = await apiGet<Thread>(
            `/email/threads/${encodeURIComponent(threadId)}?folder=${encodeURIComponent(folder)}`
        );
        if (res.success && res.data) {
            setThread(res.data);
        }
        setLoading(false);
    }, [threadId, folder]);

    return { thread, loading, fetchThread };
}
