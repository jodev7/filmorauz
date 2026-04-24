"use client";

import { useState } from "react";
import { Copy, Check } from "lucide-react";

interface MovieCodeProps {
  code: string;
  label?: string;
}

export default function MovieCode({ code, label = "🎟 Kino kodi:" }: MovieCodeProps) {
  const [copied, setCopied] = useState(false);

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(code);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch (err) {
      console.error("Failed to copy:", err);
    }
  };

  if (!code) return null;

  return (
    <div className="inline-flex items-center gap-2 bg-brand-card border border-brand-border rounded-lg px-3 py-2">
      <span className="text-sm text-gray-400">
        {label}
      </span>
      <span className="font-mono text-brand-red font-bold text-lg">
        {code}
      </span>
      <button
        onClick={handleCopy}
        className="p-1 hover:bg-brand-border rounded transition-colors"
        title={copied ? "Nusxa olindi!" : "Nusxalashtirish"}
      >
        {copied ? (
          <Check size={16} className="text-green-500" />
        ) : (
          <Copy size={16} className="text-gray-400 hover:text-white" />
        )}
      </button>
    </div>
  );
}
