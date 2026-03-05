"use client";

import { useState, useEffect, useCallback } from "react";
import { useAuth } from "@/lib/auth-context";
import { apiGet, apiDelete } from "@/lib/api";
import { Session, TwoFASetupResponse } from "@/lib/types";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Separator } from "@/components/ui/separator";
import { Badge } from "@/components/ui/badge";
import {
    Shield,
    ShieldCheck,
    ShieldOff,
    Smartphone,
    Monitor,
    Loader2,
    Trash2,
    Copy,
    Check,
    QrCode,
    User,
    Mail,
} from "lucide-react";
import { toast } from "sonner";
import { format } from "date-fns";
import { ScrollArea } from "@/components/ui/scroll-area";

export default function SettingsPage() {
    const { user, setup2FA, enable2FA, disable2FA } = useAuth();
    const [sessions, setSessions] = useState<Session[]>([]);
    const [sessionsLoading, setSessionsLoading] = useState(true);

    // 2FA state
    const [twoFAStep, setTwoFAStep] = useState<"idle" | "setup" | "verify" | "backup">("idle");
    const [setupData, setSetupData] = useState<TwoFASetupResponse | null>(null);
    const [verifyCode, setVerifyCode] = useState("");
    const [disableCode, setDisableCode] = useState("");
    const [backupCodes, setBackupCodes] = useState<string[]>([]);
    const [isSubmitting, setIsSubmitting] = useState(false);
    const [copiedBackup, setCopiedBackup] = useState(false);

    const fetchSessions = useCallback(async () => {
        const res = await apiGet<Session[]>("/auth/sessions");
        if (res.success && res.data) {
            setSessions(res.data);
        }
        setSessionsLoading(false);
    }, []);

    useEffect(() => {
        const init = async () => {
            await fetchSessions();
        };
        init();
    }, [fetchSessions]);

    const handleSetup2FA = async () => {
        setIsSubmitting(true);
        const data = await setup2FA();
        if (data) {
            setSetupData(data);
            setTwoFAStep("setup");
        } else {
            toast.error("Failed to setup 2FA");
        }
        setIsSubmitting(false);
    };

    const handleEnable2FA = async () => {
        if (verifyCode.length < 6) return;
        setIsSubmitting(true);
        const codes = await enable2FA(verifyCode);
        if (codes) {
            setBackupCodes(codes);
            setTwoFAStep("backup");
            toast.success("2FA enabled successfully!");
        } else {
            toast.error("Invalid code. Please try again.");
        }
        setIsSubmitting(false);
    };

    const handleDisable2FA = async () => {
        if (disableCode.length < 6) return;
        setIsSubmitting(true);
        const success = await disable2FA(disableCode);
        if (success) {
            setTwoFAStep("idle");
            setDisableCode("");
            toast.success("2FA disabled");
        } else {
            toast.error("Invalid code");
        }
        setIsSubmitting(false);
    };

    const handleRevokeSession = async (sessionId: string) => {
        const res = await apiDelete(`/auth/sessions/${sessionId}`);
        if (res.success) {
            toast.success("Session revoked");
            fetchSessions();
        }
    };

    const copyBackupCodes = () => {
        navigator.clipboard.writeText(backupCodes.join("\n"));
        setCopiedBackup(true);
        setTimeout(() => setCopiedBackup(false), 2000);
        toast.success("Backup codes copied!");
    };

    return (
        <ScrollArea className="h-full w-full">
            <div className="max-w-3xl mx-auto p-6 space-y-6">
                <h1 className="text-2xl font-bold text-gray-900">Settings</h1>

                {/* Profile */}
                <Card className="mb-6">
                    <CardHeader>
                        <CardTitle className="flex items-center gap-2">
                            <User className="w-5 h-5" /> Profile
                        </CardTitle>
                    </CardHeader>
                    <CardContent className="space-y-4">
                        <div className="grid grid-cols-2 gap-4">
                            <div>
                                <Label className="text-xs text-gray-400">Display Name</Label>
                                <p className="text-sm font-medium">{user?.display_name || "—"}</p>
                            </div>
                            <div>
                                <Label className="text-xs text-gray-400">Email</Label>
                                <p className="text-sm font-medium flex items-center gap-1">
                                    <Mail className="w-3.5 h-3.5 text-gray-400" />
                                    {user?.email || "—"}
                                </p>
                            </div>
                        </div>
                        <div>
                            <Label className="text-xs text-gray-400">Signature</Label>
                            <p className="text-sm text-gray-600">{user?.signature || "No signature set"}</p>
                        </div>
                    </CardContent>
                </Card>

                {/* 2FA */}
                <Card className="mb-6">
                    <CardHeader>
                        <CardTitle className="flex items-center gap-2">
                            <Shield className="w-5 h-5" /> Two-Factor Authentication
                        </CardTitle>
                        <CardDescription>
                            Add an extra layer of security to your account
                        </CardDescription>
                    </CardHeader>
                    <CardContent>
                        {user?.totp_enabled ? (
                            <div className="space-y-4">
                                <div className="flex items-center gap-3 p-4 bg-green-50 rounded-xl">
                                    <ShieldCheck className="w-6 h-6 text-green-600" />
                                    <div>
                                        <p className="text-sm font-medium text-green-800">2FA is enabled</p>
                                        <p className="text-xs text-green-600">Your account is protected with TOTP authentication</p>
                                    </div>
                                </div>

                                <div className="space-y-2">
                                    <Label>Enter code to disable 2FA:</Label>
                                    <div className="flex gap-2">
                                        <Input
                                            value={disableCode}
                                            onChange={(e) => setDisableCode(e.target.value.replace(/\D/g, "").slice(0, 6))}
                                            placeholder="000000"
                                            className="w-40 text-center font-mono tracking-widest"
                                            maxLength={6}
                                        />
                                        <Button
                                            variant="destructive"
                                            onClick={handleDisable2FA}
                                            disabled={disableCode.length < 6 || isSubmitting}
                                        >
                                            {isSubmitting ? <Loader2 className="w-4 h-4 animate-spin" /> : <ShieldOff className="w-4 h-4 mr-1" />}
                                            Disable
                                        </Button>
                                    </div>
                                </div>
                            </div>
                        ) : twoFAStep === "idle" ? (
                            <div className="space-y-4">
                                <div className="flex items-center gap-3 p-4 bg-amber-50 rounded-xl">
                                    <ShieldOff className="w-6 h-6 text-amber-600" />
                                    <div>
                                        <p className="text-sm font-medium text-amber-800">2FA is not enabled</p>
                                        <p className="text-xs text-amber-600">We recommend enabling 2FA for better security</p>
                                    </div>
                                </div>
                                <Button onClick={handleSetup2FA} disabled={isSubmitting}>
                                    {isSubmitting ? <Loader2 className="w-4 h-4 animate-spin mr-2" /> : <QrCode className="w-4 h-4 mr-2" />}
                                    Enable 2FA
                                </Button>
                            </div>
                        ) : twoFAStep === "setup" && setupData ? (
                            <div className="space-y-4">
                                <p className="text-sm text-gray-600">
                                    Scan this QR code with your authenticator app (Google Authenticator, Authy, etc.)
                                </p>
                                <div className="flex justify-center p-4 bg-white border rounded-xl">
                                    {/* eslint-disable-next-line @next/next/no-img-element */}
                                    <img
                                        src={`data:image/png;base64,${setupData.qr_code_data}`}
                                        alt="2FA QR Code"
                                        className="w-72 h-72"
                                    />
                                </div>
                                <div className="text-center">
                                    <p className="text-xs text-gray-400 mb-1">Or enter manually:</p>
                                    <code className="text-sm bg-gray-100 px-3 py-1 rounded font-mono select-all">
                                        {setupData.secret}
                                    </code>
                                </div>
                                <Separator />
                                <div className="space-y-2">
                                    <Label>Enter the code from your app:</Label>
                                    <div className="flex gap-2">
                                        <Input
                                            value={verifyCode}
                                            onChange={(e) => setVerifyCode(e.target.value.replace(/\D/g, "").slice(0, 6))}
                                            placeholder="000000"
                                            className="w-40 text-center font-mono tracking-widest"
                                            maxLength={6}
                                            autoFocus
                                        />
                                        <Button onClick={handleEnable2FA} disabled={verifyCode.length < 6 || isSubmitting}>
                                            {isSubmitting ? <Loader2 className="w-4 h-4 animate-spin mr-1" /> : <Check className="w-4 h-4 mr-1" />}
                                            Verify & Enable
                                        </Button>
                                    </div>
                                </div>
                                <Button variant="ghost" onClick={() => setTwoFAStep("idle")}>Cancel</Button>
                            </div>
                        ) : twoFAStep === "backup" ? (
                            <div className="space-y-4">
                                <div className="p-4 bg-red-50 border border-red-200 rounded-xl">
                                    <p className="text-sm font-medium text-red-800 mb-2">⚠️ Save your backup codes!</p>
                                    <p className="text-xs text-red-600 mb-3">
                                        These codes will not be shown again. Save them in a secure location.
                                    </p>
                                    <div className="grid grid-cols-2 gap-2 mb-3">
                                        {backupCodes.map((code, i) => (
                                            <code key={i} className="text-sm bg-white px-3 py-1.5 rounded border font-mono text-center">
                                                {code}
                                            </code>
                                        ))}
                                    </div>
                                    <Button variant="outline" size="sm" onClick={copyBackupCodes}>
                                        {copiedBackup ? <Check className="w-4 h-4 mr-1" /> : <Copy className="w-4 h-4 mr-1" />}
                                        {copiedBackup ? "Copied!" : "Copy all codes"}
                                    </Button>
                                </div>
                                <Button onClick={() => setTwoFAStep("idle")}>Done</Button>
                            </div>
                        ) : null}
                    </CardContent>
                </Card>

                {/* Sessions */}
                <Card className="mb-6">
                    <CardHeader>
                        <CardTitle className="flex items-center gap-2">
                            <Smartphone className="w-5 h-5" /> Active Sessions
                        </CardTitle>
                        <CardDescription>
                            Manage your active sessions and devices
                        </CardDescription>
                    </CardHeader>
                    <CardContent>
                        {sessionsLoading ? (
                            <div className="flex justify-center py-8">
                                <Loader2 className="w-6 h-6 animate-spin text-gray-400" />
                            </div>
                        ) : sessions.length === 0 ? (
                            <p className="text-sm text-gray-400 text-center py-4">No active sessions</p>
                        ) : (
                            <div className="space-y-3">
                                {sessions.map((session) => (
                                    <div
                                        key={session.id}
                                        className="flex items-center gap-3 p-3 border rounded-xl hover:bg-gray-50 transition-colors"
                                    >
                                        <Monitor className="w-5 h-5 text-gray-400 shrink-0" />
                                        <div className="flex-1 min-w-0">
                                            <p className="text-sm font-medium truncate">
                                                {session.device_name || "Unknown device"}
                                            </p>
                                            <div className="flex items-center gap-2 text-xs text-gray-400">
                                                <span>{session.ip_address}</span>
                                                <span>·</span>
                                                <span>Last active {format(new Date(session.last_active_at), "MMM d, h:mm a")}</span>
                                                {session.is_trusted && (
                                                    <Badge variant="secondary" className="text-xs">Trusted</Badge>
                                                )}
                                            </div>
                                        </div>
                                        <Button
                                            variant="ghost"
                                            size="sm"
                                            className="text-red-400 hover:text-red-600 hover:bg-red-50 h-8 px-2"
                                            onClick={() => handleRevokeSession(session.id)}
                                        >
                                            <Trash2 className="w-4 h-4" />
                                        </Button>
                                    </div>
                                ))}
                            </div>
                        )}
                    </CardContent>
                </Card>
            </div>
        </ScrollArea>
    );
}
