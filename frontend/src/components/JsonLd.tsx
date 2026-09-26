// Server-rendered JSON-LD. Unlike next/script (which injects after
// hydration), this lands in the initial HTML so every crawler sees it.
// "<" is escaped so content like "</script>" in a title can't break out.
export default function JsonLd({ data }: { data: unknown }) {
  const json = JSON.stringify(data).replace(/</g, "\\u003c");
  return <script type="application/ld+json" dangerouslySetInnerHTML={{ __html: json }} />;
}
