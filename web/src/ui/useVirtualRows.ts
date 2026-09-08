/**
 * `useVirtualizer`, with a viewport to fall back on.
 *
 * The library measures the scroll element and renders the rows that intersect
 * it. Before layout there is no measurement, so it renders **nothing** — a blank
 * list on first paint in a browser, and a permanently blank list in jsdom, which
 * never lays anything out. That is how two windowed lists in this codebase
 * reached review with passing tests and zero rendered rows (design D14).
 *
 * `initialRect` does not fix it: it applies only until the element is measured,
 * and a measurement of 0×0 wins immediately. So this substitutes the fallback
 * whenever the measured height is zero, which is exactly the case that means
 * "not laid out yet" rather than "genuinely empty".
 *
 * Fixing it here rather than mocking `getBoundingClientRect` in the test setup
 * is deliberate: a shim would make the tests pass against the shim.
 */
import { useVirtualizer } from "@tanstack/react-virtual";
import type { RefObject } from "react";

const FALLBACK = { width: 900, height: 480 };

export function useVirtualRows({
  count,
  parentRef,
  rowHeight,
}: {
  count: number;
  parentRef: RefObject<HTMLElement | null>;
  rowHeight: number;
}) {
  return useVirtualizer({
    count,
    getScrollElement: () => parentRef.current,
    estimateSize: () => rowHeight,
    overscan: 10,
    initialRect: FALLBACK,
    observeElementRect: (instance, cb) => {
      const el = instance.scrollElement;
      if (!el) return;
      const report = () => {
        const rect = el.getBoundingClientRect();
        cb({
          width: rect.width || FALLBACK.width,
          height: rect.height || FALLBACK.height,
        });
      };
      report();
      if (typeof ResizeObserver === "undefined") return;
      const ro = new ResizeObserver(report);
      ro.observe(el);
      return () => ro.disconnect();
    },
  });
}
