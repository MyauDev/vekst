/**
 * One import batch: what happened to it, and why.
 *
 * A rejected file persists **nothing** -- validation is blocking and atomic --
 * so this screen is the only record of what was wrong with it, and the error
 * list is the whole content rather than a detail.
 *
 * **Errors are keyed by the line number in the original file**, never the
 * parsed row index. That is an invariant in `CLAUDE.md`, and the reason is
 * practical: a person fixing the file opens it in a spreadsheet and goes to a
 * line. A parsed index names no line they can see.
 */
import { useParams } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";

import { getBatch } from "../data/imports";
import type { BatchDetail, ValidationError } from "../data/imports";
import { NO_DATA, exponentOf, formatMinorUnits } from "../money";
import { t } from "../i18n";
import type { Locale, MessageKey } from "../i18n";
import { useLocale } from "../ui/preferences";
import { BatchStateChip } from "../ui/StateChip";
import { EmptyState, ErrorState, Loading } from "../ui/feedback";

function fmt(m: { minorUnits: string; currencyCode: string }, locale: Locale): string {
  const e = exponentOf(m.currencyCode);
  return e === undefined ? NO_DATA : formatMinorUnits(m.minorUnits, e, locale);
}

/** The list is downloadable because a long one is fixed in a text editor beside
 *  the file, not by scrolling a browser. */
function download(batch: BatchDetail, locale: Locale) {
  const header = `${t("batch.errors.line", locale)},code,detail\n`;
  const body = batch.errors
    .map((e) => `${e.fileLine},${e.code},"${(e.detail ?? "").replace(/"/g, '""')}"`)
    .join("\n");
  const url = URL.createObjectURL(new Blob([header + body], { type: "text/csv" }));
  const a = document.createElement("a");
  a.href = url;
  a.download = `${batch.fileName}.errors.csv`;
  a.click();
  URL.revokeObjectURL(url);
}

/**
 * The error list. Capped, not windowed.
 *
 * A rejected file can carry one error per line, and the first instinct was to
 * virtualise. That is the wrong tool here: nobody fixes five thousand errors by
 * scrolling a box. They open the file and go to a line, which is why the list is
 * keyed by the original line number and why the download beside it is the real
 * answer at that size. Rendering a bounded head and saying so is both simpler
 * and more honest than a scroll region that implies the screen is the workspace.
 */
const SHOWN = 200;

function Errors({ errors, locale }: { errors: readonly ValidationError[]; locale: Locale }) {
  const shown = errors.slice(0, SHOWN);
  return (
    <div className="max-h-96 overflow-y-auto">
      {shown.map((e) => (
        <div
          key={`${e.fileLine}-${e.code}`}
          className="flex h-8 items-baseline gap-4 border-b border-border text-sm"
        >
          <span className="tabular w-16 shrink-0 text-right text-text-subtle">{e.fileLine}</span>
          <span className="min-w-0 flex-1">{t(`error.${e.code}` as MessageKey, locale)}</span>
          {e.detail ? (
            <span className="shrink-0 font-mono text-2xs text-text-muted">{e.detail}</span>
          ) : null}
        </div>
      ))}
      {errors.length > SHOWN ? (
        <p className="py-3 text-xs text-text-muted">
          {errors.length - SHOWN}+ — {t("batch.errors.download", locale)}
        </p>
      ) : null}
    </div>
  );
}

export function BatchScreen() {
  const { batchId } = useParams({ from: "/app/imports/$batchId" });
  const [locale] = useLocale();
  const { data, error, isPending } = useQuery({
    queryKey: ["batch", batchId],
    queryFn: () => getBatch(batchId),
  });

  if (isPending) return <Loading label={batchId} rows={4} />;
  if (error) return <ErrorState message={String(error)} />;
  if (!data) {
    return (
      <EmptyState title={t("empty.batch.title", locale)} detail={t("empty.batch.detail", locale)} />
    );
  }

  return (
    <div className="flex flex-col gap-5">
      <header className="flex flex-wrap items-baseline gap-x-4 gap-y-2">
        <h1 className="text-lg font-semibold tracking-tight">{data.fileName}</h1>
        <BatchStateChip state={data.state} locale={locale} />
        <span className="text-2xs uppercase tracking-widest text-text-subtle">
          {t(`imports.source.${data.sourceKind}`, locale)}
        </span>
      </header>

      {/* The outcome and its reason, at the top. A rejected batch that does not
          say why is a dead end. */}
      {data.rejectionReason ? (
        <p className="max-w-prose border-l-2 border-danger pl-4 text-sm text-text">
          {t(`batch.rejected.${data.rejectionReason}` as MessageKey, locale)}
        </p>
      ) : null}

      {data.counts ? (
        <dl className="flex flex-wrap gap-x-8 gap-y-4 border-t border-border pt-4">
          {(
            [
              ["imports.counts.imported", data.counts.rowsImported],
              ["imports.counts.duplicates", data.counts.duplicatesSkipped],
              ["imports.counts.transfers", data.counts.internalTransfersFound],
              ["imports.counts.matches", data.counts.matchesProposed],
            ] as const
          ).map(([key, value]) => (
            <div key={key} className="flex flex-col gap-0.5">
              <dt className="text-2xs uppercase tracking-widest text-text-subtle">
                {t(key, locale)}
              </dt>
              <dd className="tabular text-sm">{value}</dd>
            </div>
          ))}
        </dl>
      ) : null}

      {data.balanceCheck ? (
        <section className="border-t border-border pt-4">
          <h2 className="text-2xs font-semibold uppercase tracking-widest text-text">
            {t("batch.balance.title", locale)}
          </h2>
          <dl className="mt-3 flex flex-wrap gap-x-8 gap-y-4">
            {(
              [
                ["batch.balance.opening", data.balanceCheck.opening],
                ["batch.balance.movements", data.balanceCheck.movements],
                ["batch.balance.closing", data.balanceCheck.closing],
                ["batch.balance.declared", data.balanceCheck.declared],
              ] as const
            ).map(([key, value]) => (
              <div key={key} className="flex flex-col gap-0.5">
                <dt className="text-2xs uppercase tracking-widest text-text-subtle">
                  {t(key, locale)}
                </dt>
                <dd className="tabular text-sm">{fmt(value, locale)}</dd>
              </div>
            ))}
          </dl>
        </section>
      ) : null}

      {data.errors.length > 0 ? (
        <section className="border-t border-border pt-4">
          <div className="flex flex-wrap items-baseline gap-4">
            <h2 className="text-2xs font-semibold uppercase tracking-widest text-text">
              {t("batch.errors.title", locale)}
            </h2>
            <button
              type="button"
              onClick={() => download(data, locale)}
              className="border-b border-text pb-px text-2xs font-semibold uppercase tracking-widest text-text"
            >
              {t("batch.errors.download", locale)}
            </button>
            <span className="tabular ml-auto text-2xs text-text-subtle">
              {t("batch.errors.line", locale)}
            </span>
          </div>
          <div className="mt-3">
            <Errors errors={data.errors} locale={locale} />
          </div>
        </section>
      ) : null}
    </div>
  );
}
