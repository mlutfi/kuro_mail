"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import { Send, X, Minus, ChevronUp, Loader2, Trash2, Paperclip, FileIcon } from "lucide-react";
import { apiPost } from "@/lib/api";
import { ComposeRequest, ComposeAttachment, EmailAddress } from "@/lib/types";
import { toast } from "sonner";
import { cn } from "@/lib/utils";
import { ScrollArea } from "@/components/ui/scroll-area";

// ─── Types ────────────────────────────────────────────────────────────────────

interface ReplyData {
    subject: string;
    to: EmailAddress[];
    cc?: EmailAddress[];
    in_reply_to?: string;
    references?: string;
    body_text?: string;
    body_html?: string;
}

interface ForwardData {
    subject: string;
    body_html: string;
    body_text: string;
}

interface ComposeDialogProps {
    open: boolean;
    onClose: () => void;
    replyTo?: ReplyData;
    forward?: ForwardData;
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

function parseEmailAddresses(input: string): EmailAddress[] {
    return input
        .split(",")
        .map((s) => s.trim())
        .filter(Boolean)
        .map((email) => ({ name: "", email }));
}

/**
 * Converts plain text to simple HTML, preserving line breaks.
 */
function textToHtml(text: string): string {
    return `<p>${text.replace(/\n/g, "<br/>")}</p>`;
}

/**
 * Strips HTML tags to produce a plain-text fallback.
 */
function htmlToText(html: string): string {
    return html.replace(/<[^>]+>/g, " ").replace(/\s+/g, " ").trim();
}

/**
 * Reads a File as a base64 string
 */
function fileToBase64(file: File): Promise<string> {
    return new Promise((resolve, reject) => {
        const reader = new FileReader();
        reader.readAsDataURL(file);
        reader.onload = () => {
            let encoded = reader.result?.toString() || "";
            // Optionally remove the data:*/*;base64, prefix here or let backend do it
            // Backend currently handles it, but let's strip it to save payload size if we want
            const idx = encoded.indexOf(',');
            if (idx !== -1) {
                encoded = encoded.substring(idx + 1);
            }
            resolve(encoded);
        };
        reader.onerror = (error) => reject(error);
    });
}

/**
 * Format bytes to readable string (KB, MB)
 */
function formatBytes(bytes: number, decimals = 2) {
    if (!+bytes) return '0 Bytes';
    const k = 1024;
    const dm = decimals < 0 ? 0 : decimals;
    const sizes = ['Bytes', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return `${parseFloat((bytes / Math.pow(k, i)).toFixed(dm))} ${sizes[i]}`;
}

const MAX_FILE_SIZE_BYTES = 10 * 1024 * 1024; // 10MB

// ─── Component ───────────────────────────────────────────────────────────────

export default function ComposeDialog({
    open,
    onClose,
    replyTo,
    forward,
}: ComposeDialogProps) {
    const router = useRouter();
    const [to, setTo] = useState("");
    const [cc, setCc] = useState("");
    const [bcc, setBcc] = useState("");
    const [subject, setSubject] = useState("");
    const [bodyText, setBodyText] = useState("");
    const [showCcBcc, setShowCcBcc] = useState(false);
    const [isMinimized, setIsMinimized] = useState(false);
    const [isSending, setIsSending] = useState(false);
    const [uploadProgress, setUploadProgress] = useState(0);

    // Attachments state
    const [attachments, setAttachments] = useState<{ file: File; base64: string }[]>([]);
    const [isProcessingFiles, setIsProcessingFiles] = useState(false);

    // Populate the form whenever replyTo / forward / open changes.
    useEffect(() => {
        if (!open) return;

        setBcc("");
        setIsMinimized(false);
        setAttachments([]);
        setUploadProgress(0);

        if (replyTo) {
            setTo(replyTo.to.map((a) => a.email).join(", "));
            setCc(replyTo.cc?.map((a) => a.email).join(", ") ?? "");
            setSubject(`Re: ${replyTo.subject.replace(/^(Re|Fwd):\s*/i, "")}`);
            // Use provided body_text (quoted reply) if available.
            setBodyText(replyTo.body_text ?? "");
            setShowCcBcc(!!(replyTo.cc && replyTo.cc.length > 0));
        } else if (forward) {
            setTo("");
            setCc("");
            setSubject(`Fwd: ${forward.subject.replace(/^Fwd:\s*/i, "")}`);
            // Build forwarded body: prefer plain text, fall back to stripping HTML.
            const originalText = forward.body_text || htmlToText(forward.body_html);
            setBodyText(
                originalText
                    ? `\n\n---------- Forwarded message ----------\n${originalText}`
                    : ""
            );
            setShowCcBcc(false);
        } else {
            setTo("");
            setCc("");
            setSubject("");
            setBodyText("");
            setShowCcBcc(false);
        }
    }, [open, replyTo, forward]);

    // ─── Actions ─────────────────────────────────────────────────────────────

    const handleFileChange = async (e: React.ChangeEvent<HTMLInputElement>) => {
        const files = e.target.files;
        if (!files || files.length === 0) return;

        setIsProcessingFiles(true);
        const newAttachments: { file: File, base64: string }[] = [];

        try {
            for (let i = 0; i < files.length; i++) {
                const file = files[i];
                if (file.size > MAX_FILE_SIZE_BYTES) {
                    toast.error(`File ${file.name} exceeds 10MB limit.`);
                    continue;
                }
                const base64 = await fileToBase64(file);
                newAttachments.push({ file, base64 });
            }
            setAttachments((prev) => [...prev, ...newAttachments]);
        } catch {
            toast.error("Failed to process some files");
        } finally {
            setIsProcessingFiles(false);
            // Reset input so the same file over again can be selected if removed
            if (e.target) {
                e.target.value = '';
            }
        }
    };

    const removeAttachment = (indexToRemove: number) => {
        setAttachments(attachments.filter((_, i) => i !== indexToRemove));
    };

    const handleSend = async () => {
        if (!to.trim()) {
            toast.error("Please add at least one recipient");
            return;
        }
        if (!subject.trim()) {
            toast.error("Please add a subject");
            return;
        }

        setIsSending(true);
        setUploadProgress(10); // Start progress

        try {
            const composeAttachments: ComposeAttachment[] = attachments.map(att => ({
                filename: att.file.name,
                mime_type: att.file.type || 'application/octet-stream',
                base64: att.base64
            }));

            const body: ComposeRequest = {
                to: parseEmailAddresses(to),
                cc: cc ? parseEmailAddresses(cc) : undefined,
                bcc: bcc ? parseEmailAddresses(bcc) : undefined,
                subject,
                body_text: bodyText,
                body_html: textToHtml(bodyText),
                attachments: composeAttachments.length > 0 ? composeAttachments : undefined,
                // Threading headers — set for replies; omitted for new messages / forwards.
                in_reply_to: replyTo?.in_reply_to,
                references: replyTo?.references,
            };

            setUploadProgress(40); // Pre-flight

            const res = await apiPost("/email/send", body);

            setUploadProgress(100);

            if (res.success) {
                toast.success("Email sent!");
                onClose();
            } else {
                toast.error("Failed to send", { description: res.error });
            }
        } catch {
            toast.error("Failed to send email");
        } finally {
            setTimeout(() => {
                setIsSending(false);
                setUploadProgress(0);
            }, 300);
        }
    };

    const handleSaveDraft = async () => {
        try {
            await apiPost("/email/drafts", {
                to: parseEmailAddresses(to),
                cc: cc ? parseEmailAddresses(cc) : undefined,
                bcc: bcc ? parseEmailAddresses(bcc) : undefined,
                subject,
                body_text: bodyText,
                body_html: textToHtml(bodyText),
            });
            toast.success("Draft saved");
            onClose();
            router.push("/mail/drafts");
        } catch {
            toast.error("Failed to save draft");
        }
    };

    // ─── Render ───────────────────────────────────────────────────────────────

    if (!open) return null;

    return (
        <div
            className={cn(
                "fixed z-50 shadow-2xl border border-border/40 bg-white dark:bg-gray-900 flex flex-col transition-all duration-200 rounded-t-[7px]",
                isMinimized
                    ? "bottom-0 right-4 w-[320px] h-10"
                    : "bottom-0 right-4 w-[560px] h-[520px]"
            )}
        >
            {/* Header */}
            <div
                className="flex items-center justify-between px-4 py-2.5 bg-gray-900 rounded-t-[7px] cursor-pointer select-none"
                onClick={() => isMinimized && setIsMinimized(false)}
            >
                <span className="text-sm font-medium text-white truncate">
                    {subject || "New Message"}
                </span>
                <div className="flex items-center gap-0.5">
                    <button
                        onClick={(e) => {
                            e.stopPropagation();
                            setIsMinimized(!isMinimized);
                        }}
                        className="p-1 hover:bg-white/10 rounded-[5px] text-gray-300 transition-colors"
                    >
                        {isMinimized ? (
                            <ChevronUp className="w-4 h-4" />
                        ) : (
                            <Minus className="w-4 h-4" />
                        )}
                    </button>
                    <button
                        onClick={(e) => {
                            e.stopPropagation();
                            onClose();
                        }}
                        className="p-1 hover:bg-white/10 rounded-[5px] text-gray-300 transition-colors"
                    >
                        <X className="w-4 h-4" />
                    </button>
                </div>
            </div>

            {/* Body */}
            {!isMinimized && (
                <>
                    <div className="flex-1 flex flex-col overflow-hidden">
                        {/* To */}
                        <div className="flex items-center gap-2 px-4 py-2 border-b border-gray-100 dark:border-gray-800">
                            <Label className="text-xs text-gray-400 w-8 shrink-0">To</Label>
                            <Input
                                value={to}
                                onChange={(e) => setTo(e.target.value)}
                                placeholder="recipient@example.com"
                                className="border-0 shadow-none focus-visible:ring-0 h-8 text-sm px-0"
                            />
                            {!showCcBcc && (
                                <button
                                    onClick={() => setShowCcBcc(true)}
                                    className="text-xs text-gray-400 hover:text-blue-500 shrink-0 transition-colors"
                                >
                                    Cc/Bcc
                                </button>
                            )}
                        </div>

                        {/* Cc / Bcc */}
                        {showCcBcc && (
                            <>
                                <div className="flex items-center gap-2 px-4 py-2 border-b border-gray-100 dark:border-gray-800">
                                    <Label className="text-xs text-gray-400 w-8 shrink-0">Cc</Label>
                                    <Input
                                        value={cc}
                                        onChange={(e) => setCc(e.target.value)}
                                        className="border-0 shadow-none focus-visible:ring-0 h-8 text-sm px-0"
                                    />
                                </div>
                                <div className="flex items-center gap-2 px-4 py-2 border-b border-gray-100 dark:border-gray-800">
                                    <Label className="text-xs text-gray-400 w-8 shrink-0">Bcc</Label>
                                    <Input
                                        value={bcc}
                                        onChange={(e) => setBcc(e.target.value)}
                                        className="border-0 shadow-none focus-visible:ring-0 h-8 text-sm px-0"
                                    />
                                </div>
                            </>
                        )}

                        {/* Subject */}
                        <div className="flex items-center gap-2 px-4 py-2 border-b border-gray-100 dark:border-gray-800">
                            <Input
                                value={subject}
                                onChange={(e) => setSubject(e.target.value)}
                                placeholder="Subject"
                                className="border-0 shadow-none focus-visible:ring-0 h-8 text-sm px-0 font-medium"
                            />
                        </div>

                        {/* Body */}
                        <Textarea
                            value={bodyText}
                            onChange={(e) => setBodyText(e.target.value)}
                            placeholder="Write your message..."
                            className="flex-1 border-0 shadow-none focus-visible:ring-0 resize-none text-sm p-4 rounded-none min-h-[120px]"
                        />

                        {/* Attachments List */}
                        {attachments.length > 0 && (
                            <ScrollArea className="px-4 py-2 border-t border-gray-100 dark:border-gray-800 bg-gray-50 dark:bg-gray-800/50 flex gap-2 flex-wrap max-h-24">
                                {attachments.map((att, i) => (
                                    <div key={i} className="flex items-center gap-2 bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-[5px] px-2 py-1 text-xs">
                                        <FileIcon className="w-3.5 h-3.5 text-gray-400 shrink-0" />
                                        <div className="flex flex-col max-w-[120px]">
                                            <span className="truncate font-medium text-gray-700 leading-tight">{att.file.name}</span>
                                            <span className="text-[10px] text-gray-400">{formatBytes(att.file.size)}</span>
                                        </div>
                                        <button
                                            onClick={() => removeAttachment(i)}
                                            className="ml-1 p-0.5 text-gray-400 hover:text-red-500 rounded-full hover:bg-gray-100 transition-colors cursor-pointer"
                                        >
                                            <X className="w-3 h-3" />
                                        </button>
                                    </div>
                                ))}
                            </ScrollArea>
                        )}

                        {/* Progress Bar overlay */}
                        {isSending && uploadProgress > 0 && (
                            <div className="h-1 w-full bg-gray-100 relative overflow-hidden">
                                <div
                                    className="absolute top-0 left-0 bottom-0 bg-blue-500 transition-all duration-300 ease-out"
                                    style={{ width: `${uploadProgress}%` }}
                                />
                            </div>
                        )}
                    </div>

                    {/* Footer */}
                    <div className="flex items-center gap-2 px-3 py-2.5 border-t border-gray-100 dark:border-gray-800 bg-gray-50/60 dark:bg-gray-900/60">
                        <Button
                            onClick={handleSend}
                            disabled={isSending || isProcessingFiles}
                            className="bg-blue-600 hover:bg-blue-700 text-white rounded-[7px] px-5 h-9 text-sm font-medium shadow-sm transition-all active:scale-[0.97]"
                        >
                            {isSending ? (
                                <Loader2 className="w-4 h-4 animate-spin mr-1.5" />
                            ) : (
                                <Send className="w-4 h-4 mr-1.5" />
                            )}
                            Send
                        </Button>

                        <div className="relative">
                            <input
                                type="file"
                                id="attachment-input"
                                multiple
                                className="hidden"
                                onChange={handleFileChange}
                                disabled={isSending || isProcessingFiles}
                            />
                            <Label
                                htmlFor="attachment-input"
                                className={cn(
                                    "flex items-center justify-center p-2 rounded-[5px] text-gray-400 hover:text-gray-600 hover:bg-gray-200/80 cursor-pointer transition-colors",
                                    (isSending || isProcessingFiles) && "opacity-50 cursor-not-allowed"
                                )}
                            >
                                {isProcessingFiles ? (
                                    <Loader2 className="w-4 h-4 animate-spin" />
                                ) : (
                                    <Paperclip className="w-4 h-4" />
                                )}
                            </Label>
                        </div>

                        <div className="flex-1" />

                        <Button
                            variant="ghost"
                            size="sm"
                            className="text-gray-400 hover:text-gray-600 h-8 text-xs"
                            onClick={handleSaveDraft}
                        >
                            Save draft
                        </Button>

                        <button
                            onClick={onClose}
                            className="p-1.5 hover:bg-gray-200/80 rounded-[5px] text-gray-400 transition-colors"
                        >
                            <Trash2 className="w-4 h-4" />
                        </button>
                    </div>
                </>
            )}
        </div>
    );
}
