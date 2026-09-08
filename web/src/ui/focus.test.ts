import { readFileSync, readdirSync, statSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

/** Anchored to this file, not the working directory. */
const web = (...p: string[]) => join(dirname(fileURLToPath(import.meta.url)), "..", "..", ...p);
import { describe, expect, it } from "vitest";

/**
 * The keyboard floor, `docs/DESIGN.md` §9.
 *
 * jsdom applies no stylesheet, so asserting "a focus ring is painted" would
 * assert nothing. What is testable is the two ways the floor actually breaks:
 * the global rule going missing, and a component switching it off locally --
 * which is what `focus:outline-none` is for, and which is in almost every
 * component snippet on the internet.
 *
 * The review queue is keyboard-first. A hidden ring makes it unusable, and the
 * damage is invisible to anyone testing with a mouse.
 */
function sources(dir: string): string[] {
  return readdirSync(dir).flatMap((entry) => {
    const path = join(dir, entry);
    if (statSync(path).isDirectory()) {
      if (entry === "gen" || entry === "mock") return [];
      return sources(path);
    }
    return /\.tsx?$/.test(entry) && !/\.test\.tsx?$/.test(entry) ? [path] : [];
  });
}

describe("focus is always visible", () => {
  it("defines one global focus-visible rule in the token layer", () => {
    const css = readFileSync(web("src", "index.css"), "utf8");
    expect(css).toMatch(/:focus-visible\s*\{/);
    expect(css).toMatch(/outline:\s*2px solid var\(--color-focus\)/);
    // The offset is what keeps an ink ring legible on an ink button.
    expect(css).toMatch(/outline-offset:\s*1px/);
  });

  it("has no component switching the ring off", () => {
    const offenders = sources(web("src")).filter((f) => {
      const body = readFileSync(f, "utf8");
      return /outline-none|outline:\s*none|outline:\s*0/.test(body);
    });
    expect(offenders).toEqual([]);
  });
});
