"use client";

import { useParams, useSearchParams } from "next/navigation";
import ThreadView from "@/components/mail/thread-view";

export default function ThreadDetailPage() {
    const params = useParams();
    const searchParams = useSearchParams();
    const threadId = params.id as string;
    const folder = searchParams.get("folder") || "INBOX";

    return <ThreadView threadId={threadId} folder={folder} />;
}
