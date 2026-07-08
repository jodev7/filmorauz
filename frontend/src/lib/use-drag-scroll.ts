import { useCallback, useRef } from "react";

// Mouse drag-to-scroll for horizontal carousels. Touch devices already scroll
// natively (overflow-x), so this only kicks in for mouse pointers — grab and
// drag to move the row on desktop. A movement threshold flags real drags so a
// drag that ends on a card doesn't fire the card's click/navigation.
export function useDragScroll(ref: React.RefObject<HTMLElement | null>) {
  const state = useRef({ down: false, startX: 0, scrollLeft: 0, moved: false });

  const onPointerDown = useCallback(
    (e: React.PointerEvent) => {
      if (e.pointerType !== "mouse") return; // touch → native scroll
      const el = ref.current;
      if (!el) return;
      state.current = {
        down: true,
        startX: e.clientX,
        scrollLeft: el.scrollLeft,
        moved: false,
      };
    },
    [ref],
  );

  const onPointerMove = useCallback(
    (e: React.PointerEvent) => {
      const el = ref.current;
      const s = state.current;
      if (!s.down || !el) return;
      const dx = e.clientX - s.startX;
      if (Math.abs(dx) > 4) s.moved = true;
      el.scrollLeft = s.scrollLeft - dx;
    },
    [ref],
  );

  const end = useCallback(() => {
    state.current.down = false;
  }, []);

  // Swallow the click that follows a real drag so cards don't navigate.
  const onClickCapture = useCallback((e: React.MouseEvent) => {
    if (state.current.moved) {
      e.preventDefault();
      e.stopPropagation();
      state.current.moved = false;
    }
  }, []);

  return {
    onPointerDown,
    onPointerMove,
    onPointerUp: end,
    onPointerLeave: end,
    onClickCapture,
  };
}
