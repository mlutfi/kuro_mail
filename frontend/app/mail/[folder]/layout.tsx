import { Metadata } from "next";

export async function generateMetadata(props: { params: Promise<{ folder?: string }> }): Promise<Metadata> {
    const params = await props.params;
    const slug = params.folder || "inbox";
    const title = slug.charAt(0).toUpperCase() + slug.slice(1);
    return {
        title,
    };
}

export default function FolderLayout({ children }: { children: React.ReactNode }) {
    return <>{children}</>;
}
