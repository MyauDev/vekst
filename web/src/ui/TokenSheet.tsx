/**
 * The token sheet. Development only -- never on a rail, never in a production
 * build.
 *
 * It exists because `docs/DESIGN.md` §3, §9 and §13 assert values and ratios
 * that nothing else renders. Nine states, two complete palettes and eight chart
 * slots ship before any screen uses them, so without this page the first time
 * anyone sees the palette whole is the first time a customer does.
 *
 * It renders from the real tokens, so drift between the document and
 * `index.css` shows up as a wrong colour rather than a stale table.
 *
 * Laid out as a **type specimen**, not a dashboard: a narrow label column with
 * the material hanging off it, hairline rules instead of cards, and hard-edged
 * swatches butted into a band. `DESIGN.md` §1.1 -- buy white space by removing
 * chrome, not by adding padding.
 */
import { useLocale, useTheme, resolvedTheme } from "./preferences";
import { BatchStateChip, ReportStateChip, RowStateChip } from "./StateChip";

/** Label left, material right. The asymmetry is the layout. */
function Row({ label, note, children }: { label: string; note?: string; children: React.ReactNode }) {
  return (
    <section className="grid grid-cols-1 gap-x-6 gap-y-3 border-t border-border py-6 sm:grid-cols-[8rem_1fr]">
      <div className="flex flex-col gap-1">
        <h2 className="text-2xs font-semibold uppercase tracking-widest text-text">{label}</h2>
        {note ? <p className="text-2xs leading-relaxed text-text-subtle">{note}</p> : null}
      </div>
      <div className="min-w-0">{children}</div>
    </section>
  );
}

/** A continuous band, no gaps and no radius. A swatch with a rounded corner and
 *  a drop of air around it is a card; a specimen butts them so the steps read
 *  against each other. */
function Band({ items }: { items: [string, string][] }) {
  return (
    <div>
      <div className="flex h-14 overflow-hidden border border-border">
        {items.map(([cls]) => (
          <span key={cls} className={`flex-1 ${cls}`} />
        ))}
      </div>
      <div className="mt-1.5 flex">
        {items.map(([cls, name]) => (
          <span key={cls} className="flex-1 pr-2 text-2xs leading-tight text-text-muted">
            {name}
          </span>
        ))}
      </div>
    </div>
  );
}

const SURFACES: [string, string][] = [
  ["bg-surface", "surface"],
  ["bg-surface-raised", "raised"],
  ["bg-surface-sunken", "sunken"],
  ["bg-border", "border"],
  ["bg-border-strong", "border-strong"],
  ["bg-accent", "accent"],
];

const INKS: [string, string, string][] = [
  ["text-text", "text", "15.98 / 15.30"],
  ["text-text-muted", "text-muted", "6.35 / 7.58"],
  ["text-text-subtle", "text-subtle", "4.52 / 5.17"],
  ["text-ok", "ok", "6.61 / 6.98"],
  ["text-warn", "warn", "6.17 / 7.50"],
  ["text-danger", "danger", "5.74 / 4.92"],
];

const TYPE: [string, string, string][] = [
  ["text-4xl", "48", "Know the number"],
  ["text-3xl", "36", "Управленческая отчётность"],
  ["text-2xl", "30", "Management reporting"],
  ["text-xl", "24", "−6 812,40"],
  ["text-lg", "20", "Management P&L"],
  ["text-md", "16", "Отчёт о прибылях и убытках"],
  ["text-base", "14", "The default in the application"],
  ["text-sm", "13", "1 284 600,00"],
  ["text-xs", "12", "Dates, counts, column meta"],
  ["text-2xs", "11", "taxonomy v3 · ruleset v11"],
];

const SPACING: [string, string][] = [
  ["w-0.5", "2"],
  ["w-1", "4"],
  ["w-1.5", "6"],
  ["w-2", "8"],
  ["w-3", "12"],
  ["w-4", "16"],
  ["w-6", "24"],
  ["w-8", "32"],
];

const SERIES: [string, string][] = [
  ["bg-series-1", "blue"],
  ["bg-series-2", "orange"],
  ["bg-series-3", "aqua"],
  ["bg-series-4", "yellow"],
  ["bg-series-5", "magenta"],
  ["bg-series-6", "green"],
  ["bg-series-7", "violet"],
  ["bg-series-8", "red"],
  ["bg-series-other", "other"],
];

const DIVERGE: [string, string][] = [
  ["bg-diverge-pos-4", ""],
  ["bg-diverge-pos-3", ""],
  ["bg-diverge-pos-2", ""],
  ["bg-diverge-pos-1", ""],
  ["bg-diverge-mid", "zero"],
  ["bg-diverge-neg-1", ""],
  ["bg-diverge-neg-2", ""],
  ["bg-diverge-neg-3", ""],
  ["bg-diverge-neg-4", ""],
];

function Toggle<T extends string>({
  values,
  current,
  onPick,
}: {
  values: readonly T[];
  current: T;
  onPick: (v: T) => void;
}) {
  return (
    <div className="flex items-center gap-3">
      {values.map((v) => (
        <button
          key={v}
          type="button"
          onClick={() => onPick(v)}
          aria-pressed={current === v}
          className={
            current === v
              ? "border-b border-text pb-0.5 text-2xs font-semibold uppercase tracking-widest text-text"
              : "border-b border-transparent pb-0.5 text-2xs uppercase tracking-widest text-text-subtle"
          }
        >
          {v}
        </button>
      ))}
    </div>
  );
}

export function TokenSheet() {
  const [theme, setTheme] = useTheme();
  const [locale, setLocale] = useLocale();
  const shown = resolvedTheme(theme);

  return (
    <main className="mx-auto max-w-4xl px-8 py-8">
      {/* Masthead. A rule, not a card. */}
      <header className="flex flex-wrap items-end justify-between gap-4 border-b-2 border-text pb-3">
        <div>
          <h1 className="text-lg font-semibold tracking-tight">Token sheet</h1>
          <p className="mt-0.5 text-2xs uppercase tracking-widest text-text-subtle">
            Veekst · DESIGN.md §3 §9 §13 · from index.css
          </p>
        </div>
        <div className="flex items-end gap-6">
          <Toggle values={["light", "dark", "system"] as const} current={theme} onPick={setTheme} />
          <Toggle values={["en", "ru"] as const} current={locale} onPick={setLocale} />
        </div>
      </header>

      <p className="tabular py-3 text-2xs text-text-muted">
        Showing <span className="font-semibold text-text">{shown}</span>
        {theme === "system" ? " (following the system)" : ""}. Ratios are light / dark, computed
        from the oklch values.
      </p>

      <Row label="Surfaces" note="Hierarchy is hairlines and surface steps. Shadows are for overlays only — §5">
        <Band items={SURFACES} />
      </Row>

      <Row label="Ink" note="The three state colours are the only hue outside a chart — §6, §13.1">
        <dl className="grid grid-cols-2 gap-x-6 sm:grid-cols-3">
          {INKS.map(([cls, name, ratio]) => (
            <div key={name} className="flex items-baseline gap-2 border-b border-border py-2">
              <span className={`text-lg font-semibold ${cls}`}>Aa</span>
              <dt className="text-2xs text-text-muted">{name}</dt>
              <dd className="tabular ml-auto text-2xs text-text-subtle">{ratio}</dd>
            </div>
          ))}
        </dl>
      </Row>

      <Row
        label="States"
        note="Nine states, three axes. A mark and a word, never hue alone — §7"
      >
        <dl className="flex flex-col gap-2">
          {[
            ["Import batch", (["pending", "parsing", "rejected", "imported"] as const).map((s) => (
              <BatchStateChip key={s} state={s} locale={locale} />
            ))],
            ["Report readiness", (["ready", "partial", "blocked"] as const).map((s) => (
              <ReportStateChip key={s} state={s} locale={locale} />
            ))],
            ["Row classification", (["classified", "needsReview", "blocked"] as const).map((s) => (
              <RowStateChip key={s} state={s} locale={locale} />
            ))],
          ].map(([axis, chips], i) => (
            <div key={i} className="flex flex-wrap items-baseline gap-x-5 gap-y-2 border-b border-border pb-2">
              <dt className="w-32 shrink-0 text-2xs text-text-subtle">{axis as string}</dt>
              {chips as React.ReactNode}
            </div>
          ))}
        </dl>
      </Row>

      <Row label="Type" note="The app caps at 24. Above it is the landing, and must not appear behind sign-in — §1">
        <dl className="flex flex-col">
          {TYPE.map(([cls, px, sample]) => (
            <div key={cls} className="flex items-baseline gap-5 border-b border-border py-1.5">
              <dt className="tabular w-6 shrink-0 text-right text-2xs text-text-subtle">{px}</dt>
              <dd className={`${cls} min-w-0 truncate`}>{sample}</dd>
            </div>
          ))}
        </dl>
      </Row>

      <Row label="Spacing" note="Stops at 32 inside the app. check-web-tokens.sh fails the build above it — §5">
        <dl className="flex flex-col gap-1.5">
          {SPACING.map(([cls, px]) => (
            <div key={cls} className="flex items-center gap-4">
              <dt className="tabular w-6 shrink-0 text-right text-2xs text-text-subtle">{px}</dt>
              <dd className={`${cls} h-2 bg-text`} />
            </div>
          ))}
        </dl>
      </Row>

      <Row label="Series" note="Hue means identity only inside a chart's frame. Fixed order, never cycled — §13.1">
        <Band items={SERIES} />
      </Row>

      <Row label="Diverging" note="Blue ↔ orange. Never red for a loss: a loss is not an error — §6, §13.3">
        <div>
          <div className="flex h-14 overflow-hidden border border-border">
            {DIVERGE.map(([cls]) => (
              <span key={cls} className={`flex-1 ${cls}`} />
            ))}
          </div>
          <div className="mt-1.5 flex justify-between text-2xs text-text-muted">
            <span>positive</span>
            <span>zero</span>
            <span>negative</span>
          </div>
        </div>
      </Row>
    </main>
  );
}
