// Kept outside the "use client" AdGridBreak module so server-rendered pages
// can call it while mapping their grids.

/** Poster-grid position after which an ad row is inserted (and every N after). */
export const AD_GRID_EVERY = 12;

/** True when an ad row belongs right after the item at `index`. */
export function isAdGridBreak(index: number, total: number): boolean {
  return (index + 1) % AD_GRID_EVERY === 0 && index < total - 1;
}
