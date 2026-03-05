"use client";

import { usePathname, useRouter } from "next/navigation";
import { useEffect } from "react";
import { useFolders } from "@/hooks/use-folders";
import { Button } from "@/components/ui/button";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Separator } from "@/components/ui/separator";
import {
    Inbox,
    Send,
    FileText,
    Trash2,
    AlertTriangle,
    Star,
    Archive,
    PenSquare,
    Mail,
    Tag,
    Loader2,
} from "lucide-react";
import { cn } from "@/lib/utils";

interface SidebarProps {
    onCompose: () => void;
    collapsed?: boolean;
    onUnreadUpdate?: () => void;
    onNavigate?: () => void;
}

const SYSTEM_FOLDERS = [
    { name: "INBOX", slug: "inbox", label: "Inbox", icon: Inbox },
    { name: "Sent", slug: "sent", label: "Sent", icon: Send },
    { name: "Drafts", slug: "drafts", label: "Drafts", icon: FileText },
    { name: "Starred", slug: "starred", label: "Starred", icon: Star },
    { name: "Archive", slug: "archive", label: "Archive", icon: Archive },
    { name: "Junk", slug: "junk", label: "Spam", icon: AlertTriangle },
    { name: "Trash", slug: "trash", label: "Trash", icon: Trash2 },
];

export default function Sidebar({ onCompose, collapsed, onUnreadUpdate, onNavigate }: SidebarProps) {
    const pathname = usePathname();
    const router = useRouter();
    const { folders, loading } = useFolders();

    // Call onUnreadUpdate when folders change (for real-time badge updates)
    useEffect(() => {
        if (onUnreadUpdate && folders.length > 0) {
            onUnreadUpdate();
        }
    }, [folders, onUnreadUpdate]);

    const currentFolder = pathname.split("/mail/")[1]?.split("/")[0]?.toLowerCase() || "inbox";

    const getUnreadCount = (imapFolderName: string): number => {
        const stats = folders.find(
            (f) => f.folder.toLowerCase() === imapFolderName.toLowerCase()
        );
        return stats?.unread_count || 0;
    };

    return (
        <div className="flex flex-col h-full w-full">
            {/* Logo Navigation Header */}
            <div className="flex items-center gap-2 px-4 py-3 shrink-0 lg:hidden border-b border-gray-100">
                <div className="w-8 h-8 bg-blue-600 rounded-[8px] flex items-center justify-center shadow-sm">
                    <span className="text-white text-sm font-bold">K</span>
                </div>
                <span className="text-lg tracking-tight text-gray-800 dark:text-white">
                    <span className="font-normal">Kuro</span><span className="font-bold">Mail</span>
                </span>
            </div>

            {/* Compose Button */}
            <div className="px-2 py-3">
                <Button
                    onClick={onCompose}
                    className={cn(
                        "bg-blue-600 hover:bg-blue-700 text-white shadow-md shadow-blue-600/20 transition-all duration-200 hover:shadow-lg hover:shadow-blue-600/25 active:scale-[0.97] font-medium",
                        collapsed ? "w-11 h-11 p-0 rounded-[7px]" : "w-full h-11 rounded-[7px]"
                    )}
                >
                    <PenSquare className="w-[18px] h-[18px]" />
                    {!collapsed && <span className="ml-2">Compose</span>}
                </Button>
            </div>

            {/* Folder List */}
            <ScrollArea className="flex-1 px-2">
                <nav className="space-y-0.5">
                    {SYSTEM_FOLDERS.map((folder) => {
                        const isActive = currentFolder === folder.slug;
                        const unread = folder.slug === "inbox" ? getUnreadCount(folder.name) : 0;
                        const Icon = folder.icon;

                        return (
                            <button
                                key={folder.name}
                                onClick={() => {
                                    router.push(`/mail/${folder.slug}`);
                                    if (onNavigate) onNavigate();
                                }}
                                className={cn(
                                    "w-full flex items-center gap-3 px-3 py-2 rounded-[7px] text-[13px] font-medium transition-all duration-150 cursor-pointer",
                                    isActive
                                        ? "bg-blue-50 text-blue-700"
                                        : "text-gray-600 hover:bg-gray-100/80 hover:text-gray-800"
                                )}
                            >
                                <Icon
                                    className={cn(
                                        "w-[18px] h-[18px] shrink-0",
                                        isActive ? "text-blue-600" : "text-gray-400"
                                    )}
                                />
                                {!collapsed && (
                                    <>
                                        <span className="flex-1 text-left">{folder.label}</span>
                                        {unread > 0 && (
                                            <span
                                                className={cn(
                                                    "text-[11px] font-semibold tabular-nums min-w-[20px] text-center px-1.5 py-0.5 rounded-full",
                                                    isActive
                                                        ? "bg-blue-100 text-blue-700"
                                                        : "bg-gray-200/80 text-gray-500"
                                                )}
                                            >
                                                {unread > 99 ? "99+" : unread}
                                            </span>
                                        )}
                                    </>
                                )}
                            </button>
                        );
                    })}
                </nav>

                <Separator className="my-3 bg-gray-100" />

                {/* Labels section */}
                {!collapsed && (
                    <>
                        <div className="px-3 mb-2">
                            <span className="text-[11px] font-semibold text-gray-400 uppercase tracking-wider">
                                Labels
                            </span>
                        </div>
                        <div className="space-y-0.5">
                            <button className="w-full flex items-center gap-3 px-3 py-2 rounded-[7px] text-[13px] text-gray-500 hover:bg-gray-100/80 transition-colors cursor-pointer">
                                <Tag className="w-4 h-4 text-gray-400" />
                                <span>Manage labels</span>
                            </button>
                        </div>
                    </>
                )}
            </ScrollArea>

            {/* Footer / Storage info */}
            {!collapsed && (
                <div className="px-4 py-3 border-t border-gray-100 dark:border-gray-800">
                    <div className="flex items-center gap-2 text-[11px] text-gray-400">
                        <Mail className="w-3.5 h-3.5" />
                        {loading ? (
                            <Loader2 className="w-3 h-3 animate-spin" />
                        ) : (
                            <span>
                                {folders.reduce((sum, f) => sum + f.total_count, 0)} emails
                            </span>
                        )}
                    </div>
                </div>
            )}
        </div>
    );
}
