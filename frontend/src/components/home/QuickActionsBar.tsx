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
    <section className="max-w-7xl mx-auto px-4 mt-8">
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
        {ACTIONS.map(({ href, label, icon: Icon, accent }) => (
          <Link
            key={href}
            href={href}
            className="glass glass-hover group flex items-center gap-3 px-4 py-3.5 rounded-2xl text-white text-sm font-medium"
          >
            <span className="glass-pill inline-flex h-9 w-9 items-center justify-center rounded-xl">
              <Icon size={18} className={accent} aria-hidden="true" />
            </span>
            <span className="tracking-tight">{label}</span>
          </Link>
        ))}
      </div>
    </section>
  );
}
