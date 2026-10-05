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
 * The picker is the taxonomy's own tree, walked one level at a time: a section
 * (Net sales, Operating expenses, ...), then a department, then the leaf. A flat
 * list of leaves could not tell five "Salary" leaves apart -- they differ only by
 * the department above them. `1`-`9` address whatever the current level shows,
 * a branch opens and a leaf is chosen; `Backspace` goes back up, and the
 * breadcrumb above the options jumps to any level by click. The filter field
 * searches every leaf at once, each shown with its path, and re-numbers `1`-`9`
 * onto what matched. Only the side of the P&L the group's sign points to is
 * offered -- an outflow never sees "Net sales" -- until the reviewer asks for
 * the rest.
 *
 * 44px rows rather than the 40px of the tables (§5, as amended 2026-09-16).
 * This screen takes keyboard focus and its rows are targets, so it keeps the
 * one-step lead over the tables it has always had.
 */
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";

import { ResolveGroupError, decideGroup, listCategories, listGroupTransactions, listReviewGroups } from "../data/review";
import type { ReviewDecision } from "../data/review";
import { SEPARATOR, buildCategoryTree, leavesOf, openPath, sideOf } from "./categoryTree";
import type { Side, TreeNode } from "./categoryTree";
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
    [<Kbd key="b">⌫</Kbd>, "review.key.back"],
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
  // The branches opened on the way down, by key. Keys rather than nodes: the
  // tree is rebuilt whenever the locale changes, and a stale node would keep
  // showing the old language's names.
  const [openKeys, setOpenKeys] = useState<readonly string[]>([]);
  // The other side of the taxonomy, on request: a counterparty that both
  // pays and is paid nets to one sign, and its rows of the other sign still
  // need a category from the other side.
  const [showAllSides, setShowAllSides] = useState(false);
  // A decision that fails must say so: the backend returns a coded failure
  // (a race with another reviewer, a stale category), and a screen that
  // swallows it looks identical to one that did nothing at all.
  const [decideError, setDecideError] = useState<string | null>(null);
  const parentRef = useRef<HTMLDivElement>(null);

  const groups = useQuery({ queryKey: ["reviewGroups"], queryFn: listReviewGroups });
  const categories = useQuery({ queryKey: ["categories"], queryFn: listCategories });

  const list = groups.data ?? [];
  const group = list[Math.min(index, Math.max(list.length - 1, 0))];

  // Money that went out is an expense, money that came in is income: the
  // picker opens on the side the group's own sign says, so an outflow is
  // never offered "Net sales". A group that nets to zero says nothing.
  const units = group?.total.minorUnits ?? "0";
  const groupSide: Side | null = /^-?0*$/.test(units) ? null : units.startsWith("-") ? "expense" : "income";
  const side = showAllSides ? null : groupSide;

  // Translated once per locale change, not per render of every button below:
  // the names in it are what the picker, the search and the "Selected" line all
  // read, so a Russian reviewer can find "Аренда офиса" by typing "аренда"
  // rather than having to know the English name stored on the wire.
  const tree = useMemo(() => {
    const offered = (categories.data ?? []).filter((c) => {
      const s = sideOf(c.code);
      return side === null || s === "both" || s === side;
    });
    return buildCategoryTree(offered, locale);
  }, [categories.data, locale, side]);
  const allLeaves = useMemo(() => leavesOf(tree), [tree]);
  const trail = useMemo(() => openPath(tree, openKeys), [tree, openKeys]);
  // Recomputed, not just filtered from a stale reference: this is also what
  // "1"-"9" address below, so it has to be exactly what is on screen right
  // now, search included. A search spans the whole tree and matches a path as
  // much as a name: "finance" finds every leaf under Finance.
  const searching = query.trim() !== "";
  const visibleOptions: readonly TreeNode[] = useMemo(() => {
    const needle = query.trim().toLowerCase();
    if (needle) {
      return allLeaves.filter((l) => [...l.trail, l.name].join(" ").toLowerCase().includes(needle));
    }
    return (trail.at(-1) ?? tree).children;
  }, [allLeaves, query, trail, tree]);
  const selectedLeaf = selected ? allLeaves.find((l) => l.code === selected) : undefined;

  const choose = useCallback(
    (node: TreeNode) => {
      if (node.kind === "leaf") {
        setSelected(node.code);
      } else {
        setOpenKeys([...trail.map((b) => b.key), node.key]);
      }
    },
    [trail],
  );

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
    setOpenKeys([]);
    setShowAllSides(false);
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
  // field has focus rather than the selected group, nor Backspace, which
  // deletes what was typed.
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.metaKey || e.ctrlKey || e.altKey) return;
      const typing = document.activeElement instanceof HTMLInputElement;

      if (/^[1-9]$/.test(e.key)) {
        const node = visibleOptions[Number(e.key) - 1];
        if (node) {
          choose(node);
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
        case "Backspace":
          setOpenKeys((keys) => keys.slice(0, -1));
          e.preventDefault();
          break;
        case "Escape":
          setSelected(null);
          setOpenKeys([]);
          e.preventDefault();
          break;
      }
    }
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [visibleOptions, choose, selected, decide, list.length]);

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
            {selectedLeaf ? (
              <>
                {t("review.selected", locale)}: {selectedLeaf.trail.map((name) => name + SEPARATOR).join("")}
                <span className="font-medium text-text">{selectedLeaf.name}</span>
              </>
            ) : (
              " "
            )}
          </span>
        </div>

        {searching ? null : (
          <nav aria-label={t("review.tree.root", locale)} className="flex flex-wrap items-center gap-1 text-sm">
            {[{ key: "", label: t("review.tree.root", locale) }, ...trail].map((b, i, all) => (
              <span key={b.key} className="flex items-center gap-1">
                {i > 0 ? <span className="text-text-subtle">›</span> : null}
                {i === all.length - 1 ? (
                  <span aria-current="location" className="font-medium text-text">
                    {b.label}
                  </span>
                ) : (
                  <button
                    type="button"
                    onClick={() => setOpenKeys(trail.slice(0, i).map((x) => x.key))}
                    className="text-text-muted underline-offset-2 hover:text-text hover:underline"
                  >
                    {b.label}
                  </button>
                )}
              </span>
            ))}
          </nav>
        )}

        {groupSide ? (
          <p className="flex flex-wrap items-center gap-x-3 gap-y-1 text-sm text-text-muted">
            {showAllSides ? null : t(groupSide === "expense" ? "review.side.expense" : "review.side.income", locale)}
            <button
              type="button"
              onClick={() => {
                // Hiding a side must not leave a choice from it armed behind
                // Enter, invisible.
                if (showAllSides) setSelected(null);
                setShowAllSides(!showAllSides);
                setOpenKeys([]);
              }}
              className="text-text underline underline-offset-2 hover:text-text-muted"
            >
              {t(showAllSides ? "review.side.showMatching" : "review.side.showAll", locale)}
            </button>
          </p>
        ) : null}

        <ol className="flex flex-wrap gap-2">
          {visibleOptions.length === 0 ? (
            <li className="text-sm text-text-subtle">{t("review.search.empty", locale)}</li>
          ) : (
            visibleOptions.map((node, i) => {
              const on = node.kind === "leaf" && selected === node.code;
              return (
                <li key={node.key}>
                  <button
                    type="button"
                    onClick={() => choose(node)}
                    aria-pressed={node.kind === "leaf" ? on : undefined}
                    className={
                      "flex items-center gap-1.5 rounded border px-2 py-1 text-left text-sm transition-colors " +
                      (on
                        ? "border-text bg-surface-sunken font-medium text-text"
                        : node.kind === "branch"
                          ? "border-border text-text hover:border-border-strong hover:bg-surface-sunken"
                          : "border-transparent text-text-muted hover:border-border hover:bg-surface-sunken hover:text-text")
                    }
                  >
                    {i < 9 ? <Kbd>{i + 1}</Kbd> : null}
                    {/* In a search the tree is gone, so each match carries the
                        path the tree would otherwise have shown. */}
                    {searching && node.kind === "leaf" ? (
                      <>
                        <span className="text-text-subtle">{node.trail.map((name) => name + SEPARATOR).join("")}</span>
                        {node.name}
                      </>
                    ) : (
                      node.label
                    )}
                    {node.kind === "branch" ? (
                      <span aria-hidden="true" className="text-text-subtle">
                        ›
                      </span>
                    ) : null}
                  </button>
                </li>
              );
            })
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
