interface LogoProps {
  /** Extra classes for sizing/weight, e.g. "text-lg sm:text-2xl". */
  className?: string;
}

// Brand wordmark: Filmora (white) · Uz (orange) · .net (white), set in
// Poppins. Rendered as inline spans so it composes inside links, headings,
// and the footer without pulling in an icon or its own layout.
export default function Logo({ className = "" }: LogoProps) {
  return (
    <span
      className={`font-poppins font-semibold tracking-tight leading-none text-white ${className}`}
    >
      Filmora<span className="text-orange-500">Uz</span>.net
    </span>
  );
}
