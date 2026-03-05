import { clsx, type ClassValue } from "clsx"
import { twMerge } from "tailwind-merge"

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

/**
 * Maps a URL-friendly folder slug to the correct IMAP folder name.
 * IMAP folder names are case-sensitive ("Sent" ≠ "SENT").
 */
const FOLDER_SLUG_TO_IMAP: Record<string, string> = {
  inbox: "INBOX",
  sent: "Sent",
  drafts: "Drafts",
  trash: "Trash",
  junk: "Junk",
  spam: "Junk",
  archive: "Archive",
  starred: "Starred", // Virtual — IMAP servers may not have this folder
};

export function getImapFolderName(slug: string): string {
  const lower = slug.toLowerCase();
  return FOLDER_SLUG_TO_IMAP[lower] ?? slug;
}

/** Maps an IMAP folder name back to its URL slug */
export function getFolderSlug(imapFolder: string): string {
  const entry = Object.entries(FOLDER_SLUG_TO_IMAP).find(
    ([, v]) => v.toLowerCase() === imapFolder.toLowerCase()
  );
  return entry ? entry[0] : imapFolder.toLowerCase();
}
