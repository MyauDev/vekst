import { readFileSync, readdirSync, statSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

import { sumMinorUnits } from "../money";
import { getReport, getDrilldown } from "./report";
import { listBatches, getBatch } from "./imports";
import { listReviewGroups, reviewSummary } from "./review";

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

describe("fixtures compute rather than assert", () => {
  // add-web-experience §3.4. A typed-in total is a fixture that can disagree
  // with itself, and a report whose parts do not add up is the one defect this
  // product cannot ship.
  it("adds each line's total from its own periods", async () => {
    const report = await getReport({ from: "2026-01", to: "2026-08" });
    for (const section of report.sections) {
      for (const line of section.lines) {
        if (!line.total) continue;
        const fromValues = sumMinorUnits(
          line.values.filter((v) => v !== null).map((v) => v!.minorUnits),
        );
        expect(line.total.minorUnits, line.label).toBe(fromValues);
      }
    }
  });

  it("adds each section's subtotals from its lines", async () => {
    const report = await getReport({ from: "2026-01", to: "2026-08" });
    for (const section of report.sections) {
      section.subtotals.forEach((subtotal, i) => {
        const fromLines = sumMinorUnits(
          section.lines.flatMap((l) => (l.values[i] ? [l.values[i]!.minorUnits] : [])),
        );
        expect(subtotal.minorUnits, `${section.label} period ${i}`).toBe(fromLines);
      });
      expect(section.total.minorUnits).toBe(
        sumMinorUnits(section.subtotals.map((m) => m.minorUnits)),
      );
    }
  });

  it("adds the net result from every section", async () => {
    const report = await getReport({ from: "2026-01", to: "2026-08" });
    report.netByPeriod.forEach((net, i) => {
      expect(net.minorUnits).toBe(
        sumMinorUnits(report.sections.map((s) => s.subtotals[i]!.minorUnits)),
      );
    });
    expect(report.netTotal.minorUnits).toBe(
      sumMinorUnits(report.sections.map((s) => s.total.minorUnits)),
    );
  });

  it("closes the reconciliation strip from its own parts", async () => {
    const { reconciliation: r } = await getReport({ from: "2026-01", to: "2026-08" });
    // This is what proves to an accountant that nothing was dropped. A closing
    // figure that was typed in proves nothing.
    expect(r.closing.minorUnits).toBe(
      sumMinorUnits([
        r.opening.minorUnits, r.moneyIn.minorUnits,
        r.moneyOut.minorUnits, r.transfers.minorUnits,
      ]),
    );
  });

  it("sums the review total from its own transactions", async () => {
    for (const group of await listReviewGroups()) {
      expect(group.total.minorUnits).toBe(
        sumMinorUnits(group.transactions.map((t) => t.amount.minorUnits)),
      );
    }
    const summary = await reviewSummary();
    expect(summary.rows).toBe(
      (await listReviewGroups()).reduce((n, g) => n + g.transactions.length, 0),
    );
  });
});

describe("shapes the screens depend on", () => {
  it("blocks a line rather than computing it, and says why", async () => {
    const report = await getReport({ from: "2026-01", to: "2026-08" });
    const blocked = report.sections.flatMap((s) => s.lines).filter((l) => l.blockedReason);

    expect(blocked.length).toBeGreaterThan(0);
    for (const line of blocked) {
      expect(line.total).toBeNull();
      expect(line.values.every((v) => v === null)).toBe(true);
      // A code, not a sentence: the client owns every sentence.
      expect(line.blockedReason).toMatch(/^[a-z0-9_]+$/);
    }
  });

  it("distinguishes a true zero from no data", async () => {
    const report = await getReport({ from: "2026-01", to: "2026-08" });
    const fees = report.sections
      .flatMap((s) => s.lines)
      .find((l) => l.categoryId === "professional-fees")!;

    // Quarterly: the empty months are a real zero, not a gap.
    expect(fees.values.some((v) => v?.minorUnits === "0")).toBe(true);
    expect(fees.values.every((v) => v !== null)).toBe(true);
  });

  it("says when a figure's transactions are not in the fixture set", async () => {
    const populated = await getDrilldown({ categoryId: "logistics", period: "2026-03" });
    expect(populated.rows.length).toBeGreaterThan(0);
    expect(populated.rowsUnavailable).toBeFalsy();
    // Every other cell opens with its real figure and admits the list is absent
    // rather than inventing one.
    const empty = await getDrilldown({ categoryId: "payroll", period: "2026-05" });
    expect(empty.rowsUnavailable).toBe(true);
    expect(empty.amount.minorUnits).toBe("-2010000");
  });

  it("keys validation errors by the original file line", async () => {
    const rejected = await getBatch("b-ledger-h1");
    expect(rejected?.state).toBe("rejected");
    expect(rejected!.errors.length).toBeGreaterThan(0);
    for (const e of rejected!.errors) {
      expect(e.fileLine).toBeGreaterThan(0);
      expect(e.code).toMatch(/^[a-z0-9_]+$/);
    }
    // A rejected file persists nothing, so it carries no counts.
    expect(rejected!.counts).toBeUndefined();
    expect((await listBatches()).length).toBeGreaterThan(1);
  });
});
