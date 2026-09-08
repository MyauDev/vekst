/**
 * The import list. One row per file, with the state, the source and the counts
 * that `DESIGN.md` §2 says a minimal pass may never remove.
 */
import { Link } from "@tanstack/react-router";
import { useQuery, useQueryClient } from "@tanstack/react-query";

import { listBatches } from "../data/imports";
import type { Batch } from "../data/imports";
import { t } from "../i18n";
import type { Locale } from "../i18n";
import { formatPeriodShort } from "../ui/period";
import { useLocale } from "../ui/preferences";
import { BatchStateChip } from "../ui/StateChip";
import { EmptyState, ErrorState, Loading } from "../ui/feedback";
import { Upload } from "./Upload";

function Period({ batch, locale }: Readonly<{ batch: Batch; locale: Locale }>) {
  if (!batch.periodFrom || !batch.periodTo) return <span className="text-text-subtle">—</span>;
  const from = formatPeriodShort(batch.periodFrom, locale);
  const to = formatPeriodShort(batch.periodTo, locale);
  return <span className="tabular">{from === to ? from : `${from} – ${to}`}</span>;
}

export function ImportsScreen() {
  const [locale] = useLocale();
  const qc = useQueryClient();
  const { data, error, isPending } = useQuery({
    queryKey: ["batches"],
    queryFn: listBatches,
  });

  return (
    <div className="flex flex-col gap-5">
      <h1 className="text-lg font-semibold tracking-tight">{t("imports.title", locale)}</h1>

      <Upload locale={locale} onUploaded={() => void qc.invalidateQueries({ queryKey: ["batches"] })} />

      {isPending ? <Loading label={t("imports.title", locale)} rows={4} /> : null}
      {error ? <ErrorState message={String(error)} /> : null}
      {data?.length === 0 ? (
        <EmptyState
          title={t("empty.imports.title", locale)}
          detail={t("empty.imports.detail", locale)}
        />
      ) : null}

      {data && data.length > 0 ? (
        <table className="w-full border-collapse text-sm">
          <thead>
            <tr className="h-8">
              {(["file", "source", "state", "period", "uploaded"] as const).map((c) => (
                <th
                  key={c}
                  scope="col"
                  className="border-b border-border-strong px-3 text-left text-2xs font-medium uppercase tracking-widest text-text-subtle first:pl-0"
                >
                  {t(`imports.col.${c}`, locale)}
                </th>
              ))}
              <th className="border-b border-border-strong px-3 text-right text-2xs font-medium uppercase tracking-widest text-text-subtle">
                {t("imports.counts.imported", locale)}
              </th>
            </tr>
          </thead>
          <tbody>
            {data.map((b) => (
              <tr key={b.id} className="h-8 border-b border-border">
                <td className="px-3 pl-0">
                  <Link
                    to="/app/imports/$batchId"
                    params={{ batchId: b.id }}
                    className="underline-offset-2 hover:underline"
                  >
                    {b.fileName}
                  </Link>
                </td>
                <td className="px-3 text-2xs uppercase tracking-wider text-text-muted">
                  {t(`imports.source.${b.sourceKind}`, locale)}
                </td>
                <td className="px-3">
                  <BatchStateChip state={b.state} locale={locale} />
                </td>
                <td className="px-3">
                  <Period batch={b} locale={locale} />
                </td>
                <td className="tabular px-3 text-xs text-text-muted">
                  {b.uploadedAt.slice(0, 10)}
                </td>
                <td className="tabular px-3 text-right">
                  {b.counts ? b.counts.rowsImported : <span className="text-text-subtle">—</span>}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      ) : null}
    </div>
  );
}
