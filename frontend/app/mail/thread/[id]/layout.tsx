import { Metadata } from "next";

export const metadata: Metadata = {
    title: "Thread",
};

export default function ThreadLayout({ children }: { children: React.ReactNode }) {
    return <>{children}</>;
}
