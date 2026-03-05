"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import { useAuth } from "@/lib/auth-context";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Mail, Lock, Shield, Loader2 } from "lucide-react";
import { toast } from "sonner";

export default function LoginPage() {
    const router = useRouter();
    const { login, verify2FA, isAuthenticated } = useAuth();

    const [step, setStep] = useState<"login" | "2fa">("login");
    const [email, setEmail] = useState(process.env.NEXT_PUBLIC_DEFAULT_USER || "");
    const [password, setPassword] = useState(process.env.NEXT_PUBLIC_DEFAULT_PASS || "");
    const [tempToken, setTempToken] = useState("");
    const [code, setCode] = useState("");
    const [trustDevice, setTrustDevice] = useState(false);
    const [isSubmitting, setIsSubmitting] = useState(false);

    // Redirect if already authenticated
    useEffect(() => {
        if (isAuthenticated) {
            router.push("/mail/inbox");
        }
    }, [isAuthenticated, router]);

    if (isAuthenticated) {
        return null;
    }

    const handleLogin = async (e: React.FormEvent) => {
        e.preventDefault();
        setIsSubmitting(true);

        try {
            const result = await login(email, password);
            if (result.requiresTwoFA) {
                setTempToken(result.tempToken || "");
                setStep("2fa");
                toast.info("2FA Required", { description: "Enter your authentication code" });
            } else {
                toast.success("Welcome back!");
                router.push("/mail/inbox");
            }
        } catch (err) {
            toast.error("Login Failed", {
                description: err instanceof Error ? err.message : "Invalid credentials",
            });
        } finally {
            setIsSubmitting(false);
        }
    };

    const handleVerify2FA = async (e: React.FormEvent) => {
        e.preventDefault();
        setIsSubmitting(true);

        try {
            await verify2FA(tempToken, code, trustDevice);
            toast.success("Welcome back!");
            router.push("/mail/inbox");
        } catch (err) {
            toast.error("Verification Failed", {
                description: err instanceof Error ? err.message : "Invalid code",
            });
        } finally {
            setIsSubmitting(false);
        }
    };

    return (
        <div className="min-h-screen flex items-center justify-center bg-gradient-to-br from-gray-50 via-blue-50/50 to-indigo-50 p-4">
            {/* Background decoration */}
            <div className="absolute inset-0 overflow-hidden pointer-events-none">
                <div className="absolute -top-40 -right-40 w-96 h-96 bg-blue-400/8 rounded-full blur-3xl" />
                <div className="absolute -bottom-40 -left-40 w-96 h-96 bg-indigo-400/8 rounded-full blur-3xl" />
                <div className="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 w-[600px] h-[600px] bg-blue-500/3 rounded-full blur-3xl" />
            </div>

            <Card className="w-full max-w-[400px] relative shadow-xl shadow-gray-200/50 border-0 bg-white/85 backdrop-blur-xl rounded-[14px] animate-in fade-in slide-in-from-bottom-4 duration-500">
                <CardHeader className="text-center space-y-4 pb-2 pt-8">
                    {/* Logo */}
                    <div className="mx-auto w-14 h-14 bg-blue-600 rounded-[12px] flex items-center justify-center shadow-lg shadow-blue-600/25">
                        <Mail className="w-7 h-7 text-white" />
                    </div>
                    <div>
                        <CardTitle className="text-xl tracking-tight dark:text-white"><span className="font-normal">Kuro</span><span className="font-bold">Mail</span></CardTitle>
                        <CardDescription className="mt-1 text-[13px]">
                            {step === "login"
                                ? "Sign in to your email account"
                                : "Enter your authentication code"}
                        </CardDescription>
                    </div>
                </CardHeader>

                <CardContent className="pt-4 pb-8 px-7">
                    {step === "login" ? (
                        <form onSubmit={handleLogin} className="space-y-4">
                            <div className="space-y-1.5">
                                <Label htmlFor="email" className="text-[13px]">Email</Label>
                                <div className="relative">
                                    <Mail className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-gray-400" />
                                    <Input
                                        id="email"
                                        type="email"
                                        placeholder="you@example.com"
                                        value={email}
                                        onChange={(e) => setEmail(e.target.value)}
                                        className="pl-10 h-10 rounded-[7px] text-[13px] border-gray-200 focus:border-blue-300"
                                        autoComplete="email"
                                        required
                                    />
                                </div>
                            </div>

                            <div className="space-y-1.5">
                                <Label htmlFor="password" className="text-[13px]">Password</Label>
                                <div className="relative">
                                    <Lock className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-gray-400" />
                                    <Input
                                        id="password"
                                        type="password"
                                        placeholder="••••••••"
                                        value={password}
                                        onChange={(e) => setPassword(e.target.value)}
                                        className="pl-10 h-10 rounded-[7px] text-[13px] border-gray-200 focus:border-blue-300"
                                        autoComplete="current-password"
                                        required
                                        minLength={6}
                                    />
                                </div>
                            </div>

                            <Button
                                type="submit"
                                className="w-full h-10 bg-blue-600 hover:bg-blue-700 text-white font-medium rounded-[7px] shadow-md shadow-blue-600/20 transition-all active:scale-[0.98] text-[13px]"
                                disabled={isSubmitting}
                            >
                                {isSubmitting ? (
                                    <>
                                        <Loader2 className="w-4 h-4 mr-2 animate-spin" />
                                        Signing in...
                                    </>
                                ) : (
                                    "Sign In"
                                )}
                            </Button>
                        </form>
                    ) : (
                        <form onSubmit={handleVerify2FA} className="space-y-4">
                            <div className="mx-auto w-12 h-12 bg-amber-50 rounded-full flex items-center justify-center mb-2">
                                <Shield className="w-6 h-6 text-amber-600" />
                            </div>

                            <div className="space-y-1.5">
                                <Label htmlFor="code" className="text-[13px]">Authentication Code</Label>
                                <Input
                                    id="code"
                                    type="text"
                                    placeholder="000000"
                                    value={code}
                                    onChange={(e) => setCode(e.target.value.replace(/\D/g, "").slice(0, 8))}
                                    className="h-10 text-center text-lg tracking-widest font-mono rounded-[7px] border-gray-200"
                                    autoComplete="one-time-code"
                                    required
                                    minLength={6}
                                    maxLength={8}
                                    autoFocus
                                />
                            </div>

                            <div className="flex items-center space-x-2">
                                <input
                                    id="trust"
                                    type="checkbox"
                                    checked={trustDevice}
                                    onChange={(e) => setTrustDevice(e.target.checked)}
                                    className="rounded border-gray-300"
                                />
                                <Label htmlFor="trust" className="text-[13px] text-muted-foreground font-normal cursor-pointer">
                                    Trust this device for 30 days
                                </Label>
                            </div>

                            <Button
                                type="submit"
                                className="w-full h-10 bg-blue-600 hover:bg-blue-700 text-white font-medium rounded-[7px] text-[13px]"
                                disabled={isSubmitting}
                            >
                                {isSubmitting ? (
                                    <>
                                        <Loader2 className="w-4 h-4 mr-2 animate-spin" />
                                        Verifying...
                                    </>
                                ) : (
                                    "Verify"
                                )}
                            </Button>

                            <Button
                                type="button"
                                variant="ghost"
                                className="w-full text-[13px] rounded-[7px]"
                                onClick={() => {
                                    setStep("login");
                                    setCode("");
                                }}
                            >
                                Back to login
                            </Button>
                        </form>
                    )}
                </CardContent>
            </Card>
        </div>
    );
}
