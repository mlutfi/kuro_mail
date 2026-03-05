"use client";

import { useParams } from "next/navigation";
import ThreadList from "@/components/mail/thread-list";
import { getImapFolderName } from "@/lib/utils";

export default function FolderPage() {
    const params = useParams();
    const slug = (params.folder as string) || "inbox";
    const folder = getImapFolderName(slug);

    return <ThreadList folder={folder} />;
}
