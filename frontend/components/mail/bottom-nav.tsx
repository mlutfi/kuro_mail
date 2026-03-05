"use client";
import { PenSquare } from "lucide-react";

interface BottomNavProps {
    onCompose: () => void;
}

export default function BottomNav({ onCompose }: BottomNavProps) {
    return (
        <div className="lg:hidden fixed bottom-6 right-5 z-50">
            <button
                onClick={onCompose}
                className="flex items-center gap-3 bg-blue-600/90 backdrop-blur-md text-white px-4 py-3 rounded-[8px] shadow-[0_8px_30px_rgb(37,99,235,0.25)] hover:bg-blue-600/80 active:scale-95 transition-all duration-300 border border-white/20 cursor-pointer"
            >
                <PenSquare className="w-5 h-5" strokeWidth={2.2} />
                <span className="font-semibold text-[15px] tracking-wide">Compose</span>
            </button>
        </div>
    );
}
