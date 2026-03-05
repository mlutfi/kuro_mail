"use client";

import { useState, useEffect } from "react";
import { useSearchParams } from "next/navigation";
import { apiGet } from "@/lib/api";
import { PaginatedThreads, ThreadListItem } from "@/lib/types";
import ThreadListItemComponent from "@/components/mail/thread-list-item";
import { Button } from "@/components/ui/button";
import { Loader2, Search as SearchIcon, ArrowLeft } from "lucide-react";
import { useRouter } from "next/navigation";
import { ScrollArea } from "@/components/ui/scroll-area";

export default function SearchPage() {
    const searchParams = useSearchParams();
    const router = useRouter();
    const query = searchParams.get("q") || "";
    const [results, setResults] = useState<ThreadListItem[]>([]);
    const [loading, setLoading] = useState(false);
    const [total, setTotal] = useState(0);

    useEffect(() => {
        if (!query) return;

        const fetchSearch = async () => {
            setLoading(true);
            const res = await apiGet<PaginatedThreads>(`/email/search?q=${encodeURIComponent(query)}`);
            if (res.success && res.data) {
                setResults(res.data.data || []);
                setTotal(res.data.total);
            }
            setLoading(false);
        };

        fetchSearch();
    }, [query]);

    return (
        <div className="flex flex-col h-full">
            {/* Header */}
            <div className="flex items-center gap-3 px-4 py-3 border-b">
                <Button
                    variant="ghost"
                    size="sm"
                    className="h-9 w-9 p-0"
                    onClick={() => router.push("/mail/inbox")}
                >
                    <ArrowLeft className="w-5 h-5" />
                </Button>
                <SearchIcon className="w-5 h-5 text-gray-400" />
                <span className="text-sm text-gray-600">
                    Search results for <strong>&ldquo;{query}&rdquo;</strong>
                    {!loading && <span className="text-gray-400 ml-2">({total} results)</span>}
                </span>
            </div>

            {/* Results */}
            <ScrollArea className="flex-1">
                {loading ? (
                    <div className="flex items-center justify-center h-64">
                        <Loader2 className="w-8 h-8 animate-spin text-gray-400" />
                    </div>
                ) : results.length === 0 ? (
                    <div className="flex flex-col items-center justify-center h-64 text-gray-400 gap-2">
                        <SearchIcon className="w-12 h-12 text-gray-200" />
                        <span>No results found</span>
                    </div>
                ) : (
                    results.map((thread) => (
                        <ThreadListItemComponent
                            key={thread.thread_id}
                            thread={thread}
                            isSelected={false}
                            onSelect={() => { }}
                            onStarToggle={() => { }}
                            folder="INBOX"
                        />
                    ))
                )}
            </ScrollArea>
        </div>
    );
}
