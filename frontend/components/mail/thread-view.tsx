"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import { useThread } from "@/hooks/use-threads";
import MessageItem from "./message-item";
import ComposeDialog from "./compose-dialog";
import { Button } from "@/components/ui/button";
import {
    ArrowLeft,
    Archive,
    MailOpen,
    Mail,
    Loader2,
    Trash2,
    Star,
    Inbox,
} from "lucide-react";
import { EmailMessage, EmailAddress } from "@/lib/types";
import { cn } from "@/lib/utils";
import { apiPost } from "@/lib/api";
import { format } from "date-fns";
import { toast } from "sonner";
import { ScrollArea } from "@/components/ui/scroll-area";

interface ThreadViewProps {
    threadId: string;
    folder?: string;
    onUnreadUpdate?: () => void;
}

// ─── Types ────────────────────────────────────────────────────────────────────

interface ReplyData {
    subject: string;
    to: EmailAddress[];
    cc?: EmailAddress[];
    in_reply_to?: string;
    references?: string;
    body_text?: string;
    body_html?: string;
}

interface ForwardData {
    subject: string;
    body_html: string;
    body_text: string;
}

// ─── Component ───────────────────────────────────────────────────────────────

export default function ThreadView({ threadId, folder = "INBOX", onUnreadUpdate }: ThreadViewProps) {
    const router = useRouter();
    const { thread, loading, fetchThread } = useThread(threadId, folder);

    const [composeOpen, setComposeOpen] = useState(false);
    const [replyData, setReplyData] = useState<ReplyData | null>(null);
    const [forwardData, setForwardData] = useState<ForwardData | null>(null);

    // Fetch thread on mount.
    useEffect(() => {
        fetchThread();
    }, [fetchThread]);

    // Mark all unread messages as read once the thread loads.
    useEffect(() => {
        if (!thread) return;
        const unreadUids = thread.messages
            .filter((m) => !m.is_read)
            .map((m) => m.imap_uid)
            .filter((uid): uid is number => uid != null && uid > 0);
        if (unreadUids.length > 0) {
            console.log("[ThreadView] Marking as read:", { folder, uids: unreadUids });
            apiPost("/email/messages/mark-read", { folder, uids: unreadUids, read: true }).catch(err => {
                console.error("[ThreadView] Failed to mark read:", err);
            });
        }
    }, [thread, folder]);

    // ─── Helpers ─────────────────────────────────────────────────────────────

    const getAllUIDs = () =>
        thread?.messages
            .map((m) => m.imap_uid)
            .filter((uid): uid is number => uid != null) ?? [];

    /**
     * Builds a quoted-reply text block, preferring body_text but falling back
     * to stripping HTML tags from body_html when only HTML is available.
     */
    const buildQuotedReply = (message: EmailMessage): string => {
        const date = format(new Date(message.received_at), "EEE, MMM d, yyyy 'at' h:mm a");
        const from = message.from.name
            ? `${message.from.name} <${message.from.email}>`
            : message.from.email;

        let original = message.body_text || "";
        if (!original && message.body_html) {
            // Strip HTML tags for a plain-text quote fallback.
            original = message.body_html.replace(/<[^>]+>/g, " ").replace(/\s+/g, " ").trim();
        }
        if (!original) {
            original = message.body_preview || "";
        }

        const quoted = original
            .split("\n")
            .map((line) => `> ${line}`)
            .join("\n");

        return `\n\nOn ${date}, ${from} wrote:\n${quoted}`;
    };

    /**
     * Builds the In-Reply-To and References headers for a reply.
     * Per RFC 5322: References = original References + original Message-ID.
     */
    const buildReplyHeaders = (message: EmailMessage) => ({
        in_reply_to: message.message_id,
        references: [...(message.references ?? []), message.message_id]
            .filter(Boolean)
            .join(" "),
    });

    // ─── Thread-level actions ─────────────────────────────────────────────────

    const handleTrash = async () => {
        if (!thread) return;
        await apiPost("/email/messages/trash", { folder, uids: getAllUIDs() });
        if (folder.toUpperCase() === "TRASH") {
            toast.success("Deleted permanently");
        } else {
            toast.success("Moved to trash");
        }
        router.push(`/mail/${folder.toLowerCase()}`);
    };

    const handleArchive = async () => {
        if (!thread) return;
        await apiPost("/email/messages/move", {
            src_folder: folder,
            dst_folder: "Archive",
            uids: getAllUIDs(),
        });
        toast.success("Archived");
        router.push(`/mail/${folder.toLowerCase()}`);
    };

    const handleMoveToInbox = async () => {
        if (!thread) return;
        await apiPost("/email/messages/move", {
            src_folder: folder,
            dst_folder: "INBOX",
            uids: getAllUIDs(),
        });
        toast.success("Moved to Inbox");
        router.push(`/mail/${folder.toLowerCase()}`);
    };

    const handleMarkRead = async () => {
        if (!thread) return;
        const unreadUids = thread.messages
            .filter((m) => !m.is_read)
            .map((m) => m.imap_uid)
            .filter((uid): uid is number => uid != null);
        if (unreadUids.length > 0) {
            await apiPost("/email/messages/mark-read", { folder, uids: unreadUids, read: true });
        }
        toast.success("Marked as read");
    };

    const handleMarkUnread = async () => {
        if (!thread) return;
        const uids = thread.messages
            .map((m) => m.imap_uid)
            .filter((uid): uid is number => uid != null && uid > 0);
        if (uids.length > 0) {
            await apiPost("/email/messages/mark-read", { folder, uids, read: false });
            toast.success("Marked as unread");
            // Refresh sidebar badge
            if (onUnreadUpdate) {
                onUnreadUpdate();
            }
            router.push(`/mail/${folder.toLowerCase()}`);
        }
    };

    const handleToggleStar = async () => {
        if (!thread) return;
        const newStarred = !thread.is_starred;
        await apiPost("/email/messages/mark-starred", {
            folder,
            uids: getAllUIDs(),
            starred: newStarred,
        });
        toast.info(newStarred ? "Starred" : "Unstarred");
        fetchThread();
    };

    // ─── Per-message actions ──────────────────────────────────────────────────

    const handleReply = (message: EmailMessage) => {
        setForwardData(null);
        setReplyData({
            subject: message.subject,
            to: [message.from],
            cc: message.cc,
            ...buildReplyHeaders(message),
            body_text: buildQuotedReply(message),
        });
        setComposeOpen(true);
    };

    const handleForward = (message: EmailMessage) => {
        setReplyData(null);
        setForwardData({
            subject: message.subject,
            body_html: message.body_html ?? "",
            // Prefer plain text; fall back to stripping the HTML body.
            body_text:
                message.body_text ||
                (message.body_html
                    ? message.body_html.replace(/<[^>]+>/g, " ").replace(/\s+/g, " ").trim()
                    : message.body_preview ?? ""),
        });
        setComposeOpen(true);
    };

    const handleCloseCompose = () => {
        setComposeOpen(false);
        setReplyData(null);
        setForwardData(null);
    };

    // ─── Render ───────────────────────────────────────────────────────────────

    if (loading) {
        return (
            <div className="flex items-center justify-center h-64">
                <div className="flex flex-col items-center gap-3 text-gray-400">
                    <Loader2 className="w-7 h-7 animate-spin text-blue-500" />
                    <span className="text-sm">Loading thread...</span>
                </div>
            </div>
        );
    }

    if (!thread) {
        return (
            <div className="flex items-center justify-center h-64">
                <div className="text-gray-400 text-sm">Thread not found</div>
            </div>
        );
    }

    return (
        <div className="flex flex-col h-full">
            {/* Thread Header */}
            <div className="flex items-center gap-2 px-4 py-3 border-b border-gray-100 dark:border-gray-800 bg-white dark:bg-gray-900 sticky top-0 z-10">
                <Button
                    variant="ghost"
                    size="sm"
                    className="h-8 w-8 p-0 text-gray-500 rounded-[7px]"
                    onClick={() => router.push(`/mail/${folder.toLowerCase()}`)}
                >
                    <ArrowLeft className="w-[18px] h-[18px]" />
                </Button>

                <div className="flex-1 min-w-0 ml-1">
                    <h1 className="text-base font-semibold text-gray-900 truncate leading-tight">
                        {thread.subject || "(no subject)"}
                    </h1>
                    <div className="flex items-center gap-2 text-[11px] text-gray-400 mt-0.5">
                        <span>
                            {thread.message_count} message{thread.message_count !== 1 ? "s" : ""}
                        </span>
                        {thread.has_attachments && <span>· Attachments</span>}
                    </div>
                </div>

                <div className="flex items-center gap-0.5">
                    {(folder.toUpperCase() === "ARCHIVE" || folder.toUpperCase() === "TRASH") ? (
                        <Button
                            variant="ghost"
                            size="sm"
                            className="h-8 w-8 p-0 text-gray-400 hover:text-gray-600 rounded-[7px]"
                            onClick={handleMoveToInbox}
                        >
                            <Inbox className="w-4 h-4" />
                        </Button>
                    ) : (
                        <Button
                            variant="ghost"
                            size="sm"
                            className="h-8 w-8 p-0 text-gray-400 hover:text-gray-600 rounded-[7px]"
                            onClick={handleArchive}
                        >
                            <Archive className="w-4 h-4" />
                        </Button>
                    )}
                    <Button
                        variant="ghost"
                        size="sm"
                        className="h-8 w-8 p-0 text-gray-400 hover:text-red-500 hover:bg-red-50 rounded-[7px]"
                        onClick={handleTrash}
                    >
                        <Trash2 className="w-4 h-4" />
                    </Button>
                    <Button
                        variant="ghost"
                        size="sm"
                        className="h-8 w-8 p-0 text-gray-400 hover:text-gray-600 rounded-[7px]"
                        onClick={handleMarkUnread}
                    >
                        <Mail className="w-4 h-4" />
                    </Button>
                    <Button
                        variant="ghost"
                        size="sm"
                        className="h-8 w-8 p-0 text-gray-400 hover:text-gray-600 rounded-[7px]"
                        onClick={handleMarkRead}
                    >
                        <MailOpen className="w-4 h-4" />
                    </Button>
                    <Button
                        variant="ghost"
                        size="sm"
                        className="h-8 w-8 p-0 rounded-[7px]"
                        onClick={handleToggleStar}
                    >
                        <Star
                            className={cn(
                                "w-4 h-4",
                                thread.is_starred
                                    ? "fill-amber-400 text-amber-400"
                                    : "text-gray-400"
                            )}
                        />
                    </Button>
                </div>
            </div>

            {/* Messages */}
            <ScrollArea className="flex-1">
                {thread.messages.map((message, index) => (
                    <MessageItem
                        key={`${message.id}-${index}`}
                        message={message}
                        isLast={index === thread.messages.length - 1}
                        defaultExpanded={index === thread.messages.length - 1}
                        onReply={handleReply}
                        onForward={handleForward}
                    />
                ))}
            </ScrollArea>

            {/* Compose Dialog */}
            <ComposeDialog
                open={composeOpen}
                onClose={handleCloseCompose}
                replyTo={replyData ?? undefined}
                forward={forwardData ?? undefined}
            />
        </div>
    );
}
