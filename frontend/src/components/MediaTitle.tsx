import { extractPlainTitle } from "@/lib/seo-template";

interface MediaTitleProps {
  /** The raw (possibly SEO-wrapped) title, e.g. "Avatar (2009) O'zbek tilida …". */
  title?: string | null;
}

// Renders the clean movie/series name for humans while keeping the full
// SEO title string in the DOM (visually + AT hidden) so search crawlers still
// index it. Returns a fragment so it drops straight into an existing heading.
export default function MediaTitle({ title }: MediaTitleProps) {
  const full = (title || "").trim();
  const plain = extractPlainTitle(full);
  return (
    <>
      {plain || full}
      {full && plain && full !== plain && (
        <span className="sr-only" aria-hidden="true">
          {" "}
          {full}
        </span>
      )}
    </>
  );
}
