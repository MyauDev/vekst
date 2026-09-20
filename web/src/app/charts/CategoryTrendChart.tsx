/**
 * Category trend — small multiples, one line each. `WORKFLOW.md` §5.3, task
 * 8.12. Built on the vendored `@bklit/line-chart`: bklit has no small-
 * multiples primitive, so this is a CSS grid of small `LineChart` instances,
 * one per category.
 *
 * One hue plus de-emphasis grey for context (§13.4): small multiples are an
 * all-pairs comparison, which caps distinguishable series at three by colour
 * alone, so every facet uses the same hue and identity comes from the
 * facet's own heading instead. The de-emphasis line is each category's own
 * average across the range — the context a reader actually wants ("is this
 * month high or low for this line"), not a second series.
 *
 * No axis at all, not even the locale-safe label row `RevenueExpenseChart`
 * builds: eight facets this small have no room for one, and "beats an
 * 8-line spaghetti chart" (§5.3) is a shape-recognition job, not a
 * read-the-exact-value one — that is what the table view and each facet's
 * own tooltip are for.
 */
import { useMemo } from "react";
import { LineChart } from "@/components/charts/line-chart";
import { Line } from "@/components/charts/line";
import { ChartTooltip } from "@/components/charts/tooltip";

import { exponentOf, formatMinorUnits } from "../../money";
import type { Locale } from "../../i18n";
import type { Report } from "../../data/report";
import { ChartShell } from "./ChartShell";
import { PeriodTable } from "./ChartTable";
import { chartsAnimate } from "./support";

const MAX_FACETS = 8;

interface Facet {
  categoryId: string;
  label: string;
  rows: { period: string; value: number }[];
  texts: readonly string[];
}

function facets(report: Report, locale: Locale): Facet[] {
  const exp = exponentOf(report.currencyCode);
  return report.lines
    .map((l) => ({
      categoryId: l.categoryId,
      label: l.label,
      totalAbs: exp === undefined ? 0 : Math.abs(Number(BigInt(l.total.minorUnits))),
      rows: report.periods.map((period, i) => {
        const m = l.values[i];
        const value = m && exp !== undefined ? Math.abs(Number(BigInt(m.minorUnits)) / 10 ** exp) : 0;
        return { period, value };
      }),
      texts: l.values.map((v) => (exp !== undefined ? formatMinorUnits(v.minorUnits, exp, locale) : "—")),
    }))
    .sort((a, b) => b.totalAbs - a.totalAbs)
    .slice(0, MAX_FACETS);
}

function Facet({ facet }: Readonly<{ facet: Facet }>) {
  const avg = facet.rows.reduce((s, r) => s + r.value, 0) / (facet.rows.length || 1);
  const rowsWithAvg = facet.rows.map((r) => ({ ...r, avg }));

  return (
    <div>
      <p className="text-2xs font-semibold text-text-subtle">{facet.label}</p>
      <LineChart data={rowsWithAvg} xDataKey="period" margin={{ top: 8, right: 4, bottom: 4, left: 4 }} aspectRatio="2.4 / 1">
        <Line dataKey="value" stroke="var(--chart-1)" strokeWidth={2} animate={chartsAnimate()} showMarkers={false} />
        <Line dataKey="avg" stroke="var(--vk-chart-deemph)" strokeWidth={1} animate={false} showMarkers={false} showHighlight={false} />
        <ChartTooltip
          showDatePill={false}
          rows={(point) => [
            {
              color: "var(--chart-1)",
              label: facet.label,
              value: facet.texts[facet.rows.findIndex((r) => r.period === point["period"])] ?? "",
            },
          ]}
        />
      </LineChart>
    </div>
  );
}

export function CategoryTrendChart({
  report,
  locale,
}: Readonly<{ report: Report; locale: Locale }>) {
  const data = useMemo(() => facets(report, locale), [report, locale]);

  return (
    <ChartShell
      titleKey="chart.trend.title"
      locale={locale}
      hasData={data.length > 0}
      table={
        <PeriodTable
          periods={report.periods}
          series={data.map((f) => ({ label: f.label, values: f.texts }))}
          locale={locale}
        />
      }
      chart={
        <div className="grid grid-cols-2 gap-x-6 gap-y-4 sm:grid-cols-4">
          {data.map((f) => (
            <Facet key={f.categoryId} facet={f} />
          ))}
        </div>
      }
    />
  );
}
