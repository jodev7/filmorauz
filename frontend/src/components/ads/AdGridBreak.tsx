"use client";

import WebsiteAdSlot from "@/components/ads/WebsiteAdSlot";

/**
 * Full-width inline ad row inside a poster grid. Renders nothing at all when
 * there is no ad, so the grid keeps its normal rhythm.
 */
export default function AdGridBreak({ placement = "grid_inline_block" }: { placement?: string }) {
  return <WebsiteAdSlot placement={placement} variant="inline" className="col-span-full" hideEmpty />;
}
