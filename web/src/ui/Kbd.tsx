/**
 * A key cap.
 *
 * **Not Unlumen.** §7.2 named its `Kbd`, and pulling a dependency for a bordered
 * span is not a trade worth making — the same reasoning as `site/Reveal.tsx`.
 * What matters is that the legend is *there*: `DESIGN.md` §8 requires it visible
 * rather than behind a help control, because a queue worked by keyboard whose
 * keys are hidden is a queue nobody works twice.
 */
import type { ReactNode } from "react";

export function Kbd({ children }: { children: ReactNode }) {
  return (
    <kbd className="inline-flex h-5 min-w-5 items-center justify-center border border-border-strong px-1 font-mono text-2xs text-text">
      {children}
    </kbd>
  );
}
