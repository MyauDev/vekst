/**
 * A single entrance: content rises a few pixels and fades in as it arrives.
 *
 * **Not Unlumen.** §5.6 of the task list named it for the landing's motion, and
 * its catalogue is aurora washes, gooey filters, magnetic buttons and WebGL
 * backgrounds. Those are the shapes `DESIGN.md` §14 exists to keep out — they
 * are what a page reaches for when it has nothing to say. A product whose claim
 * is that the number is final should not shimmer.
 *
 * So the landing's motion is one restrained thing, written here in twelve
 * lines rather than pulled in as a dependency. Register A is allowed motion
 * (§1); this is what it should be spent on.
 *
 * `prefers-reduced-motion` is honoured globally by the token layer, which
 * collapses every transition to nothing.
 */
import { useEffect, useRef, useState } from "react";
import type { ReactNode } from "react";

export function Reveal({ children, delay = 0 }: { children: ReactNode; delay?: number }) {
  const ref = useRef<HTMLDivElement>(null);
  const [shown, setShown] = useState(false);

  useEffect(() => {
    const node = ref.current;
    // No IntersectionObserver (jsdom, and older engines): show it. Content that
    // depends on an observer to become visible is content that can vanish.
    if (!node || typeof IntersectionObserver === "undefined") {
      setShown(true);
      return;
    }
    const io = new IntersectionObserver(
      ([entry]) => {
        if (entry?.isIntersecting) {
          setShown(true);
          io.disconnect();
        }
      },
      { rootMargin: "-40px" },
    );
    io.observe(node);
    return () => io.disconnect();
  }, []);

  return (
    <div
      ref={ref}
      style={{
        opacity: shown ? 1 : 0,
        transform: shown ? "none" : "translateY(8px)",
        transition: `opacity var(--duration-slow) var(--ease-entrance) ${delay}ms, transform var(--duration-slow) var(--ease-entrance) ${delay}ms`,
      }}
    >
      {children}
    </div>
  );
}
