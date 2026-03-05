"use client";

import { useState, useMemo, useEffect } from "react";
import { format } from "date-fns";
import DOMPurify from "dompurify";
import { EmailMessage, EmailAddress } from "@/lib/types";
import { apiGet, API_URL } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import {
    Star,
    ChevronDown,
    ChevronUp,
    Reply,
    Forward,
    MoreVertical,
    Paperclip,
    Download,
} from "lucide-react";
import {
    DropdownMenu,
    DropdownMenuContent,
    DropdownMenuItem,
    DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { cn } from "@/lib/utils";

interface MessageItemProps {
    message: EmailMessage;
    isLast: boolean;
    defaultExpanded?: boolean;
    onReply: (message: EmailMessage) => void;
    onForward: (message: EmailMessage) => void;
}

function getInitials(addr: EmailAddress): string {
    if (addr.name) {
        return addr.name
            .split(" ")
            .map((w) => w[0])
            .slice(0, 2)
            .join("")
            .toUpperCase();
    }
    return addr.email[0].toUpperCase();
}

function getAvatarColor(email: string): string {
    const colors = [
        "bg-blue-500",
        "bg-emerald-500",
        "bg-violet-500",
        "bg-amber-500",
        "bg-rose-500",
        "bg-teal-500",
        "bg-orange-500",
        "bg-indigo-500",
    ];
    let hash = 0;
    for (let i = 0; i < email.length; i++) {
        hash = email.charCodeAt(i) + ((hash << 5) - hash);
    }
    return colors[Math.abs(hash) % colors.length];
}

function formatAddresses(addrs: EmailAddress[]): string {
    return addrs.map((a) => (a.name ? `${a.name} <${a.email}>` : a.email)).join(", ");
}

function formatFileSize(bytes: number): string {
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1048576) return `${(bytes / 1024).toFixed(1)} KB`;
    return `${(bytes / 1048576).toFixed(1)} MB`;
}

export default function MessageItem({
    message,
    isLast,
    defaultExpanded = false,
    onReply,
    onForward,
}: MessageItemProps) {
    const [expanded, setExpanded] = useState(defaultExpanded || isLast);
    const [showDetails, setShowDetails] = useState(false);
    const [fullMessage, setFullMessage] = useState<EmailMessage | null>(null);
    const [bodyLoading, setBodyLoading] = useState(false);

    // Fetch full message body when expanded and body is not loaded yet
    useEffect(() => {
        if (!expanded) return;
        const msg = fullMessage || message;
        if (msg.body_html || msg.body_text) return; // already have body
        if (bodyLoading || fullMessage) return;
        if (!message.imap_uid || !message.imap_folder) return;

        let cancelled = false;
        const fetchBody = async () => {
            const res = await apiGet<EmailMessage>(
                `/email/messages/${encodeURIComponent(message.imap_folder)}/${message.imap_uid}`
            );
            if (!cancelled && res.success && res.data) {
                setFullMessage(res.data);
                setBodyLoading(false);
            }
        };
        setBodyLoading(true);
        fetchBody();
        return () => { cancelled = true; };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [expanded, message.imap_uid, message.imap_folder]);

    const displayMessage = fullMessage || message;

    const sanitizedHtml = useMemo(() => {
        if (!displayMessage.body_html) return "";
        if (typeof window === "undefined") return displayMessage.body_html;
        return DOMPurify.sanitize(displayMessage.body_html, {
            ADD_TAGS: ["style"],
            ADD_ATTR: ["target"],
        });
    }, [displayMessage.body_html]);

    // Handle attachment download
    const handleDownload = async (attachment: { id: string; filename: string }) => {
        if (!message.imap_uid || !message.imap_folder) {
            console.error("Missing imap_uid or imap_folder for download");
            return;
        }

        try {
            const url = `${API_URL}/email/messages/${encodeURIComponent(message.imap_folder)}/${message.imap_uid}/attachments/${attachment.id}`;
            const token = localStorage.getItem("access_token");

            console.log("[Download] URL:", url);
            console.log("[Download] Attachment ID:", attachment.id);
            console.log("[Download] IMAP UID:", message.imap_uid);
            console.log("[Download] IMAP Folder:", message.imap_folder);

            const response = await fetch(url, {
                headers: token ? { "Authorization": `Bearer ${token}` } : {}
            });

            console.log("[Download] Response status:", response.status);
            console.log("[Download] Response statusText:", response.statusText);

            if (!response.ok) {
                const errorText = await response.text();
                console.error("[Download] Error response:", errorText);
                throw new Error(`Failed to download attachment: ${response.status} ${response.statusText}`);
            }

            const blob = await response.blob();
            const downloadUrl = window.URL.createObjectURL(blob);
            const a = document.createElement("a");
            a.href = downloadUrl;
            a.download = attachment.filename;
            document.body.appendChild(a);
            a.click();
            window.URL.revokeObjectURL(downloadUrl);
            document.body.removeChild(a);
        } catch (error) {
            console.error("Download failed:", error);
            // Don't show alert since we have console logging for debugging
        }
    };

    return (
        <div className={cn("border-b border-gray-100", expanded ? "pb-0" : "")}>
            {/* Header — always visible */}
            <div
                className={cn(
                    "flex items-start gap-3 px-5 py-3 cursor-pointer transition-colors duration-100",
                    !expanded && "hover:bg-gray-50/60"
                )}
                onClick={() => setExpanded(!expanded)}
            >
                {/* Avatar */}
                <Avatar className="w-9 h-9 shrink-0 mt-0.5">
                    <AvatarFallback className={cn("text-white text-xs font-semibold", getAvatarColor(message.from.email))}>
                        {getInitials(message.from)}
                    </AvatarFallback>
                </Avatar>

                <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-2">
                        <span className="font-semibold text-[13px] text-gray-900 truncate">
                            {message.from.name || message.from.email}
                        </span>
                        {!expanded && (
                            <span className="text-[13px] text-gray-400 truncate flex-1">
                                — {message.body_preview}
                            </span>
                        )}
                        <span className="text-[11px] text-gray-400 shrink-0 whitespace-nowrap tabular-nums">
                            {format(new Date(message.received_at), "MMM d, yyyy h:mm a")}
                        </span>
                    </div>

                    {expanded && (
                        <div className="mt-0.5">
                            <button
                                onClick={(e) => {
                                    e.stopPropagation();
                                    setShowDetails(!showDetails);
                                }}
                                className="text-[11px] text-gray-400 hover:text-gray-600 flex items-center gap-1 transition-colors"
                            >
                                to {formatAddresses(message.to).substring(0, 40)}
                                {message.to.length > 1 && "..."}
                                <ChevronDown className="w-3 h-3" />
                            </button>

                            {showDetails && (
                                <div className="mt-2 text-[12px] text-gray-500 space-y-1 bg-gray-50/80 rounded-[7px] p-3 border border-gray-100">
                                    <div><span className="font-medium text-gray-600">From:</span> {formatAddresses([message.from])}</div>
                                    <div><span className="font-medium text-gray-600">To:</span> {formatAddresses(message.to)}</div>
                                    {message.cc && message.cc.length > 0 && (
                                        <div><span className="font-medium text-gray-600">Cc:</span> {formatAddresses(message.cc)}</div>
                                    )}
                                    <div><span className="font-medium text-gray-600">Date:</span> {format(new Date(message.received_at), "EEEE, MMMM d, yyyy 'at' h:mm a")}</div>
                                    <div><span className="font-medium text-gray-600">Subject:</span> {message.subject}</div>
                                </div>
                            )}
                        </div>
                    )}
                </div>

                {/* Actions */}
                {expanded && (
                    <div className="flex items-center gap-0.5 shrink-0" onClick={(e) => e.stopPropagation()}>
                        <Button variant="ghost" size="sm" className="h-8 w-8 p-0 text-gray-400 hover:text-blue-600 rounded-[7px]" onClick={() => onReply(displayMessage)}>
                            <Reply className="w-4 h-4" />
                        </Button>
                        <DropdownMenu>
                            <DropdownMenuTrigger asChild>
                                <Button variant="ghost" size="sm" className="h-8 w-8 p-0 text-gray-400 hover:text-gray-600 rounded-[7px]">
                                    <MoreVertical className="w-4 h-4" />
                                </Button>
                            </DropdownMenuTrigger>
                            <DropdownMenuContent align="end" className="min-w-[140px]">
                                <DropdownMenuItem onClick={() => onReply(displayMessage)} className="text-[13px]">
                                    <Reply className="w-4 h-4 mr-2" /> Reply
                                </DropdownMenuItem>
                                <DropdownMenuItem onClick={() => onForward(displayMessage)} className="text-[13px]">
                                    <Forward className="w-4 h-4 mr-2" /> Forward
                                </DropdownMenuItem>
                            </DropdownMenuContent>
                        </DropdownMenu>
                    </div>
                )}
            </div>

            {/* Body */}
            {expanded && (
                <div className="px-5 pb-5">
                    {/* HTML Content */}
                    <div className="ml-12">
                        {bodyLoading ? (
                            <div className="flex items-center gap-2 text-gray-400 text-sm py-4">
                                <svg className="w-4 h-4 animate-spin" viewBox="0 0 24 24" fill="none">
                                    <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                                    <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z" />
                                </svg>
                                Loading message...
                            </div>
                        ) : sanitizedHtml ? (
                            <div
                                className="prose prose-sm max-w-none text-gray-700 [&_a]:text-blue-600 leading-relaxed"
                                dangerouslySetInnerHTML={{ __html: sanitizedHtml }}
                            />
                        ) : (
                            <pre className="text-[13px] text-gray-700 whitespace-pre-wrap font-sans leading-relaxed">
                                {displayMessage.body_text || displayMessage.body_preview}
                            </pre>
                        )}

                        {/* Attachments */}
                        {displayMessage.attachments && displayMessage.attachments.length > 0 && (
                            <div className="mt-4 flex flex-wrap gap-2">
                                {displayMessage.attachments.map((att) => (
                                    <div
                                        key={att.id}
                                        className="flex items-center gap-2 px-3 py-2 border border-gray-200 rounded-[7px] bg-gray-50/60 hover:bg-gray-100/80 transition-colors cursor-pointer group"
                                        onClick={(e) => {
                                            e.stopPropagation();
                                            handleDownload(att);
                                        }}
                                    >
                                        <Paperclip className="w-4 h-4 text-gray-400" />
                                        <div className="min-w-0">
                                            <div className="text-[13px] text-gray-700 truncate max-w-[200px]">
                                                {att.filename}
                                            </div>
                                            <div className="text-[11px] text-gray-400">
                                                {formatFileSize(att.size)}
                                            </div>
                                        </div>
                                        <Download className="w-4 h-4 text-gray-300 group-hover:text-gray-500 ml-1 transition-colors" />
                                    </div>
                                ))}
                            </div>
                        )}

                        {/* Reply / Forward buttons at bottom of last message */}
                        {isLast && (
                            <div className="mt-5 flex gap-2">
                                <Button
                                    variant="outline"
                                    size="sm"
                                    className="rounded-[7px] text-[13px] h-9 px-4 border-gray-200 hover:bg-gray-50 hover:border-gray-300 transition-all"
                                    onClick={() => onReply(displayMessage)}
                                >
                                    <Reply className="w-4 h-4 mr-1.5" /> Reply
                                </Button>
                                <Button
                                    variant="outline"
                                    size="sm"
                                    className="rounded-[7px] text-[13px] h-9 px-4 border-gray-200 hover:bg-gray-50 hover:border-gray-300 transition-all"
                                    onClick={() => onForward(displayMessage)}
                                >
                                    <Forward className="w-4 h-4 mr-1.5" /> Forward
                                </Button>
                            </div>
                        )}
                    </div>
                </div>
            )}
        </div>
    );
}
