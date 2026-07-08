import Link from "next/link";
import { ArrowRight, type LucideIcon } from "lucide-react";

interface SectionHeaderProps {
  title: string;
  icon: LucideIcon;
  href?: string;
  linkLabel?: string;
  /** Accent colour for the icon; falls back to the brand orange. */
  accent?: "orange" | "yellow";
}

// Apple-minimal section header: a soft glass icon chip, a clean weighted
// title (Inter, tight tracking — not the condensed display face), and a
// pill "see all" link. Shared by every row on the redesigned homepage so
// the whole page reads as one system.
export default function SectionHeader({
  title,
  icon: Icon,
  href,
  linkLabel = "Hammasi",
  accent = "orange",
}: SectionHeaderProps) {
  const accentColor = accent === "yellow" ? "text-yellow-400" : "text-orange-400";

  return (
    <div className="flex items-center justify-between mb-5">
      <h2 className="flex items-center gap-3">
        <span
          className={`glass-pill inline-flex h-9 w-9 items-center justify-center rounded-xl ${accentColor}`}
        >
          <Icon size={17} strokeWidth={2} aria-hidden="true" />
        </span>
        <span className="text-lg sm:text-xl font-semibold tracking-tight text-white">
          {title}
        </span>
      </h2>
      {href && (
        <Link
          href={href}
          className="glass-pill glass-hover group inline-flex items-center gap-1.5 rounded-full px-3.5 py-1.5 text-xs font-medium text-gray-300 hover:text-white"
        >
          {linkLabel}
          <ArrowRight
            size={13}
            className="transition-transform duration-300 group-hover:translate-x-0.5"
            aria-hidden="true"
          />
        </Link>
      )}
    </div>
  );
}
