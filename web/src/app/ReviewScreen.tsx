/**
 * The review queue. Keyboard-first, one counterparty group at a time -- and,
 * since the keyboard path is not the only path a person actually finds their
 * way in with, every one of its actions also has a clickable equivalent.
 *
 * `DESIGN.md` §8: digits pick a category, `Enter` approves the whole group, `T`
 * marks an internal transfer, `N` marks it out of the P&L, arrows move, `Esc`
 * clears. The legend is **always visible**, never behind a help icon — a queue
 * worked by keyboard whose keys are hidden is a queue nobody works twice. It now
 * sits right under the header rather than at the foot of the page, for the same
 * reason: an explanation nobody scrolls to might as well not exist.
 *
 * A group, not a row: deciding that VEKTOR LOGISTIKA is Logistics decides it for
 * every row that will ever carry that name, and the backend writes vendor memory
 * so the next import does not ask again. `review.blurb` says so in words, because
 * nothing else on this screen does -- the legend lists keys, not what pressing
 * one commits to.
 *
 * Only digits `1`-`9` can ever address a category directly, and an organisation's
 * taxonomy runs to dozens of leaves (the industry template alone seeds around
 * 60), so the earlier, keyboard-only version of this screen made every category
 * past the ninth unreachable by any means. The filter field narrows that list to
 * what a person actually typed, which also re-numbers `1`-`9` onto whatever
 * matched -- and a click reaches any of them regardless, filtered or not.
 *
 * 44px rows rather than the 40px of the tables (§5, as amended 2026-09-16).
 * This screen takes keyboard focus and its rows are targets, so it keeps the
 * one-step lead over the tables it has always had.
 */
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";

import { ResolveGroupError, decideGroup, listCategories, listGroupTransactions, listReviewGroups } from "../data/review";
import type { ReviewDecision } from "../data/review";
import { categoryName } from "./categoryName";
import { NO_DATA, exponentOf, formatMinorUnits } from "../money";
import { authErrorMessage, t } from "../i18n";
import type { Locale } from "../i18n";
import { useLocale } from "../ui/preferences";
import { Kbd } from "../ui/Kbd";
import { useVirtualRows } from "../ui/useVirtualRows";
import { EmptyState, ErrorState, Loading } from "../ui/feedback";

const ROW = 44;

const inputClass =
  "rounded border border-border-strong bg-surface px-2 py-1 text-sm text-text focus:border-text";

function fmt(m: { minorUnits: string; currencyCode: string }, locale: Locale): string {
  const e = exponentOf(m.currencyCode);
  return e === undefined ? NO_DATA : formatMinorUnits(m.minorUnits, e, locale);
}

function Legend({ locale }: Readonly<{ locale: Locale }>) {
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
    <div className="flex flex-wrap items-center gap-x-6 gap-y-3 rounded-panel border border-border bg-surface-raised p-6">
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
  const [query, setQuery] = useState("");
  // A decision that fails must say so: the backend returns a coded failure
  // (a race with another reviewer, a stale category), and a screen that
  // swallows it looks identical to one that did nothing at all.
  const [decideError, setDecideError] = useState<string | null>(null);
  const parentRef = useRef<HTMLDivElement>(null);

  const groups = useQuery({ queryKey: ["reviewGroups"], queryFn: listReviewGroups });
  const categories = useQuery({ queryKey: ["categories"], queryFn: listCategories });

  // Translated once per locale change, not per render of every button below:
  // `label` becomes what the picker, the search and the "Selected" line all
  // read, so a Russian reviewer can find "Аренда офиса" by typing "аренда"
  // rather than having to know the English name stored on the wire.
  const allCategories = useMemo(
    () => (categories.data ?? []).map((c) => ({ ...c, label: categoryName(c.code, c.label, locale) })),
    [categories.data, locale],
  );
  // Recomputed, not just filtered from a stale reference: this is also what
  // "1"-"9" address below, so it has to be exactly what is on screen right
  // now, search included.
  const visibleCategories = useMemo(() => {
    const needle = query.trim().toLowerCase();
    return needle ? allCategories.filter((c) => c.label.toLowerCase().includes(needle)) : allCategories;
  }, [allCategories, query]);
  const selectedCategory = selected ? allCategories.find((c) => c.code === selected) : undefined;

  const list = groups.data ?? [];
  const group = list[Math.min(index, Math.max(list.length - 1, 0))];

  // A group's rows are a separate call (`ListGroupTransactions`), not part of
  // `ListReviewGroups` -- fetched for whichever group is open, not for all of
  // them eagerly.
  const transactions = useQuery({
    queryKey: ["reviewGroupTransactions", group?.counterpartyKey],
    queryFn: () => listGroupTransactions(group!.counterpartyKey),
    enabled: !!group,
  });

  const decide = useCallback(
    async (decision: ReviewDecision) => {
      if (!group) return;
      setDecideError(null);
      try {
        await decideGroup({ counterpartyKey: group.counterpartyKey, decision });
      } catch (err) {
        setDecideError(
          err instanceof ResolveGroupError ? authErrorMessage(err.code, locale) : t("error.unknown", locale),
        );
        return;
      }
      setSelected(null);
      setIndex(0);
      await qc.invalidateQueries({ queryKey: ["reviewGroups"] });
      // The rail badge counts the same list, so it has to be refreshed with it.
      await qc.invalidateQueries({ queryKey: ["reviewSummary"] });
    },
    [group, qc, locale],
  );

  // Every per-group choice is scoped to the group it was made in. Without
  // this, moving to the next counterparty with the arrow keys (decide()'s own
  // reset only covers the decided group) carries the filter text and the
  // selected category along -- a stale filter hides categories that are very
  // much still there, which reads as "the picker lost most of the taxonomy",
  // and a stale selection risks approving the new group under the old one's
  // category.
  useEffect(() => {
    setDecideError(null);
    setQuery("");
    setSelected(null);
  }, [group?.counterpartyKey]);

  // Bound to the document rather than to a focused element: the queue is the
  // screen, and requiring a click to "focus" it first would make a
  // keyboard-first screen start with the pointer.
  //
  // The filter field below is the one place that changes this. No category
  // label in this taxonomy contains a digit, so "1"-"9" stay shortcuts even
  // while it holds focus -- filter, then press a digit, with no click and no
  // Tab in between. Enter has no native meaning in a single-line text input
  // either (nothing here submits a form), so it survives too: filter,
  // digit, Enter, done, hands never leave the keyboard. "t"/"n" do not --
  // "Training" and "Not in P&L" are both real category text a person may be
  // typing -- and neither do the arrow keys, which move the caret while a
  // field has focus rather than the selected group.
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.metaKey || e.ctrlKey || e.altKey) return;
      const typing = document.activeElement instanceof HTMLInputElement;

      if (/^[1-9]$/.test(e.key)) {
        const cat = visibleCategories[Number(e.key) - 1];
        if (cat) {
          setSelected(cat.code);
          e.preventDefault();
        }
        return;
      }
      if (typing && e.key !== "Enter") return;
      switch (e.key) {
        case "Enter":
          if (selected) void decide({ kind: "classify", categoryCode: selected });
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
  }, [visibleCategories, selected, decide, list.length]);

  const rows = transactions.data ?? [];
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
    <div className="flex flex-col gap-6">
      <header className="flex flex-wrap items-baseline gap-x-4 gap-y-2">
        <h1 className="text-2xl font-semibold tracking-tight">{t("review.title", locale)}</h1>
        <span className="tabular text-2xs uppercase tracking-widest text-text-subtle">
          {index + 1} {t("review.groupOf", locale)} {list.length}
        </span>
      </header>

      <p className="max-w-prose text-sm text-text-muted">{t("review.blurb", locale)}</p>

      {/* Right under the header, not at the foot of the page: an explanation
          nobody scrolls to might as well not exist (DESIGN.md §8). */}
      <Legend locale={locale} />

      <div className="flex flex-wrap items-baseline gap-x-4 gap-y-2 rounded-panel border border-border bg-surface-raised p-6">
        <h2 className="text-lg font-semibold tracking-tight">{group.counterpartyLabel}</h2>
        <span className="tabular text-lg text-figure">{fmt(group.total, locale)}</span>
      </div>

      <section className="flex flex-col gap-4 rounded-panel border border-border bg-surface-raised p-6">
        <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-2">
          <input
            type="text"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder={t("review.search.placeholder", locale)}
            aria-label={t("review.search.placeholder", locale)}
            className={inputClass}
          />
          {/* The underline-only choice a category makes when picked by digit
              is easy to miss; this says the same thing in words. */}
          <span className="text-sm text-text-muted">
            {selectedCategory ? (
              <>
                {t("review.selected", locale)}:{" "}
                <span className="font-medium text-text">{selectedCategory.label}</span>
              </>
            ) : (
              " "
            )}
          </span>
        </div>

        <ol className="flex flex-wrap gap-2">
          {visibleCategories.length === 0 ? (
            <li className="text-sm text-text-subtle">{t("review.search.empty", locale)}</li>
          ) : (
            visibleCategories.map((c, i) => (
              <li key={c.id}>
                <button
                  type="button"
                  onClick={() => setSelected(c.code)}
                  aria-pressed={selected === c.code}
                  className={
                    "flex items-center gap-1.5 rounded border px-2 py-1 text-sm transition-colors " +
                    (selected === c.code
                      ? "border-text bg-surface-sunken font-medium text-text"
                      : "border-transparent text-text-muted hover:border-border hover:bg-surface-sunken hover:text-text")
                  }
                >
                  {i < 9 ? <Kbd>{i + 1}</Kbd> : null}
                  {c.label}
                </button>
              </li>
            ))
          )}
        </ol>

        {decideError ? (
          <p role="alert" className="text-sm font-medium text-danger">
            {decideError}
          </p>
        ) : null}

        <div className="flex flex-wrap gap-3 border-t border-border pt-4">
          <button
            type="button"
            disabled={!selected}
            onClick={() => selected && void decide({ kind: "classify", categoryCode: selected })}
            className="rounded border border-text bg-text px-4 py-1.5 text-sm font-medium text-surface hover:bg-text-muted disabled:cursor-not-allowed disabled:opacity-40"
          >
            {t("review.action.approve", locale)}
          </button>
          <button
            type="button"
            onClick={() => void decide({ kind: "internal_transfer" })}
            className="rounded border border-border-strong px-4 py-1.5 text-sm text-text hover:border-text"
          >
            {t("review.action.transfer", locale)}
          </button>
          <button
            type="button"
            onClick={() => void decide({ kind: "not_in_pnl" })}
            className="rounded border border-border-strong px-4 py-1.5 text-sm text-text hover:border-text"
          >
            {t("review.action.notPnl", locale)}
          </button>
        </div>
      </section>

      <section className="rounded-panel border border-border bg-surface-raised p-6">
        <h3 className="text-2xs font-semibold uppercase tracking-widest text-text-subtle">
          {t("review.transactions", locale)}
        </h3>
        <div ref={parentRef} className="mt-4 max-h-96 overflow-y-auto">
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
    </div>
  );
}
