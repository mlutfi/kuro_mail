"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import { useRequireAuth } from "@/lib/auth-context";
import { useWebSocket } from "@/hooks/use-websocket";
import { useFolders } from "@/hooks/use-folders";
import Sidebar from "@/components/mail/sidebar";
import SearchBar from "@/components/mail/search-bar";
import ComposeDialog from "@/components/mail/compose-dialog";
import BottomNav from "@/components/mail/bottom-nav";
import { ThemeToggle } from "@/components/theme-toggle";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import { Sheet, SheetContent, SheetTrigger, SheetTitle } from "@/components/ui/sheet";
import {
    DropdownMenu,
    DropdownMenuContent,
    DropdownMenuItem,
    DropdownMenuSeparator,
    DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Menu, Settings, LogOut, Shield, Wifi, WifiOff, Mail } from "lucide-react";

export default function MailLayout({
    children,
}: {
    children: React.ReactNode;
}) {
    const { user, isLoading, isAuthenticated, logout } = useRequireAuth();
    const { connected, on } = useWebSocket();
    const { refetch: refetchFolders } = useFolders();
    const router = useRouter();

    // Listen for real-time inbox updates to refresh sidebar badge
    useEffect(() => {
        if (!isAuthenticated) return;
        const unsub = on("inbox_update", () => {
            refetchFolders();
        });
        return () => {
            unsub();
        };
    }, [isAuthenticated, on, refetchFolders]);
    const [composeOpen, setComposeOpen] = useState(false);
    const [sidebarOpen, setSidebarOpen] = useState(false);

    if (isLoading) {
        return (
            <div className="min-h-screen bg-gray-50/80 dark:bg-gray-900/80 flex items-center justify-center relative overflow-hidden">
                <div className="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 w-64 h-64 bg-blue-400/20 rounded-full blur-3xl animate-pulse"></div>

                <div className="relative flex flex-col items-center z-10">
                    <div className="relative">
                        <div className="absolute inset-0 bg-blue-400 rounded-3xl animate-ping opacity-20" style={{ animationDuration: '3s' }}></div>

                        <div className="relative w-20 h-20 bg-white dark:bg-gray-800 rounded-3xl flex items-center justify-center shadow-xl shadow-blue-500/10 border border-blue-50 dark:border-gray-700 animate-float">
                            <Mail className="w-10 h-10 text-blue-600" strokeWidth={1.5} />
                        </div>

                        <div className="w-12 h-2 bg-blue-600/20 rounded-[100%] mx-auto mt-6 blur-[2px] animate-pulse-shadow"></div>
                    </div>

                    <div className="mt-8 flex flex-col items-center gap-2.5">
                        <span className="text-[13px] font-semibold text-blue-900/60 dark:text-white tracking-widest uppercase">
                            KuroMail
                        </span>
                        <div className="flex gap-1.5">
                            <span className="w-1.5 h-1.5 rounded-full bg-blue-600/40 animate-bounce" style={{ animationDelay: '-0.3s' }}></span>
                            <span className="w-1.5 h-1.5 rounded-full bg-blue-600/60 animate-bounce" style={{ animationDelay: '-0.15s' }}></span>
                            <span className="w-1.5 h-1.5 rounded-full bg-blue-600/80 animate-bounce"></span>
                        </div>
                    </div>
                </div>
            </div>
        );
    }

    if (!isAuthenticated) {
        return null; // useRequireAuth will redirect
    }

    const userInitials = user?.display_name
        ? user.display_name
            .split(" ")
            .map((w) => w[0])
            .slice(0, 2)
            .join("")
            .toUpperCase()
        : user?.email?.[0]?.toUpperCase() || "?";

    return (
        <div className="h-screen flex flex-col bg-gray-50/80 dark:bg-gray-900/80">
            {/* Top Bar */}
            <header className="h-14 border-b border-gray-100 dark:border-gray-800 bg-white dark:bg-gray-900 flex items-center gap-3 px-4 shrink-0 z-20">
                {/* Mobile menu */}
                <Sheet open={sidebarOpen} onOpenChange={setSidebarOpen}>
                    <SheetTrigger asChild>
                        <Button variant="ghost" size="sm" className="lg:hidden h-8 w-8 p-0 rounded-[7px]">
                            <Menu className="w-[18px] h-[18px]" />
                        </Button>
                    </SheetTrigger>
                    <SheetContent side="left" className="w-72 p-0">
                        <SheetTitle className="sr-only">Menu</SheetTitle>
                        <Sidebar
                            onCompose={() => {
                                setSidebarOpen(false);
                                setComposeOpen(true);
                            }}
                            onNavigate={() => setSidebarOpen(false)}
                        />
                    </SheetContent>
                </Sheet>

                {/* Logo */}
                <div className="flex items-center gap-2 shrink-0">
                    <div className="w-8 h-8 md:w-9 md:h-9 bg-blue-600 rounded-[8px] flex items-center justify-center">
                        <span className="text-white text-sm md:text-base font-bold">K</span>
                    </div>
                    <span className="text-lg md:text-xl text-gray-800 dark:text-white hidden sm:inline tracking-tight">
                        <span className="font-normal">Kuro</span><span className="font-bold">Mail</span>
                    </span>
                </div>

                {/* Search */}
                <SearchBar className="flex-1 max-w-xl mx-auto" />

                {/* Connection status */}
                <div className="shrink-0">
                    {connected ? (
                        <Wifi className="w-4 h-4 text-emerald-500" />
                    ) : (
                        <WifiOff className="w-4 h-4 text-gray-300 dark:text-gray-500" />
                    )}
                </div>

                <div className="flex items-center gap-2">
                    <ThemeToggle />
                    {/* User menu */}
                    <DropdownMenu>
                        <DropdownMenuTrigger asChild>
                            <button className="shrink-0 focus:outline-none flex items-center gap-3 cursor-pointer">
                                <div className="hidden md:flex flex-col items-end text-right">
                                    <span className="text-[13px] font-semibold text-gray-800 dark:text-gray-200 leading-tight">{user?.display_name || "User"}</span>
                                    <span className="text-[11px] text-gray-500 dark:text-gray-400 leading-tight">{user?.email || "user@kuromail.com"}</span>
                                </div>
                                <Avatar className="w-9 h-9 md:w-10 md:h-10 cursor-pointer ring-2 ring-transparent hover:ring-blue-100 dark:hover:ring-blue-900 transition-all shadow-sm">
                                    <AvatarFallback className="bg-blue-600 text-white text-sm font-bold">
                                        {userInitials}
                                    </AvatarFallback>
                                </Avatar>
                            </button>
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end" className="w-56 rounded-[7px]">
                            <div className="px-3 py-2.5">
                                <p className="text-[13px] font-medium">{user?.display_name}</p>
                                <p className="text-[11px] text-gray-400">{user?.email}</p>
                            </div>
                            <DropdownMenuSeparator />
                            <DropdownMenuItem onClick={() => router.push("/mail/settings")} className="text-[13px] rounded-[5px]">
                                <Settings className="w-4 h-4 mr-2" />
                                Settings
                            </DropdownMenuItem>
                            <DropdownMenuItem onClick={() => router.push("/mail/settings")} className="text-[13px] rounded-[5px]">
                                <Shield className="w-4 h-4 mr-2" />
                                Security & 2FA
                            </DropdownMenuItem>
                            <DropdownMenuSeparator />
                            <DropdownMenuItem
                                className="text-red-500 focus:text-red-600 text-[13px] rounded-[5px]"
                                onClick={async () => {
                                    await logout();
                                    router.push("/login");
                                }}
                            >
                                <LogOut className="w-4 h-4 mr-2" />
                                Sign out
                            </DropdownMenuItem>
                        </DropdownMenuContent>
                    </DropdownMenu>
                </div>
            </header>

            {/* Main */}
            <div className="flex-1 flex overflow-hidden">
                {/* Desktop Sidebar */}
                <aside className="hidden lg:flex w-60 border-r border-gray-100 dark:border-gray-800 bg-white dark:bg-gray-900 shrink-0">
                    <Sidebar onCompose={() => setComposeOpen(true)} />
                </aside>

                {/* Content */}
                <main className="flex-1 overflow-hidden bg-white dark:bg-background">
                    {children}
                </main>
            </div>

            {/* Mobile Bottom Navigation */}
            <BottomNav onCompose={() => setComposeOpen(true)} />

            {/* Compose */}
            <ComposeDialog
                open={composeOpen}
                onClose={() => setComposeOpen(false)}
            />
        </div>
    );
}
