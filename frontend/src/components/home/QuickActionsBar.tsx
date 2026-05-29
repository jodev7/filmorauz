import Link from "next/link";
import { Sparkles, Film, Layers, LayoutGrid } from "lucide-react";

// Section-level entry points shown directly under the hero. Distinct from the
// genre chips (which filter by genre) — these jump to whole sections of the
// site and give the homepage a clear "where do I go next" affordance.
const ACTIONS = [
  { href: "/premium", label: "Premium", icon: Sparkles, accent: "text-yellow-500" },
  { href: "/series", label: "Seriallar", icon: Film, accent: "text-orange-500" },
  { href: "/collections", label: "To'plamlar", icon: Layers, accent: "text-orange-500" },
  { href: "/genres", label: "Barcha janrlar", icon: LayoutGrid, accent: "text-orange-500" },
];

export default function QuickActionsBar() {
  return (
    <section className="max-w-7xl mx-auto px-4 mt-6">
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-2.5">
        {ACTIONS.map(({ href, label, icon: Icon, accent }) => (
          <Link
            key={href}
            href={href}
            className="flex items-center gap-2.5 px-4 py-3 bg-[#12121a] border border-[#1e1e2e] rounded-xl text-white text-sm font-medium hover:border-orange-500/50 hover:bg-[#161620] transition-colors"
          >
            <Icon size={18} className={accent} aria-hidden="true" />
            {label}
          </Link>
        ))}
      </div>
    </section>
  );
}
