import { useEffect, useState } from "react";

import { CATEGORIES, t } from "./data";
import type { ReviewGroup } from "./data";
import { formatMinorUnits, sumMinorUnits } from "./money";
import type { Locale } from "./money";
import { Chip, Key, Panel, ScreenHeading, c } from "./ui";

export interface Resolution {
  counterparty: string;
  decision: string;
  rows: number;
}

const VISIBLE_CATEGORIES = CATEGORIES.slice(0, 6);

export function ReviewScreen({
  lang,
  queue,
  resolved,
  onResolve,
}: {
  lang: Locale;
  queue: readonly ReviewGroup[];
  resolved: readonly Resolution[];
  onResolve: (group: ReviewGroup, decision: string) => void;
}) {
  const [index, setIndex] = useState(0);
  const [picked, setPicked] = useState<string | null>(null);

  const safeIndex = queue.length === 0 ? 0 : Math.min(index, queue.length - 1);
  const current = queue[safeIndex];

  useEffect(() => {
    function onKeyDown(event: KeyboardEvent) {
      if (event.metaKey || event.ctrlKey || event.altKey) return;
      if (!current) return;

      if (event.key >= "1" && event.key <= "9") {
        const category = VISIBLE_CATEGORIES[Number(event.key) - 1];
        if (category) {
          setPicked(category);
          event.preventDefault();
        }
        return;
      }

      switch (event.key) {
        case "Enter": {
          const decision = picked ?? (current.suggested || null);
          if (decision) {
            onResolve(current, decision);
            setPicked(null);
          }
          event.preventDefault();
          break;
        }
        case "t":
        case "T":
          onResolve(current, "Internal transfer");
          setPicked(null);
          event.preventDefault();
          break;
        case "n":
        case "N":
          onResolve(current, "Not in the P&L");
          setPicked(null);
          event.preventDefault();
          break;
        case "ArrowDown":
          setIndex((i) => Math.min(i + 1, queue.length - 1));
          setPicked(null);
          event.preventDefault();
          break;
        case "ArrowUp":
          setIndex((i) => Math.max(i - 1, 0));
          setPicked(null);
          event.preventDefault();
          break;
        case "Escape":
          setPicked(null);
          break;
        default:
          break;
      }
    }

    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [current, picked, queue.length, onResolve]);

  const remainingRows = queue.reduce((acc, g) => acc + g.rows, 0);
  const remainingAmount = sumMinorUnits(queue.map((g) => g.amount));

  return (
    <div
      style={{
        padding: "20px 24px 0",
        display: "flex",
        flexDirection: "column",
        gap: 14,
        minWidth: 0,
        flexGrow: 1,
      }}
    >
      <ScreenHeading
        title={t("reviewQueue", lang)}
        right={<span style={{ fontSize: 11, color: c.subtle }}>Sorted by amount, then by repeat count</span>}
      />
      <div style={{ fontSize: 12, color: c.muted, fontVariantNumeric: "tabular-nums", marginTop: -8 }}>
        {remainingRows} rows below 0.80 confidence · {queue.length} counterparties ·{" "}
        {formatMinorUnits(remainingAmount, lang)} unreviewed
        {resolved.length > 0 ? ` · ${resolved.length} decided this session` : ""}
      </div>

      {current ? (
        <div style={{ display: "flex", gap: 16, alignItems: "flex-start", minHeight: 0 }}>
          <Panel style={{ width: 340, flexShrink: 0 }}>
            <div
              style={{
                height: 28,
                display: "flex",
                alignItems: "center",
                padding: "0 12px",
                background: c.sunken,
                borderBottom: `1px solid ${c.border}`,
                fontSize: 11,
                color: c.subtle,
              }}
            >
              <span>Counterparty</span>
              <span style={{ marginLeft: "auto" }}>Amount</span>
            </div>
            {queue.map((group, i) => {
              const active = i === safeIndex;
              return (
                <button
                  key={group.id}
                  type="button"
                  onClick={() => {
                    setIndex(i);
                    setPicked(null);
                  }}
                  style={{
                    display: "flex",
                    alignItems: "center",
                    gap: 10,
                    width: "100%",
                    height: 44,
                    padding: "0 12px",
                    borderTop: i === 0 ? undefined : `1px solid ${c.hairline}`,
                    background: active ? c.sunken : "transparent",
                    outline: active ? `2px solid ${c.focus}` : undefined,
                    outlineOffset: -2,
                  }}
                >
                  <span style={{ display: "flex", flexDirection: "column", gap: 1, minWidth: 0 }}>
                    <span
                      style={{
                        fontSize: 13,
                        fontWeight: active ? 500 : 400,
                        whiteSpace: "nowrap",
                        overflow: "hidden",
                        textOverflow: "ellipsis",
                      }}
                    >
                      {group.counterparty}
                    </span>
                    <span style={{ fontSize: 11, color: c.subtle, fontVariantNumeric: "tabular-nums" }}>
                      {group.rows} rows · {group.layer} · {group.confidence}
                    </span>
                  </span>
                  <span
                    className="vk-num"
                    style={{ marginLeft: "auto", fontSize: 13, fontWeight: active ? 500 : 400 }}
                  >
                    {formatMinorUnits(group.amount, lang)}
                  </span>
                </button>
              );
            })}
          </Panel>

          <div style={{ flexGrow: 1, minWidth: 0, display: "flex", flexDirection: "column", gap: 14 }}>
            <Panel>
              <div
                style={{
                  padding: "12px 16px",
                  borderBottom: `1px solid ${c.border}`,
                  display: "flex",
                  alignItems: "center",
                  gap: 10,
                }}
              >
                <div style={{ display: "flex", flexDirection: "column", gap: 2 }}>
                  <span style={{ fontSize: 15, fontWeight: 600 }}>{current.counterparty}</span>
                  <span style={{ fontSize: 11, color: c.subtle, fontVariantNumeric: "tabular-nums" }}>
                    {current.rows} rows · Jan – Aug 2026 · bank · engine {current.layer} · confidence {current.confidence}
                  </span>
                </div>
                <span className="vk-num" style={{ marginLeft: "auto", fontSize: 16, fontWeight: 600 }}>
                  {formatMinorUnits(current.amount, lang)}
                </span>
              </div>

              <div
                style={{
                  display: "grid",
                  gridTemplateColumns: "90px 1fr 140px",
                  height: 28,
                  alignItems: "center",
                  background: c.sunken,
                  borderBottom: `1px solid ${c.border}`,
                  fontSize: 11,
                  color: c.subtle,
                }}
              >
                <div style={{ padding: "0 16px" }}>Date</div>
                <div style={{ padding: "0 12px" }}>Description on the statement</div>
                <div className="vk-num" style={{ padding: "0 12px" }}>Amount</div>
              </div>

              {current.transactions.map((transaction, i) => (
                <div
                  key={`${transaction.date}-${transaction.description}`}
                  style={{
                    display: "grid",
                    gridTemplateColumns: "90px 1fr 140px",
                    height: 36,
                    alignItems: "center",
                    borderTop: i === 0 ? undefined : `1px solid ${c.hairline}`,
                  }}
                >
                  <div className="vk-num" style={{ padding: "0 16px", fontSize: 12, color: c.muted }}>
                    {transaction.date}
                  </div>
                  <div
                    style={{
                      padding: "0 12px",
                      fontSize: 13,
                      whiteSpace: "nowrap",
                      overflow: "hidden",
                      textOverflow: "ellipsis",
                    }}
                  >
                    {transaction.description}
                  </div>
                  <div className="vk-num" style={{ padding: "0 12px", fontSize: 13 }}>
                    {formatMinorUnits(transaction.amount, lang)}
                  </div>
                </div>
              ))}

              {current.rows > current.transactions.length ? (
                <div
                  style={{
                    display: "grid",
                    gridTemplateColumns: "90px 1fr 140px",
                    height: 36,
                    alignItems: "center",
                    borderTop: `1px solid ${c.hairline}`,
                    color: c.subtle,
                  }}
                >
                  <div />
                  <div style={{ padding: "0 12px", fontSize: 12 }}>
                    {current.rows - current.transactions.length} more rows from this counterparty
                  </div>
                  <div />
                </div>
              ) : null}
            </Panel>

            <Panel style={{ padding: "12px 16px", display: "flex", flexDirection: "column", gap: 10 }}>
              <div style={{ display: "flex", alignItems: "center", gap: 8, flexWrap: "wrap" }}>
                <span style={{ fontSize: 13, fontWeight: 500 }}>Assign a category</span>
                <span style={{ fontSize: 11, color: c.subtle }}>
                  One decision covers all {current.rows} rows and is remembered for this counterparty next month
                </span>
              </div>
              <div style={{ display: "grid", gridTemplateColumns: "repeat(3, minmax(0, 1fr))", gap: 8 }}>
                {VISIBLE_CATEGORIES.map((category, i) => {
                  const chosen = (picked ?? current.suggested) === category;
                  return (
                    <button
                      key={category}
                      type="button"
                      onClick={() => setPicked(category)}
                      style={{
                        display: "flex",
                        alignItems: "center",
                        gap: 8,
                        height: 32,
                        padding: "0 10px",
                        border: `1px solid ${chosen ? c.borderStrong : c.border}`,
                        borderRadius: "var(--vk-radius-control)",
                        background: chosen ? c.sunken : "transparent",
                        outline: chosen ? `2px solid ${c.focus}` : undefined,
                        outlineOffset: -2,
                        fontWeight: chosen ? 500 : 400,
                        fontSize: 13,
                      }}
                    >
                      <Key>{i + 1}</Key>
                      <span>{category}</span>
                    </button>
                  );
                })}
              </div>
            </Panel>
          </div>
        </div>
      ) : (
        <Panel style={{ padding: "32px 24px", display: "flex", flexDirection: "column", gap: 8, alignItems: "flex-start" }}>
          <Chip tone="ok">{t("ready", lang)}</Chip>
          <div style={{ fontSize: 15, fontWeight: 600 }}>The queue is clear.</div>
          <div style={{ fontSize: 13, color: c.muted }}>
            {resolved.length} counterparties decided. Every decision was written to vendor memory, so the same names do
            not come back next month.
          </div>
        </Panel>
      )}

      <div
        style={{
          marginTop: "auto",
          borderTop: `1px solid ${c.border}`,
          margin: "auto -24px 0",
          padding: "10px 24px",
          display: "flex",
          alignItems: "center",
          gap: 16,
          background: c.surface,
          flexWrap: "wrap",
        }}
      >
        <span style={{ display: "inline-flex", alignItems: "center", gap: 6, fontSize: 12, color: c.muted }}>
          <Key>1</Key>–<Key>6</Key> pick a category
        </span>
        <span style={{ display: "inline-flex", alignItems: "center", gap: 6, fontSize: 12, color: c.muted }}>
          <Key>↵</Key> approve the group
        </span>
        <span style={{ display: "inline-flex", alignItems: "center", gap: 6, fontSize: 12, color: c.muted }}>
          <Key>T</Key> internal transfer
        </span>
        <span style={{ display: "inline-flex", alignItems: "center", gap: 6, fontSize: 12, color: c.muted }}>
          <Key>N</Key> not in the P&L
        </span>
        <span style={{ display: "inline-flex", alignItems: "center", gap: 6, fontSize: 12, color: c.muted }}>
          <Key>↑</Key>
          <Key>↓</Key> move
        </span>
        <span style={{ marginLeft: "auto", fontSize: 12, color: c.subtle, fontVariantNumeric: "tabular-nums" }}>
          {queue.length === 0 ? "0 left" : `${safeIndex + 1} of ${queue.length} · ${remainingRows} rows left`}
        </span>
      </div>
    </div>
  );
}
