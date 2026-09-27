import Link from "next/link";
import { personPath } from "@/lib/api";

const GRADIENTS = [
  "from-orange-500 to-rose-600",
  "from-sky-500 to-indigo-600",
  "from-emerald-500 to-teal-700",
  "from-fuchsia-500 to-purple-700",
  "from-amber-500 to-orange-700",
];

function initials(name: string): string {
  return name
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((p) => p[0]?.toUpperCase() ?? "")
    .join("");
}

/** Round photo (or initials) avatar + name linking to the person's page. */
export default function PersonChip({ name, role, photo }: { name: string; role: string; photo?: string }) {
  let h = 0;
  for (const c of name) h = (h * 31 + c.charCodeAt(0)) >>> 0;
  return (
    <Link href={personPath(name)} className="group flex w-24 flex-col items-center text-center">
      <span
        className={`relative flex h-16 w-16 items-center justify-center overflow-hidden rounded-full bg-gradient-to-br ${GRADIENTS[h % GRADIENTS.length]} text-lg font-bold text-white ring-2 ring-transparent transition group-hover:ring-orange-400`}
        aria-hidden="true"
      >
        {initials(name)}
        {photo && (
          // eslint-disable-next-line @next/next/no-img-element
          <img src={photo} alt="" loading="lazy" decoding="async" className="absolute inset-0 h-full w-full object-cover" />
        )}
      </span>
      <span className="mt-2 line-clamp-2 text-xs font-medium leading-tight text-gray-200 group-hover:text-white">{name}</span>
      <span className="line-clamp-1 text-[11px] text-gray-500" title={role}>{role}</span>
    </Link>
  );
}
