import { readFileSync, readdirSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

import { foldTopN } from "./support";

const web = (...p: string[]) => join(dirname(fileURLToPath(import.meta.url)), "..", "..", "..", ...p);
const chartsDir = dirname(fileURLToPath(import.meta.url));

/** `web/src/index.css`'s `:root` or `[data-theme="dark"]` block, as text --
 *  jsdom applies no stylesheet (see `report.test.tsx`'s "both palettes"
 *  block), so a token's real value is only checkable by reading the source,
 *  the same technique the existing `@media print` test already uses. */
function themeBlock(css: string, selector: string): string {
  const start = css.indexOf(`${selector} {`);
  const end = css.indexOf("\n}", start);
  return css.slice(start, end);
}

describe("task 8.16 — dark is a chosen set of steps, not a filter", () => {
  const css = readFileSync(web("src", "index.css"), "utf8");
  const light = themeBlock(css, ":root");
  const dark = themeBlock(css, '[data-theme="dark"]');

  const SLOTS = ["--vk-series-1", "--vk-series-2", "--vk-series-3", "--vk-series-4",
    "--vk-series-5", "--vk-series-6", "--vk-series-7", "--vk-series-8",
    "--vk-diverge-pos-1", "--vk-diverge-pos-4", "--vk-diverge-neg-1", "--vk-diverge-neg-4",
    "--vk-chart-grid", "--vk-chart-axis", "--vk-chart-deemph"];

  it.each(SLOTS)("%s has an explicit hex in both palettes, and they differ", (name) => {
    const hex = /#[0-9a-fA-F]{6}/;
    const lightMatch = light.slice(light.indexOf(`${name}:`)).match(hex);
    const darkMatch = dark.slice(dark.indexOf(`${name}:`)).match(hex);
    expect(lightMatch, `${name} has no explicit hex in :root`).not.toBeNull();
    expect(darkMatch, `${name} has no explicit hex in [data-theme="dark"]`).not.toBeNull();
    // "Chosen steps, not a filter or an inversion" (DESIGN.md §13.2) means a
    // dark value that is its own decision, which the weakest possible check
    // is: it is not textually the same string as light's.
    expect(darkMatch![0]).not.toBe(lightMatch![0]);
  });

  it("neither block reaches for a CSS filter instead of a chosen value", () => {
    expect(dark).not.toMatch(/filter\s*:/);
    expect(light).not.toMatch(/filter\s*:/);
  });
});

describe("task 8.17 — a ninth category folds into Other", () => {
  const slice = (label: string, value: number) => ({ label, value, item: null });

  it("keeps every category under the limit untouched", () => {
    const items = [slice("a", 3), slice("b", 1), slice("c", 2)];
    const { kept, other } = foldTopN(items, 8, "Other");
    expect(kept.map((k) => k.label)).toEqual(["a", "c", "b"]); // sorted descending
    expect(other).toBeUndefined();
  });

  it("folds the tail of a ninth-and-beyond category into one Other", () => {
    const items = Array.from({ length: 10 }, (_, i) => slice(`cat-${i}`, 10 - i));
    const { kept, other } = foldTopN(items, 8, "Other");
    expect(kept).toHaveLength(8);
    // The two smallest (values 2 and 1) are what gets folded.
    expect(other).toEqual({ label: "Other", value: 3 });
  });

  it("never invents a colour for the fold -- callers get a label, not a slot", () => {
    const items = Array.from({ length: 9 }, (_, i) => slice(`cat-${i}`, i + 1));
    const { other } = foldTopN(items, 8, "Other");
    expect(other?.label).toBe("Other");
  });
});

describe("task 8.18 — no chart component references a raw series hex", () => {
  // check-web-tokens.sh already enforces this across web/src; this asserts it
  // for the charts specifically, the same regex, so the guard does not
  // silently depend on the script's scope never changing.
  const files = readdirSync(chartsDir)
    .filter((f) => /\.tsx?$/.test(f) && !/\.test\.tsx?$/.test(f))
    .map((f) => join(chartsDir, f));

  it.each(files.map((f) => [f.split("/").pop()!, f] as const))("%s has no raw hex", (_name, f) => {
    const body = readFileSync(f, "utf8");
    expect(body).not.toMatch(/#[0-9a-fA-F]{3,8}\b/);
  });
});

describe("WORKFLOW.md §5.3 acceptance scenarios", () => {
  it("the net-result diverging colours are bridged to the diverging palette, never danger", () => {
    // §6: an expense line is negative every month; a loss month is not an
    // error, and red is what marks a rejected import. NetResultChart no
    // longer names a --vk- token directly -- it reads the bklit-facing
    // aliases index.css defines for it -- so both ends of that indirection
    // are checked: the chart uses the aliases, and the aliases point at the
    // diverging palette rather than danger.
    const chart = readFileSync(join(chartsDir, "NetResultChart.tsx"), "utf8");
    expect(chart).toMatch(/--chart-diverge-positive/);
    expect(chart).toMatch(/--chart-diverge-negative/);

    const css = readFileSync(web("src", "index.css"), "utf8");
    const bridge = css.slice(css.indexOf("--chart-diverge-positive"));
    expect(bridge.slice(0, bridge.indexOf(";"))).toMatch(/--vk-diverge-pos-\d/);
    const negLine = bridge.slice(bridge.indexOf("--chart-diverge-negative"));
    expect(negLine.slice(0, negLine.indexOf(";"))).toMatch(/--vk-diverge-neg-\d/);
    expect(css).not.toMatch(/--chart-diverge-(positive|negative):\s*var\(--vk-danger/);
  });

  it("the two-series chart renders a legend", () => {
    // §13.5: a legend wherever two or more series appear. bklit ships no
    // legend primitive, so this is plain JSX rather than an option key --
    // data-chart-legend marks it for exactly this assertion.
    const body = readFileSync(join(chartsDir, "RevenueExpenseChart.tsx"), "utf8");
    expect(body).toMatch(/data-chart-legend/);
  });

  it("the single-series charts do not", () => {
    // Absence is the assertion here: a legend on a single series is the
    // "an icon beside every label" mistake in chart form.
    for (const file of ["ExpenseCategoriesChart.tsx", "NetResultChart.tsx", "CategoryTrendChart.tsx"]) {
      const body = readFileSync(join(chartsDir, file), "utf8");
      expect(body).not.toMatch(/data-chart-legend/);
    }
  });

  it("no single-value-scale chart introduces a second y-axis", () => {
    // §8.7. bklit's `yAxisId` prop is the only way to add a second scale to
    // one chart; its absence is the guarantee. CategoryTrendChart is exempt
    // -- it is `data.length` separate single-axis LineChart instances, not
    // one chart with several -- and MoneyFlowChart is a Sankey, which has no
    // y-axis concept at all.
    for (const file of ["ExpenseCategoriesChart.tsx", "NetResultChart.tsx", "RevenueExpenseChart.tsx"]) {
      const body = readFileSync(join(chartsDir, file), "utf8");
      expect(body, `${file} declares a yAxisId`).not.toMatch(/yAxisId/);
    }
  });
});
