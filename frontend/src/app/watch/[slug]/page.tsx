import { permanentRedirect } from "next/navigation";

// The standalone watch page has been merged into the movie detail page —
// the player now opens inline at /movies/[slug]. This route is kept only to
// 308-redirect old/shared links (and auto-open the player via ?play=1) so
// nothing breaks for bookmarks, Telegram shares, or indexed URLs.
interface PageProps {
  params: { slug: string };
}

export default function WatchPage({ params }: PageProps) {
  permanentRedirect(`/movies/${params.slug}?play=1`);
}
