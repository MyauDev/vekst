import { useState } from "react";

import {
  CURRENCY,
  DRILLDOWN_KEY,
  DRILLDOWN_ROWS,
  MONTHS,
  PNL_LINES,
  PROVENANCE,
  RECONCILIATION,
  SECTIONS,
  t,
} from "./data";
import type { PnlLine } from "./data";
import { formatMinorUnits, percentOfRevenue, sumMinorUnits } from "./money";
import type { Locale } from "./money";
import { Chip, Panel, ScreenHeading, c } from "./ui";

const GRID = "340px repeat(8, 84px) 108px 64px";

function columnTotals(lines: readonly PnlLine[]): string[] {
  return MONTHS.map((_, i) => lines.reduce((acc, l) => acc + BigInt(l.values[i] ?? "0"), 0n).toString());
}

function addColumns(a: readonly string[], b: readonly string[]): string[] {
  return a.map((v, i) => (BigInt(v) + BigInt(b[i] ?? "0")).toString());
}

type Row =
  | { kind: "section"; label: string }
  | { kind: "line"; line: PnlLine }
  | { kind: "subtotal"; label: string; cells: readonly string[] };

interface Cell {
  label: string;
  monthIndex: number;
  amount: string;
}

export function ReportScreen({ lang }: { lang: Locale }) {
  const [open, setOpen] = useState<Cell | null>(null);

  const priced = (section: string) => PNL_LINES.filter((l) => l.section === section && l.values.length > 0);
  const revenue = columnTotals(priced("Revenue"));
  const costOfSales = columnTotals(priced("Cost of sales"));
  const opex = columnTotals(priced("Operating expenses"));
  const gross = addColumns(revenue, costOfSales);
  const ebitda = addColumns(gross, opex);
  const revenueTotal = sumMinorUnits(revenue);

  const rows: Row[] = [];
  for (const section of SECTIONS) {
    rows.push({ kind: "section", label: section });
    for (const line of PNL_LINES.filter((l) => l.section === section)) rows.push({ kind: "line", line });
    if (section === "Revenue") rows.push({ kind: "subtotal", label: t("totalRevenue", lang), cells: revenue });
    if (section === "Cost of sales") rows.push({ kind: "subtotal", label: t("grossProfit", lang), cells: gross });
    if (section === "Operating expenses") rows.push({ kind: "subtotal", label: t("ebitda", lang), cells: ebitda });
  }

  const closing = sumMinorUnits([
    RECONCILIATION.opening,
    RECONCILIATION.moneyIn,
    RECONCILIATION.moneyOut,
    RECONCILIATION.transfers,
  ]);

  const strip = [
    { label: t("opening", lang), value: RECONCILIATION.opening },
    { label: t("moneyIn", lang), value: RECONCILIATION.moneyIn },
    { label: t("moneyOut", lang), value: RECONCILIATION.moneyOut },
    { label: t("transfers", lang), value: RECONCILIATION.transfers },
    { label: t("closing", lang), value: closing },
  ];

  return (
    <>
      <div style={{ padding: "20px 24px", display: "flex", flexDirection: "column", gap: 16, minWidth: 0 }}>
        <ScreenHeading
          title={t("pnl", lang)}
          meta={`${CURRENCY} · ${PROVENANCE}`}
          right={
            <>
              <Chip tone="outline">{t("cashBasis", lang)}</Chip>
              <Chip tone="warn">{t("partial", lang)} · 1 line blocked</Chip>
            </>
          }
        />

        <Panel style={{ display: "flex", alignItems: "stretch" }}>
          {strip.map((item) => (
            <div
              key={item.label}
              style={{
                flexGrow: 1,
                padding: "10px 14px",
                borderLeft: `1px solid ${c.border}`,
                display: "flex",
                flexDirection: "column",
                gap: 3,
              }}
            >
              <span style={{ fontSize: 11, color: c.subtle }}>{item.label}</span>
              <span className="vk-num" style={{ fontSize: 13, fontWeight: 500, textAlign: "left" }}>
                {formatMinorUnits(item.value, lang)}
              </span>
            </div>
          ))}
          <div
            style={{
              padding: "10px 14px",
              borderLeft: `1px solid ${c.border}`,
              display: "flex",
              alignItems: "center",
              gap: 6,
              color: c.ok,
            }}
          >
            <svg width="14" height="14" viewBox="0 0 14 14" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
              <path d="M2.8 7.4 5.6 10.2 11.2 4" />
            </svg>
            <span style={{ fontSize: 12, fontWeight: 500 }}>{t("balances", lang)}</span>
          </div>
        </Panel>

        <Panel>
          <div
            style={{
              display: "grid",
              gridTemplateColumns: GRID,
              alignItems: "center",
              height: 32,
              borderBottom: `1px solid ${c.borderStrong}`,
              background: c.sunken,
              fontSize: 11,
              color: c.subtle,
            }}
          >
            <div style={{ padding: "0 12px" }}>{t("category", lang)}</div>
            {MONTHS.map((m) => (
              <div key={m} className="vk-num" style={{ padding: "0 10px" }}>
                {m}
              </div>
            ))}
            <div className="vk-num" style={{ padding: "0 12px", color: c.muted, fontWeight: 500 }}>
              {t("total", lang)}
            </div>
            <div className="vk-num" style={{ padding: "0 12px" }}>
              {t("pctRev", lang)}
            </div>
          </div>

          {rows.map((row, index) => {
            if (row.kind === "section") {
              return (
                <div
                  key={`s-${row.label}`}
                  style={{
                    display: "grid",
                    gridTemplateColumns: GRID,
                    alignItems: "center",
                    height: 30,
                    borderTop: index === 0 ? undefined : `1px solid ${c.borderStrong}`,
                    background: c.surface,
                    fontSize: 11,
                    fontWeight: 600,
                    color: c.muted,
                  }}
                >
                  <div style={{ padding: "0 12px" }}>{row.label}</div>
                </div>
              );
            }

            const blocked = row.kind === "line" && row.line.values.length === 0;
            const cells = row.kind === "line" ? row.line.values : row.cells;
            const label = row.kind === "line" ? row.line.label : row.label;
            const total = blocked ? "" : sumMinorUnits(cells);
            const subtotal = row.kind === "subtotal";

            return (
              <div
                key={`r-${label}`}
                style={{
                  display: "grid",
                  gridTemplateColumns: GRID,
                  alignItems: "center",
                  height: 32,
                  borderTop: `1px solid ${subtotal ? c.borderStrong : c.hairline}`,
                }}
              >
                <div style={{ padding: "0 12px", display: "flex", alignItems: "center", gap: 8, minWidth: 0 }}>
                  <span
                    style={{
                      fontSize: 13,
                      fontWeight: subtotal ? 600 : 400,
                      color: blocked ? c.subtle : c.text,
                      whiteSpace: "nowrap",
                      overflow: "hidden",
                      textOverflow: "ellipsis",
                    }}
                  >
                    {label}
                  </span>
                  {blocked && row.kind === "line" ? (
                    <>
                      <Chip tone="danger">{t("blocked", lang)}</Chip>
                      <span style={{ fontSize: 11, color: c.subtle, whiteSpace: "nowrap" }}>{row.line.blockedReason}</span>
                    </>
                  ) : null}
                </div>

                {MONTHS.map((month, i) => {
                  const amount = cells[i];
                  if (blocked || amount === undefined) {
                    return (
                      <div key={month} className="vk-num" style={{ padding: "0 10px", fontSize: 13, color: c.subtle }}>
                        —
                      </div>
                    );
                  }
                  const active = open?.label === label && open.monthIndex === i;
                  return (
                    <button
                      key={month}
                      type="button"
                      className="vk-num"
                      onClick={() => setOpen({ label, monthIndex: i, amount })}
                      style={{
                        padding: "0 10px",
                        height: "100%",
                        fontSize: 13,
                        fontWeight: subtotal ? 600 : 400,
                        display: "flex",
                        alignItems: "center",
                        justifyContent: "flex-end",
                        background: active ? c.accentSurface : "transparent",
                        boxShadow: active ? `inset 0 0 0 1px ${c.focus}` : undefined,
                      }}
                    >
                      {formatMinorUnits(amount, lang)}
                    </button>
                  );
                })}

                <div
                  className="vk-num"
                  style={{ padding: "0 12px", fontSize: 13, fontWeight: subtotal ? 600 : 500, color: blocked ? c.subtle : c.text }}
                >
                  {blocked ? "—" : formatMinorUnits(total, lang)}
                </div>
                <div className="vk-num" style={{ padding: "0 12px", fontSize: 12, color: c.subtle }}>
                  {blocked ? "" : percentOfRevenue(total, revenueTotal, lang)}
                </div>
              </div>
            );
          })}
        </Panel>

        <div style={{ display: "flex", alignItems: "center", gap: 14, fontSize: 11, color: c.subtle }}>
          <span>6 internal transfers excluded from every line</span>
          <span style={{ width: 1, height: 10, background: c.border }} />
          <span>Click any figure to open the transactions behind it</span>
        </div>
      </div>

      {open ? <Drilldown cell={open} lang={lang} onClose={() => setOpen(null)} /> : null}
    </>
  );
}

function Drilldown({ cell, lang, onClose }: { cell: Cell; lang: Locale; onClose: () => void }) {
  const key = `${cell.label}:${cell.monthIndex}`;
  const rows = key === DRILLDOWN_KEY ? DRILLDOWN_ROWS : [];
  const shown = sumMinorUnits(rows.map((r) => r.amount));
  const rest = (BigInt(cell.amount) - BigInt(shown)).toString();
  const month = MONTHS[cell.monthIndex] ?? "";

  return (
    <aside
      style={{
        position: "absolute",
        top: 48,
        right: 0,
        bottom: 0,
        width: 480,
        background: c.raised,
        borderLeft: `1px solid ${c.borderStrong}`,
        boxShadow: "-8px 0 24px oklch(24% 0.008 95 / 0.08)",
        display: "flex",
        flexDirection: "column",
        overflowY: "auto",
      }}
    >
      <div style={{ padding: "16px 20px 14px", borderBottom: `1px solid ${c.border}`, display: "flex", flexDirection: "column", gap: 10 }}>
        <div style={{ display: "flex", alignItems: "flex-start", gap: 10 }}>
          <div style={{ display: "flex", flexDirection: "column", gap: 3, minWidth: 0 }}>
            <span style={{ fontSize: 11, color: c.subtle }}>{t("pnl", lang)}</span>
            <span style={{ fontSize: 16, fontWeight: 600 }}>
              {cell.label} · {month} 2026
            </span>
          </div>
          <button
            type="button"
            onClick={onClose}
            aria-label="Close"
            style={{
              marginLeft: "auto",
              width: 24,
              height: 24,
              borderRadius: "var(--vk-radius-control)",
              border: `1px solid ${c.border}`,
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              flexShrink: 0,
            }}
          >
            <svg width="12" height="12" viewBox="0 0 12 12" fill="none" stroke={c.muted} strokeWidth="1.4" strokeLinecap="round" aria-hidden="true">
              <path d="M3 3l6 6M9 3l-6 6" />
            </svg>
          </button>
        </div>
        <div style={{ display: "flex", alignItems: "baseline", gap: 10 }}>
          <span className="vk-num" style={{ fontSize: 24, fontWeight: 600, letterSpacing: "-0.01em" }}>
            {formatMinorUnits(cell.amount, lang)}
          </span>
          <span style={{ fontSize: 12, color: c.subtle }}>{CURRENCY}</span>
          <span style={{ marginLeft: "auto" }}>
            <Chip tone="outline">{t("cashBasis", lang)}</Chip>
          </span>
        </div>
        <div className="vk-mono" style={{ fontSize: 11, color: c.subtle }}>
          {PROVENANCE}
        </div>
      </div>

      {rows.length === 0 ? (
        <div style={{ padding: "16px 20px", fontSize: 13, color: c.muted }}>
          The transaction list is mocked for Logistics · March only. Every other cell shows its real figure and
          provenance, because inventing a few hundred transactions is exactly what the plan bans.
        </div>
      ) : (
        <>
          {rows.map((row) => (
            <div
              key={`${row.date}-${row.description}`}
              style={{
                display: "grid",
                gridTemplateColumns: "64px 1fr 96px",
                alignItems: "center",
                padding: "8px 0",
                borderTop: `1px solid ${c.hairline}`,
              }}
            >
              <div className="vk-num" style={{ padding: "0 20px", fontSize: 12, color: c.muted }}>
                {row.date}
              </div>
              <div style={{ padding: "0 8px", display: "flex", flexDirection: "column", gap: 3, minWidth: 0 }}>
                <span style={{ fontSize: 13, whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>
                  {row.description}
                </span>
                <span style={{ display: "flex", gap: 6, flexWrap: "wrap" }}>
                  <Chip tone="outline">{row.category}</Chip>
                  <Chip tone={Number(row.confidence) >= 0.8 ? "ok" : "warn"}>
                    {row.layer} · {row.confidence}
                  </Chip>
                  {row.fromVendorMemory ? <Chip tone="neutral">from vendor memory</Chip> : null}
                </span>
                {row.evidence ? <span style={{ fontSize: 11, color: c.subtle }}>{row.evidence}</span> : null}
              </div>
              <div className="vk-num" style={{ padding: "0 20px", fontSize: 13 }}>
                {formatMinorUnits(row.amount, lang)}
              </div>
            </div>
          ))}
          <div
            style={{
              display: "grid",
              gridTemplateColumns: "64px 1fr 96px",
              alignItems: "center",
              height: 34,
              borderTop: `1px solid ${c.hairline}`,
              color: c.subtle,
            }}
          >
            <div />
            <div style={{ padding: "0 8px", fontSize: 12 }}>7 more transactions</div>
            <div className="vk-num" style={{ padding: "0 20px", fontSize: 12 }}>
              {formatMinorUnits(rest, lang)}
            </div>
          </div>
          <div
            style={{
              marginTop: "auto",
              borderTop: `1px solid ${c.borderStrong}`,
              background: c.surface,
              padding: "12px 20px",
              display: "flex",
              alignItems: "center",
              gap: 8,
            }}
          >
            <svg width="14" height="14" viewBox="0 0 14 14" fill="none" stroke={c.ok} strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
              <path d="M2.8 7.4 5.6 10.2 11.2 4" />
            </svg>
            <span style={{ fontSize: 12, color: c.muted }}>12 transactions sum to</span>
            <span className="vk-num" style={{ fontSize: 13, fontWeight: 500 }}>
              {formatMinorUnits(cell.amount, lang)}
            </span>
            <span style={{ fontSize: 12, color: c.muted }}>— the figure in the report.</span>
          </div>
        </>
      )}
    </aside>
  );
}
