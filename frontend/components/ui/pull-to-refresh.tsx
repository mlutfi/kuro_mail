"use client";

import { useState, useRef, ReactNode } from "react";
import { Loader2 } from "lucide-react";

interface PullToRefreshProps {
    children: ReactNode;
    onRefresh: () => Promise<void> | void;
}

export function PullToRefresh({ children, onRefresh }: PullToRefreshProps) {
    const [startY, setStartY] = useState(0);
    const [pullY, setPullY] = useState(0);
    const [refreshing, setRefreshing] = useState(false);
    const containerRef = useRef<HTMLDivElement>(null);

    const handleTouchStart = (e: React.TouchEvent) => {
        if (containerRef.current && containerRef.current.scrollTop === 0) {
            setStartY(e.touches[0].clientY);
        } else {
            setStartY(0);
        }
    };

    const handleTouchMove = (e: React.TouchEvent) => {
        if (startY > 0 && !refreshing) {
            const y = e.touches[0].clientY;
            const diff = y - startY;
            if (diff > 0) {
                // Dampen the pull effect
                if (diff > 80) {
                    setPullY(80 + (diff - 80) * 0.2);
                } else {
                    setPullY(diff);
                }
            }
        }
    };

    const handleTouchEnd = async () => {
        if (pullY > 60 && !refreshing) {
            setRefreshing(true);
            setPullY(60);
            try {
                await onRefresh();
            } finally {
                setRefreshing(false);
                setPullY(0);
            }
        } else {
            setPullY(0);
        }
        setStartY(0);
    };

    return (
        <div
            ref={containerRef}
            className="flex-1 overflow-y-auto relative h-full w-full"
            onTouchStart={handleTouchStart}
            onTouchMove={handleTouchMove}
            onTouchEnd={handleTouchEnd}
        >
            <div
                className="absolute top-0 left-0 right-0 flex justify-center items-center overflow-hidden transition-all duration-200 z-10"
                style={{ height: `${refreshing ? 60 : pullY}px`, opacity: pullY > 10 ? 1 : 0 }}
            >
                <div
                    className={`bg-white rounded-full p-2 shadow-md flex items-center justify-center transition-transform duration-200 ${refreshing ? 'animate-spin' : ''}`}
                    style={{ transform: `rotate(${pullY * 3}deg)` }}
                >
                    <Loader2 className="w-5 h-5 text-blue-600" />
                </div>
            </div>
            <div className="transition-transform duration-200 h-full w-full" style={{ transform: `translateY(${refreshing ? 60 : pullY}px)` }}>
                {children}
            </div>
        </div>
    );
}
