"use client";

import { useEffect, useState } from "react";
import { X } from "lucide-react";
import { Ad, getAdsByPlacement, recordAdImpression, recordAdClick } from "@/lib/api";

interface WebsiteAdSlotProps {
  placement: string;
  className?: string;
  /** If true, renders as an overlay/popup with a dismiss button */
  popup?: boolean;
}

export default function WebsiteAdSlot({ placement, className = "", popup = false }: WebsiteAdSlotProps) {
  const [ads, setAds] = useState<Ad[]>([]);
  const [dismissed, setDismissed] = useState(false);
  const [current, setCurrent] = useState(0);

  useEffect(() => {
    getAdsByPlacement(placement).then((data) => {
      setAds(data);
    });
  }, [placement]);

  useEffect(() => {
    if (ads.length === 0) return;
    const ad = ads[current];
    recordAdImpression(ad.id).catch(() => {});
  }, [ads, current]);

  if (ads.length === 0 || dismissed) return null;

  const ad = ads[current % ads.length];

  const handleClick = () => {
    recordAdClick(ad.id).catch(() => {});
    window.open(ad.target_url, "_blank", "noopener,noreferrer");
  };

  if (popup) {
    return (
      <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm">
        <div className="relative max-w-lg w-full mx-4 bg-brand-card rounded-xl overflow-hidden shadow-2xl border border-brand-border">
          <button
            onClick={() => setDismissed(true)}
            className="absolute top-2 right-2 z-10 p-1 rounded-full bg-black/50 text-white hover:bg-black/80 transition"
            aria-label="Close ad"
          >
            <X size={16} />
          </button>
          <AdContent ad={ad} onClick={handleClick} />
          {ads.length > 1 && (
            <div className="flex justify-center gap-1 pb-3">
              {ads.map((_, i) => (
                <button
                  key={i}
                  onClick={() => setCurrent(i)}
                  className={`w-2 h-2 rounded-full transition-colors ${i === current % ads.length ? "bg-brand-red" : "bg-brand-border"}`}
                />
              ))}
            </div>
          )}
        </div>
      </div>
    );
  }

  return (
    <div className={`relative ${className}`}>
      <div
        className="cursor-pointer rounded-lg overflow-hidden border border-brand-border hover:border-brand-red/50 transition-colors"
        onClick={handleClick}
      >
        <AdContent ad={ad} onClick={handleClick} />
      </div>
      <span className="absolute top-1 right-1 text-[10px] text-gray-500 bg-black/60 px-1 rounded">
        Ad
      </span>
    </div>
  );
}

function AdContent({ ad, onClick }: { ad: Ad; onClick: () => void }) {
  if (ad.image_url) {
    return (
      // eslint-disable-next-line @next/next/no-img-element
      <img
        src={ad.image_url}
        alt={ad.title}
        className="w-full h-auto object-cover"
        onClick={onClick}
      />
    );
  }

  // Text-only fallback
  return (
    <div
      className="p-4 bg-brand-card flex flex-col gap-2"
      onClick={onClick}
    >
      <p className="text-white font-semibold text-sm">{ad.title}</p>
      {ad.description && (
        <p className="text-gray-400 text-xs">{ad.description}</p>
      )}
      {ad.call_to_action && (
        <span className="inline-block mt-1 px-3 py-1 bg-brand-red text-white text-xs rounded-full w-fit">
          {ad.call_to_action}
        </span>
      )}
    </div>
  );
}
