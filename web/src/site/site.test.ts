import { readFileSync, readdirSync, statSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const src = (...p: string[]) => join(dirname(fileURLToPath(import.meta.url)), "..", ...p);

function filesUnder(dir: string): string[] {
  return readdirSync(dir).flatMap((e) => {
    const p = join(dir, e);
    if (statSync(p).isDirectory()) return filesUnder(p);
    return /\.tsx?$/.test(e) && !/\.test\.tsx?$/.test(e) ? [p] : [];
  });
}

describe("the public surface stays separable", () => {
  const site = filesUnder(src("site"));

  it("reads no application state", () => {
    // add-web-experience §5.8. `ARCHITECTURE.md` §8 schedules a static
    // marketing bundle at /site for Commercial. A landing page that loads a
    // screen's data or resolves a session cannot be lifted into one -- it has
    // to be rewritten, and by then nobody remembers why it was cheap.
    expect(site.length).toBeGreaterThan(3);
    const offenders = site.filter((f) => {
      const body = readFileSync(f, "utf8");
      return (
        /from "@connectrpc/.test(body) ||
        /\/gen\//.test(body) ||
        /testTransport|useQuery|IdentityService/.test(body)
      );
    });
    expect(offenders).toEqual([]);
  });

  it("imports the data layer only as types, which are erased at build", () => {
    // Sharing the product's components is the point -- §5.4 renders the real
    // table rather than a screenshot. Sharing its *state* is what breaks the
    // extraction. A type import carries neither.
    const offenders = site.filter((f) => {
      const body = readFileSync(f, "utf8");
      const valueImports = [...body.matchAll(/^import\s+(?!type\b)[^;]*?from\s+"([^"]+)"/gms)];
      return valueImports.some(([, spec]) => spec!.includes("/data/"));
    });
    expect(offenders).toEqual([]);
  });
});

describe("register discipline", () => {
  it("keeps the landing's display size out of the application", () => {
    // add-web-experience §5.9, amended 2026-09-16. DESIGN.md §1 used to cap the
    // app at 24px and this test forbade all three sizes above it. §1 now lets
    // the application reach 36px for a screen heading, so 2xl and 3xl are
    // legal behind sign-in.
    //
    // 4xl is not, and the guard survives for that: 48px is Register A's
    // display size, sized for one promise on an otherwise empty page. On a
    // screen whose job is many ordered figures there is nothing it can be
    // the right size for.
    const app = filesUnder(src("app"));
    expect(app.length).toBeGreaterThan(5);
    const offenders = app.filter((f) => /\btext-4xl\b/.test(readFileSync(f, "utf8")));
    expect(offenders).toEqual([]);
  });
});
