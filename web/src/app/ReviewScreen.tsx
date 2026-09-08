/**
 * The review queue. Keyboard-first, one counterparty group at a time.
 *
 * `DESIGN.md` §8: digits pick a category, `Enter` approves the whole group, `T`
 * marks an internal transfer, `N` marks it out of the P&L, arrows move, `Esc`
 * clears. The legend is **always visible**, never behind a help icon — a queue
 * worked by keyboard whose keys are hidden is a queue nobody works twice.
 *
 * A group, not a row: deciding that VEKTOR LOGISTIKA is Logistics decides it for
 * every row that will ever carry that name, and the backend writes vendor memory
 * so the next import does not ask again.
 *
 * 36px rows rather than the 32px of the tables (§5). This screen takes keyboard
 * focus and its rows are targets.
 */
import { useCallback, useEffect, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";

import { decideGroup, listCategories, listReviewGroups } from "../data/review";
import type { ReviewDecision } from "../data/review";
import { NO_DATA, exponentOf, formatMinorUnits } from "../money";
import { t } from "../i18n";
import type { Locale } from "../i18n";
import { useLocale } from "../ui/preferences";
import { Kbd } from "../ui/Kbd";
import { useVirtualRows } from "../ui/useVirtualRows";
import { EmptyState, ErrorState, Loading } from "../ui/feedback";

const ROW = 36;

function fmt(m: { minorUnits: string; currencyCode: string }, locale: Locale): string {
  const e = exponentOf(m.currencyCode);
  return e === undefined ? NO_DATA : formatMinorUnits(m.minorUnits, e, locale);
}

function Legend({ locale }: { locale: Locale }) {
  const items = [
    [<Kbd key="d">1</Kbd>, "review.key.digit"],
    [<Kbd key="e">↵</Kbd>, "review.key.enter"],
    [<Kbd key="t">T</Kbd>, "review.key.transfer"],
    [<Kbd key="n">N</Kbd>, "review.key.notPnl"],
    [
      <span key="m" className="inline-flex gap-1">
        <Kbd>↑</Kbd>
        <Kbd>↓</Kbd>
      </span>,
      "review.key.move",
    ],
    [<Kbd key="c">Esc</Kbd>, "review.key.clear"],
  ] as const;

  return (
    <div className="flex flex-wrap items-center gap-x-5 gap-y-2 border-t border-border pt-3">
      <span className="text-2xs font-semibold uppercase tracking-widest text-text">
        {t("review.legend", locale)}
      </span>
      {items.map(([cap, key]) => (
        <span key={key} className="flex items-center gap-1.5 text-2xs text-text-muted">
          {cap}
          {t(key, locale)}
        </span>
      ))}
    </div>
  );
}

export function ReviewScreen() {
  const [locale] = useLocale();
  const qc = useQueryClient();
  const [index, setIndex] = useState(0);
  const [selected, setSelected] = useState<string | null>(null);
  const parentRef = useRef<HTMLDivElement>(null);

  const groups = useQuery({ queryKey: ["reviewGroups"], queryFn: listReviewGroups });
  const categories = useQuery({ queryKey: ["categories"], queryFn: listCategories });

  const list = groups.data ?? [];
  const group = list[Math.min(index, Math.max(list.length - 1, 0))];

  const decide = useCallback(
    async (decision: ReviewDecision) => {
      if (!group) return;
      await decideGroup({ counterpartyKey: group.counterpartyKey, decision });
      setSelected(null);
      setIndex(0);
      await qc.invalidateQueries({ queryKey: ["reviewGroups"] });
      // The rail badge counts the same list, so it has to be refreshed with it.
      await qc.invalidateQueries({ queryKey: ["reviewSummary"] });
    },
    [group, qc],
  );

  // Bound to the document rather than to a focused element: the queue is the
  // screen, and requiring a click to "focus" it first would make a
  // keyboard-first screen start with the pointer.
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.metaKey || e.ctrlKey || e.altKey) return;
      const cats = categories.data ?? [];

      if (/^[1-9]$/.test(e.key)) {
        const cat = cats[Number(e.key) - 1];
        if (cat) {
          setSelected(cat.id);
          e.preventDefault();
        }
        return;
      }
      switch (e.key) {
        case "Enter":
          if (selected) void decide({ kind: "classify", categoryId: selected });
          e.preventDefault();
          break;
        case "t":
        case "T":
          void decide({ kind: "internal_transfer" });
          e.preventDefault();
          break;
        case "n":
        case "N":
          void decide({ kind: "not_in_pnl" });
          e.preventDefault();
          break;
        case "ArrowDown":
          setIndex((i) => Math.min(i + 1, Math.max(list.length - 1, 0)));
          e.preventDefault();
          break;
        case "ArrowUp":
          setIndex((i) => Math.max(i - 1, 0));
          e.preventDefault();
          break;
        case "Escape":
          setSelected(null);
          e.preventDefault();
          break;
      }
    }
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [categories.data, selected, decide, list.length]);

  const rows = group?.transactions ?? [];
  const virtual = useVirtualRows({ count: rows.length, parentRef, rowHeight: ROW });

  if (groups.isPending) return <Loading label={t("review.title", locale)} rows={5} />;
  if (groups.error) return <ErrorState message={String(groups.error)} />;
  if (!group) {
    return (
      <EmptyState
        title={t("empty.review.title", locale)}
        detail={t("empty.review.detail", locale)}
      />
    );
  }

  return (
    <div className="flex flex-col gap-4">
      <header className="flex flex-wrap items-baseline gap-x-4 gap-y-1">
        <h1 className="text-lg font-semibold tracking-tight">{t("review.title", locale)}</h1>
        <span className="tabular text-2xs uppercase tracking-widest text-text-subtle">
          {index + 1} {t("review.groupOf", locale)} {list.length}
        </span>
      </header>

      <div className="flex flex-wrap items-baseline gap-x-4 gap-y-1 border-t border-border pt-4">
        <h2 className="text-md font-semibold">{group.counterpartyLabel}</h2>
        <span className="tabular text-md text-figure">{fmt(group.total, locale)}</span>
        {group.suggestedCategoryId ? (
          <span className="text-2xs uppercase tracking-widest text-text-subtle">
            {t("review.suggested", locale)} · {group.suggestedCategoryId}
            {group.suggestedConfidence
              ? ` · ${Math.round(group.suggestedConfidence * 100)}%`
              : ""}
          </span>
        ) : null}
      </div>

      <ol className="flex flex-wrap gap-x-4 gap-y-2">
        {(categories.data ?? []).map((c, i) => (
          <li key={c.id} className="flex items-center gap-1.5">
            <Kbd>{i + 1}</Kbd>
            <span
              className={
                selected === c.id
                  ? "border-b border-text pb-px text-sm font-medium text-text"
                  : "text-sm text-text-muted"
              }
            >
              {c.label}
            </span>
          </li>
        ))}
      </ol>

      <section>
        <h3 className="text-2xs font-semibold uppercase tracking-widest text-text">
          {t("review.transactions", locale)}
        </h3>
        <div ref={parentRef} className="mt-2 max-h-96 overflow-y-auto">
          <div style={{ height: virtual.getTotalSize(), position: "relative" }}>
            {virtual.getVirtualItems().map((item) => {
              const row = rows[item.index]!;
              return (
                <div
                  key={row.id}
                  className="absolute inset-x-0 flex items-center gap-3 border-b border-border text-sm"
                  style={{ height: ROW, transform: `translateY(${item.start}px)` }}
                >
                  <span className="tabular w-24 shrink-0 text-text-muted">{row.bookedOn}</span>
                  <span className="min-w-0 flex-1 truncate">{row.description}</span>
                  <span className="tabular w-28 shrink-0 text-right text-figure">
                    {fmt(row.amount, locale)}
                  </span>
                </div>
              );
            })}
          </div>
        </div>
      </section>

      <Legend locale={locale} />
    </div>
  );
}
