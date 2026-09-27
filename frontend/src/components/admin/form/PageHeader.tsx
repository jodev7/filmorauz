import Link from "next/link";
import { ChevronLeft } from "lucide-react";
import { ReactNode } from "react";

/** Title row shared by admin create/edit pages. */
export default function AdminPageHeader({
  backHref,
  backLabel,
  title,
  subtitle,
  badges,
  actions,
}: {
  backHref: string;
  backLabel: string;
  title: string;
  subtitle?: ReactNode;
  badges?: ReactNode;
  actions?: ReactNode;
}) {
  return (
    <header className="mb-6">
      <Link href={backHref} className="mb-3 inline-flex items-center gap-1 text-sm text-gray-500 transition-colors hover:text-white">
        <ChevronLeft size={16} /> {backLabel}
      </Link>
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <h1 className="flex flex-wrap items-center gap-2 text-2xl font-bold text-white">
            <span className="truncate">{title}</span>
            {badges}
          </h1>
          {subtitle && <div className="mt-1 text-sm text-gray-500">{subtitle}</div>}
        </div>
        {actions && <div className="flex flex-wrap items-center gap-2">{actions}</div>}
      </div>
    </header>
  );
}
