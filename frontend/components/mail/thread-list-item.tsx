"use client";

import { useRouter } from "next/navigation";
import { format, isToday, isYesterday, isThisYear } from "date-fns";
import { Star, Paperclip } from "lucide-react";
import { Checkbox } from "@/components/ui/checkbox";
import { ThreadListItem, EmailAddress } from "@/lib/types";
import { cn } from "@/lib/utils";

interface ThreadListItemComponentProps {
    thread: ThreadListItem;
    isSelected: boolean;
    onSelect: (threadId: string, checked: boolean) => void;
    onStarToggle: (threadId: string) => void;
    folder: string;
}

function formatParticipants(participants: EmailAddress[]): string {
    if (!participants || participants.length === 0) return "Unknown";
    if (participants.length === 1) {
        return participants[0].name || participants[0].email.split("@")[0];
    }
    const first = participants[0].name || participants[0].email.split("@")[0];
    return `${first} (+${participants.length - 1})`;
}

function formatDate(dateStr: string): string {
    const date = new Date(dateStr);
    if (isToday(date)) return format(date, "h:mm a");
    if (isYesterday(date)) return "Yesterday";
    if (isThisYear(date)) return format(date, "MMM d");
    return format(date, "MMM d, yyyy");
}

export default function ThreadListItemComponent({
    thread,
    isSelected,
    onSelect,
    onStarToggle,
    folder,
}: ThreadListItemComponentProps) {
    const router = useRouter();
    const isUnread = thread.unread_count > 0;

    const handleClick = () => {
        router.push(`/mail/thread/${thread.thread_id}?folder=${folder}`);
    };

    return (
        <div
            className={cn(
                "group flex items-center gap-2 px-3 py-2.5 border-b border-gray-50 dark:border-gray-800/50 cursor-pointer transition-colors duration-100",
                isSelected
                    ? "bg-blue-50/80 dark:bg-blue-900/20"
                    : isUnread
                        ? "bg-white dark:bg-gray-800 hover:bg-gray-50/60 dark:hover:bg-gray-800/80"
                        : "bg-gray-50/30 dark:bg-gray-800/30 hover:bg-gray-50/60 dark:hover:bg-gray-800/80"
            )}
        >
            {/* Checkbox */}
            <div className="shrink-0 px-0.5">
                <Checkbox
                    checked={isSelected}
                    onCheckedChange={(checked) => onSelect(thread.thread_id, !!checked)}
                    className="border-gray-300 data-[state=checked]:bg-blue-600 data-[state=checked]:border-blue-600"
                    onClick={(e) => e.stopPropagation()}
                />
            </div>

            {/* Star */}
            <button
                onClick={(e) => {
                    e.stopPropagation();
                    onStarToggle(thread.thread_id);
                }}
                className="shrink-0 p-0.5"
            >
                <Star
                    className={cn(
                        "w-4 h-4 transition-colors",
                        thread.is_starred
                            ? "fill-amber-400 text-amber-400"
                            : "text-gray-300 hover:text-amber-300"
                    )}
                />
            </button>

            {/* Content — clickable */}
            <div className="flex-1 min-w-0 flex items-center gap-3" onClick={handleClick}>
                {/* Participants */}
                <div
                    className={cn(
                        "w-[180px] shrink-0 truncate text-[13px]",
                        isUnread ? "font-semibold text-gray-900 dark:text-gray-400" : "text-gray-600 dark:text-gray-300"
                    )}
                >
                    {formatParticipants(thread.participants)}
                    {thread.message_count > 1 && (
                        <span className="ml-1 text-[11px] text-gray-400 dark:text-gray-500 font-normal">
                            ({thread.message_count})
                        </span>
                    )}
                </div>

                {/* Subject + Preview */}
                <div className="flex-1 min-w-0 flex items-baseline gap-1.5">
                    <span
                        className={cn(
                            "truncate text-[13px]",
                            isUnread ? "font-semibold text-gray-900 dark:text-gray-500" : "text-gray-700 dark:text-gray-400"
                        )}
                    >
                        {thread.subject || "(no subject)"}
                    </span>
                    <span className="hidden md:inline text-[13px] text-gray-400 truncate flex-1">
                        — {thread.body_preview}
                    </span>
                </div>

                {/* Attachment icon */}
                {thread.has_attachments && (
                    <Paperclip className="w-3.5 h-3.5 shrink-0 text-gray-400" />
                )}

                {/* Date */}
                <span
                    className={cn(
                        "shrink-0 text-[11px] whitespace-nowrap tabular-nums",
                        isUnread ? "font-semibold text-gray-800" : "text-gray-400"
                    )}
                >
                    {formatDate(thread.latest_date)}
                </span>
            </div>

            {/* Unread dot */}
            {isUnread && (
                <div className="w-2 h-2 shrink-0 rounded-full bg-blue-500 shadow-sm shadow-blue-500/30" />
            )}
        </div>
    );
}
