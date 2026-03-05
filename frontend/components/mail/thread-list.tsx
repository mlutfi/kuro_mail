"use client";

import { useState, useEffect } from "react";
import { useThreads } from "@/hooks/use-threads";
import { useWebSocket } from "@/hooks/use-websocket";
import ThreadListItemComponent from "./thread-list-item";
import { PullToRefresh } from "@/components/ui/pull-to-refresh";
import { Button } from "@/components/ui/button";
import {
    RefreshCw,
    ChevronLeft,
    ChevronRight,
    Trash2,
    Mail,
    MailOpen,
    Archive,
    Loader2,
    Inbox,
    Send,
    FileText,
    Star,
    AlertTriangle,
} from "lucide-react";
import { Checkbox } from "@/components/ui/checkbox";
import { toast } from "sonner";
import React from "react";
import { ScrollArea } from "@/components/ui/scroll-area";

interface ThreadListProps {
    folder: string;
}

const FOLDER_ICONS: Record<string, React.ElementType> = {
    INBOX: Inbox,
    SENT: Send,
    DRAFTS: FileText,
    STARRED: Star,
    ARCHIVE: Archive,
    JUNK: AlertTriangle,
    TRASH: Trash2,
};

export default function ThreadList({ folder }: ThreadListProps) {
    const {
        threads,
        total,
        page,
        hasNext,
        loading,
        fetchThreads,
        markRead,
        markStarred,
        moveToTrash,
        moveMessages,
    } = useThreads(folder);

    const { on } = useWebSocket();
    const [selectedThreads, setSelectedThreads] = useState<Set<string>>(new Set());

    // Fetch on mount and folder change
    useEffect(() => {
        fetchThreads(1);
    }, [fetchThreads]);

    // Listen for real-time updates
    useEffect(() => {
        const unsub = on("inbox_update", () => {
            fetchThreads(page);
        });
        const unsub2 = on("new_email", () => {
            if (folder.toUpperCase() === "INBOX") {
                fetchThreads(page);
            }
        });
        return () => {
            unsub();
            unsub2();
        };
    }, [on, fetchThreads, page, folder]);

    const handleSelectAll = (checked: boolean) => {
        if (checked) {
            setSelectedThreads(new Set(threads.map((t) => t.thread_id)));
        } else {
            setSelectedThreads(new Set());
        }
    };

    const handleSelect = (threadId: string, checked: boolean) => {
        setSelectedThreads((prev) => {
            const next = new Set(prev);
            if (checked) next.add(threadId);
            else next.delete(threadId);
            return next;
        });
    };

    const handleStarToggle = async (threadId: string) => {
        const thread = threads.find((t) => t.thread_id === threadId);
        if (!thread) return;
        const newStarred = !thread.is_starred;
        await markStarred(thread.message_uids || [], newStarred);
        toast.info(newStarred ? "Starred" : "Unstarred");
    };

    const handleBulkRead = async (read: boolean) => {
        if (selectedThreads.size === 0) return;
        const uids = threads
            .filter((t) => selectedThreads.has(t.thread_id))
            .flatMap((t) => t.message_uids || []);
        if (uids.length > 0) await markRead(uids, read);
        toast.success(read ? "Marked as read" : "Marked as unread");
        setSelectedThreads(new Set());
    };

    const handleBulkTrash = async () => {
        if (selectedThreads.size === 0) return;
        const uids = threads
            .filter((t) => selectedThreads.has(t.thread_id))
            .flatMap((t) => t.message_uids || []);
        if (uids.length > 0) await moveToTrash(uids);

        if (folder.toUpperCase() === "TRASH") {
            toast.success("Deleted permanently");
        } else {
            toast.success("Moved to trash");
        }

        setSelectedThreads(new Set());
    };

    const handleBulkArchive = async () => {
        if (selectedThreads.size === 0) return;
        const uids = threads
            .filter((t) => selectedThreads.has(t.thread_id))
            .flatMap((t) => t.message_uids || []);
        if (uids.length > 0) await moveMessages(uids, "Archive");
        toast.success("Archived");
        setSelectedThreads(new Set());
    };

    const handleBulkMoveToInbox = async () => {
        if (selectedThreads.size === 0) return;
        const uids = threads
            .filter((t) => selectedThreads.has(t.thread_id))
            .flatMap((t) => t.message_uids || []);
        if (uids.length > 0) await moveMessages(uids, "INBOX");
        toast.success("Moved to Inbox");
        setSelectedThreads(new Set());
    };

    const handlePageChange = (newPage: number) => {
        fetchThreads(newPage);
        setSelectedThreads(new Set());
    };

    const allSelected = threads.length > 0 && selectedThreads.size === threads.length;
    const someSelected = selectedThreads.size > 0;

    const Icon = FOLDER_ICONS[folder.toUpperCase()] || Inbox;
    const folderName = folder.toUpperCase() === "JUNK" ? "Spam" : folder.charAt(0).toUpperCase() + folder.slice(1).toLowerCase();

    return (
        <div className="flex flex-col h-full">
            {/* Toolbar */}
            <div className="flex items-center gap-1.5 px-3 py-2 border-b border-gray-100 dark:border-gray-800 bg-white dark:bg-gray-900 sticky top-0 z-10">
                <div className="flex items-center gap-2 mr-1">
                    <Checkbox
                        checked={allSelected}
                        onCheckedChange={handleSelectAll}
                        className="border-gray-300 data-[state=checked]:bg-blue-600 data-[state=checked]:border-blue-600"
                    />
                </div>

                {someSelected ? (
                    <div className="flex items-center gap-0.5">
                        <Button
                            variant="ghost"
                            size="sm"
                            className="h-8 px-2 text-gray-600 rounded-[7px] text-[13px]"
                            onClick={() => handleBulkRead(true)}
                        >
                            <MailOpen className="w-4 h-4 mr-1" />
                            <span className="hidden sm:inline">Read</span>
                        </Button>
                        <Button
                            variant="ghost"
                            size="sm"
                            className="h-8 px-2 text-gray-600 rounded-[7px] text-[13px]"
                            onClick={() => handleBulkRead(false)}
                        >
                            <Mail className="w-4 h-4 mr-1" />
                            <span className="hidden sm:inline">Unread</span>
                        </Button>

                        {(folder.toUpperCase() === "ARCHIVE" || folder.toUpperCase() === "TRASH") ? (
                            <Button
                                variant="ghost"
                                size="sm"
                                className="h-8 px-2 text-gray-600 rounded-[7px] text-[13px]"
                                onClick={handleBulkMoveToInbox}
                            >
                                <Inbox className="w-4 h-4 mr-1" />
                                <span className="hidden sm:inline">Move to Inbox</span>
                            </Button>
                        ) : (
                            <Button
                                variant="ghost"
                                size="sm"
                                className="h-8 px-2 text-gray-600 rounded-[7px] text-[13px]"
                                onClick={handleBulkArchive}
                            >
                                <Archive className="w-4 h-4 mr-1" />
                                <span className="hidden sm:inline">Archive</span>
                            </Button>
                        )}

                        <Button
                            variant="ghost"
                            size="sm"
                            className="h-8 px-2 text-red-500 hover:text-red-600 hover:bg-red-50 rounded-[7px] text-[13px]"
                            onClick={handleBulkTrash}
                        >
                            <Trash2 className="w-4 h-4 mr-1" />
                            <span className="hidden sm:inline">
                                {folder.toUpperCase() === "TRASH" ? "Delete Forever" : "Delete"}
                            </span>
                        </Button>
                    </div>
                ) : (
                    <>
                        <Button
                            variant="ghost"
                            size="sm"
                            className="h-8 w-8 p-0 text-gray-500 rounded-[7px] cursor-pointer"
                            onClick={() => fetchThreads(page)}
                        >
                            <RefreshCw className={`w-4 h-4 ${loading ? "animate-spin" : ""}`} />
                        </Button>

                        {/* Mobile Title */}
                        <div className="flex items-center gap-2 lg:hidden ml-1">
                            <Icon className="w-4 h-4 text-gray-500" />
                            <span className="font-semibold text-[15px] text-gray-800 tracking-tight">{folderName}</span>
                        </div>
                    </>
                )}

                {/* Spacer */}
                <div className="flex-1" />

                {/* Pagination */}
                <div className="flex items-center gap-1 text-[11px] text-gray-400 tabular-nums">
                    {total > 0 && (
                        <span>
                            {(page - 1) * 50 + 1}–{Math.min(page * 50, total)} of {total}
                        </span>
                    )}
                    <Button
                        variant="ghost"
                        size="sm"
                        className="h-7 w-7 p-0 rounded-[7px]"
                        onClick={() => handlePageChange(page - 1)}
                        disabled={page <= 1}
                    >
                        <ChevronLeft className="w-4 h-4" />
                    </Button>
                    <Button
                        variant="ghost"
                        size="sm"
                        className="h-7 w-7 p-0 rounded-[7px]"
                        onClick={() => handlePageChange(page + 1)}
                        disabled={!hasNext}
                    >
                        <ChevronRight className="w-4 h-4" />
                    </Button>
                </div>
            </div>

            {/* Thread items */}
            <ScrollArea className="flex-1 w-full h-full">
                <PullToRefresh onRefresh={async () => fetchThreads(page)}>
                    {loading && threads.length === 0 ? (
                        <div className="flex items-center justify-center h-64">
                            <div className="flex flex-col items-center gap-3 text-gray-400">
                                <Loader2 className="w-7 h-7 animate-spin text-blue-500" />
                                <span className="text-sm">Loading emails...</span>
                            </div>
                        </div>
                    ) : threads.length === 0 ? (
                        <div className="flex items-center justify-center h-64">
                            <div className="flex flex-col items-center gap-3">
                                <div className="w-16 h-16 rounded-full bg-gray-100 flex items-center justify-center">
                                    <Inbox className="w-8 h-8 text-gray-300" />
                                </div>
                                <span className="text-base font-medium text-gray-400">No emails</span>
                                <span className="text-sm text-gray-400">This folder is empty</span>
                            </div>
                        </div>
                    ) : (
                        threads.map((thread) => (
                            <ThreadListItemComponent
                                key={thread.thread_id}
                                thread={thread}
                                isSelected={selectedThreads.has(thread.thread_id)}
                                onSelect={handleSelect}
                                onStarToggle={handleStarToggle}
                                folder={folder}
                            />
                        ))
                    )}
                </PullToRefresh>
            </ScrollArea>
        </div>
    );
}
