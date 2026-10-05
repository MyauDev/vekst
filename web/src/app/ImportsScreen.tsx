/**
 * The import list. One row per file, with the state, the source and the counts
 * that `DESIGN.md` §2 says a minimal pass may never remove.
 */
import { Link } from "@tanstack/react-router";
import { useQuery, useQueryClient } from "@tanstack/react-query";

import { listBatches } from "../data/imports";
import { t } from "../i18n";
import { useLocale } from "../ui/preferences";
import { BatchStateChip } from "../ui/StateChip";
import { EmptyState, ErrorState, Loading } from "../ui/feedback";
import { Upload } from "./Upload";

export function ImportsScreen() {
  const [locale] = useLocale();
  const qc = useQueryClient();
  const { data, error, isPending } = useQuery({
    queryKey: ["batches"],
    queryFn: listBatches,
  });

  return (
    <div className="flex flex-col gap-6">
      <h1 className="text-2xl font-semibold tracking-tight">{t("imports.title", locale)}</h1>

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
        // The card is the wrapper rather than the table itself: `border-collapse`
        // and `border-radius` do not coexist -- the corners get clipped away.
        <div className="rounded-panel border border-border bg-surface-raised p-6">
          <table className="w-full border-collapse text-sm">
            <thead>
              <tr className="h-10">
                {(["file", "source", "state", "uploaded"] as const).map((c) => (
                  <th
                    key={c}
                    scope="col"
                    className="border-b border-border-strong px-3 text-left text-2xs font-medium uppercase tracking-widest text-text-subtle first:pl-0"
                  >
                    {t(`imports.col.${c}`, locale)}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {data.map((b) => (
                <tr key={b.id} className="h-10 border-b border-border">
                  <td className="px-3 pl-0">
                    <Link
                      to="/app/imports/$batchId"
                      params={{ batchId: b.id }}
                      className="underline-offset-2 hover:underline active:text-text-muted"
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
                  <td className="tabular px-3 text-xs text-text-muted">
                    {b.uploadedAt.slice(0, 10)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}
    </div>
  );
}
