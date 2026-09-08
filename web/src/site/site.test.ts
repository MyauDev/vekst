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
  it("keeps Register A type sizes out of the application", () => {
    // add-web-experience §5.9. DESIGN.md §1 caps the app at 24px. The three
    // sizes above it belong to the landing, and behind sign-in they cost rows
    // per screen -- §1.1, the failure mode this whole design guards against.
    const app = filesUnder(src("app"));
    expect(app.length).toBeGreaterThan(5);
    const offenders = app.filter((f) =>
      /\btext-(2xl|3xl|4xl)\b/.test(readFileSync(f, "utf8")),
    );
    expect(offenders).toEqual([]);
  });
});
