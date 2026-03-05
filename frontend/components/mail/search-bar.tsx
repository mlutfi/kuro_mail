"use client";

import { useState, useCallback } from "react";
import { useRouter } from "next/navigation";
import { Input } from "@/components/ui/input";
import { Search, X } from "lucide-react";
import { cn } from "@/lib/utils";

interface SearchBarProps {
    className?: string;
}

export default function SearchBar({ className }: SearchBarProps) {
    const router = useRouter();
    const [query, setQuery] = useState("");
    const [isFocused, setIsFocused] = useState(false);

    const handleSearch = useCallback(
        (e: React.FormEvent) => {
            e.preventDefault();
            if (query.trim()) {
                router.push(`/mail/search?q=${encodeURIComponent(query.trim())}`);
            }
        },
        [query, router]
    );

    return (
        <form onSubmit={handleSearch} className={cn("relative", className)}>
            <div
                className={cn(
                    "relative flex items-center transition-all duration-200",
                    isFocused
                        ? "bg-white dark:bg-gray-800 shadow-md shadow-gray-200/40 dark:shadow-none ring-1 ring-gray-200 dark:ring-gray-700"
                        : "bg-gray-100/80 dark:bg-gray-800/80 hover:bg-gray-100 dark:hover:bg-gray-800",
                    "rounded-[7px]"
                )}
            >
                <Search className="absolute left-3.5 w-4 h-4 text-gray-400" />
                <Input
                    type="text"
                    placeholder="Search mail..."
                    value={query}
                    onChange={(e) => setQuery(e.target.value)}
                    onFocus={() => setIsFocused(true)}
                    onBlur={() => setIsFocused(false)}
                    className="pl-10 pr-10 h-10 bg-transparent border-0 shadow-none focus-visible:ring-0 rounded-[7px] placeholder:text-gray-400 text-[13px]"
                />
                {query && (
                    <button
                        type="button"
                        onClick={() => setQuery("")}
                        className="absolute right-3 p-0.5 rounded-[5px] hover:bg-gray-200 transition-colors"
                    >
                        <X className="w-4 h-4 text-gray-400" />
                    </button>
                )}
            </div>
        </form>
    );
}
