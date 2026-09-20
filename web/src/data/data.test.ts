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

describe("the seam the backend arrives through", () => {
  // add-web-experience §3.3. The boundary is deliberately drawn at *screens*
  // rather than at every file: AppLayout does import a transport and the
  // generated IdentityService, because resolving the session is authentication
  // rather than screen data, and Identity is one of the two services that
  // actually has a proto today. A screen is what must stay ignorant, because a
  // screen is what the real backend will change under.
  it("has no screen importing a transport or a generated client", () => {
    const screens = [
      ...filesUnder(src("app")).filter((f) => /Screen\.tsx$/.test(f)),
      ...filesUnder(src("site")),
    ];
    expect(screens.length).toBeGreaterThan(3);

    const offenders = screens.filter((f) => {
      const body = readFileSync(f, "utf8");
      return /from "@connectrpc|from "\.\..*\/gen\/|testTransport/.test(body);
    });
    expect(offenders).toEqual([]);
  });

  it("keeps fixtures out of components", () => {
    // A component holding sample data is a component that has to be rewritten
    // when the backend lands, which is the thing this layer exists to prevent.
    //
    // Scoped to `.tsx`: a `.ts` module beside a component is a data module, and
    // `site/sample.ts` is a legitimate one -- the landing needs figures of its
    // own so `site/` stays self-contained for the Commercial extraction.
    const components = [...filesUnder(src("app")), ...filesUnder(src("site")), ...filesUnder(src("ui"))]
      .filter((f) => f.endsWith(".tsx"));
    const offenders = components.filter((f) => /minorUnits:\s*"/.test(readFileSync(f, "utf8")));
    expect(offenders).toEqual([]);
  });
});

// "fixtures compute rather than assert" and "shapes the screens depend on"
// used to live here, exercising report.ts's fixture functions directly.
// report.ts is real now (§8.8); the equivalent coverage -- lines add up from
// their own periods, the reconciliation strip closes from its own parts, a
// mixed-basis entity is refused, a computed line opens onto its operands --
// moved to report.test.tsx and report.test.ts, against a mocked transport,
// the same relocation imports.ts's and review.ts's own fixture tests
// already went through.
