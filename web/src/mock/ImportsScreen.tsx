import { useState } from "react";

import { BALANCE_CHECK, BATCHES, VALIDATION_ERRORS, t } from "./data";
import type { Batch, BatchState } from "./data";
import { formatMinorUnits } from "./money";
import type { Locale } from "./money";
import { Chip, Panel, ScreenHeading, c } from "./ui";
import type { Tone } from "./ui";

const GRID = "440px 110px 160px 90px 170px 130px";

const STATE_TONE: Record<BatchState, Tone> = {
  pending: "outline",
  parsing: "neutral",
  rejected: "danger",
  imported: "ok",
};

export function ImportsScreen({ lang }: { lang: Locale }) {
  const [selectedId, setSelectedId] = useState<string>("b-08");
  const selected = BATCHES.find((b) => b.id === selectedId);

  return (
    <div style={{ padding: "20px 24px", display: "flex", flexDirection: "column", gap: 16, minWidth: 0 }}>
      <ScreenHeading
        title={t("imports", lang)}
        right={
          <>
            <span style={{ fontSize: 11, color: c.subtle }}>Sample files: bank export · 1C export · card statement</span>
            <div
              style={{
                display: "flex",
                alignItems: "center",
                gap: 6,
                height: 28,
                padding: "0 12px",
                borderRadius: "var(--vk-radius-control)",
                background: c.accent,
                color: c.inverse,
                fontSize: 13,
                fontWeight: 500,
              }}
            >
              <svg width="13" height="13" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
                <path d="M8 13V4" />
                <path d="M4.6 7.2 8 3.8l3.4 3.4" />
              </svg>
              Upload file
            </div>
          </>
        }
      />

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
          <div style={{ padding: "0 12px" }}>File</div>
          <div style={{ padding: "0 12px" }}>Source</div>
          <div style={{ padding: "0 12px" }}>Statement period</div>
          <div className="vk-num" style={{ padding: "0 12px" }}>Rows</div>
          <div style={{ padding: "0 12px" }}>State</div>
          <div className="vk-num" style={{ padding: "0 12px" }}>Uploaded</div>
        </div>

        {BATCHES.map((batch, index) => (
          <BatchRow
            key={batch.id}
            batch={batch}
            lang={lang}
            first={index === 0}
            selected={batch.id === selectedId}
            onSelect={() => setSelectedId(batch.id)}
          />
        ))}
      </Panel>

      {selected?.state === "rejected" && !selected.note ? <ValidationReport lang={lang} file={selected.file} /> : null}
      {selected?.state === "rejected" && selected.note ? (
        <Panel style={{ padding: "14px 16px", display: "flex", alignItems: "center", gap: 10 }}>
          <Chip tone="warn">{t("rejected", lang)}</Chip>
          <span style={{ fontSize: 13 }}>
            D1 — this exact file was imported already. Nothing was read a second time.
          </span>
        </Panel>
      ) : null}
      {selected?.state === "imported" ? (
        <Panel style={{ padding: "14px 16px", display: "flex", alignItems: "center", gap: 10 }}>
          <Chip tone="ok">{t("imported", lang)}</Chip>
          <span style={{ fontSize: 13, color: c.muted }}>
            {selected.summary
              ? "The import summary is on the row above."
              : "This batch imported without a duplicate or a transfer to report."}
          </span>
        </Panel>
      ) : null}
    </div>
  );
}

function BatchRow({
  batch,
  lang,
  first,
  selected,
  onSelect,
}: {
  batch: Batch;
  lang: Locale;
  first: boolean;
  selected: boolean;
  onSelect: () => void;
}) {
  return (
    <>
      <button
        type="button"
        onClick={onSelect}
        aria-pressed={selected}
        style={{
          display: "grid",
          gridTemplateColumns: GRID,
          alignItems: "center",
          width: "100%",
          height: 32,
          borderTop: first ? undefined : `1px solid ${c.hairline}`,
          background: selected ? c.sunken : "transparent",
          boxShadow: selected ? `inset 2px 0 0 ${c.accent}` : undefined,
        }}
      >
        <span
          style={{
            padding: "0 12px",
            fontSize: 13,
            fontWeight: selected ? 500 : 400,
            whiteSpace: "nowrap",
            overflow: "hidden",
            textOverflow: "ellipsis",
          }}
        >
          {batch.file}
        </span>
        <span style={{ padding: "0 12px" }}>
          <Chip tone="outline">{batch.sourceKind}</Chip>
        </span>
        <span style={{ padding: "0 12px", fontSize: 12, color: c.muted }}>{batch.period}</span>
        <span className="vk-num" style={{ padding: "0 12px", fontSize: 13, color: batch.rows === null ? c.subtle : c.text }}>
          {batch.rows === null ? "—" : batch.rows.toLocaleString("en-GB")}
        </span>
        <span style={{ padding: "0 12px", display: "flex", alignItems: "center", gap: 6 }}>
          <Chip tone={batch.note ? "warn" : STATE_TONE[batch.state]}>{t(batch.state, lang)}</Chip>
          {batch.note ? <span style={{ fontSize: 11, color: c.subtle }}>{batch.note}</span> : null}
        </span>
        <span className="vk-num" style={{ padding: "0 12px", fontSize: 12, color: c.subtle }}>
          {batch.uploaded}
        </span>
      </button>

      {batch.summary ? (
        <div
          style={{
            borderTop: `1px solid ${c.hairline}`,
            padding: "7px 12px 9px",
            display: "flex",
            alignItems: "center",
            gap: 14,
            fontSize: 11,
            color: c.muted,
            background: c.surface,
          }}
        >
          <span>
            <b style={{ fontWeight: 500 }}>{batch.summary.imported.toLocaleString("en-GB")}</b> rows imported
          </span>
          <span style={{ width: 1, height: 10, background: c.border }} />
          <span>
            <b style={{ fontWeight: 500 }}>{batch.summary.duplicates}</b> duplicates skipped
          </span>
          <span style={{ width: 1, height: 10, background: c.border }} />
          <span>
            <b style={{ fontWeight: 500 }}>{batch.summary.transfers}</b> internal transfers found
          </span>
          <span style={{ width: 1, height: 10, background: c.border }} />
          <span>
            <b style={{ fontWeight: 500 }}>{batch.summary.matches}</b> possible matches to your accounting data
          </span>
        </div>
      ) : null}
    </>
  );
}

function ValidationReport({ lang, file }: { lang: Locale; file: string }) {
  return (
    <Panel>
      <div style={{ padding: "14px 16px", display: "flex", flexDirection: "column", gap: 10, borderBottom: `1px solid ${c.border}` }}>
        <div style={{ display: "flex", alignItems: "center", gap: 8, flexWrap: "wrap" }}>
          <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke={c.danger} strokeWidth="1.5" strokeLinecap="round" aria-hidden="true">
            <circle cx="8" cy="8" r="6" />
            <path d="M8 4.8v3.6" />
            <path d="M8 11h.01" />
          </svg>
          <span style={{ fontSize: 15, fontWeight: 600 }}>Validation failed</span>
          <span style={{ fontSize: 13, color: c.muted, fontVariantNumeric: "tabular-nums" }}>
            {file} · 3,182 rows read · 3,179 valid ·{" "}
            <span style={{ color: c.danger, fontWeight: 500 }}>3 errors</span>
          </span>
          <span style={{ marginLeft: "auto", fontSize: 12, fontWeight: 500 }}>Nothing was imported.</span>
        </div>
        <div
          style={{
            background: c.dangerBg,
            border: `1px solid ${c.border}`,
            borderRadius: "var(--vk-radius-control)",
            padding: "9px 12px",
            fontSize: 12,
            fontVariantNumeric: "tabular-nums",
          }}
        >
          <span style={{ color: c.muted }}>Balance check</span>
          {"  "}
          opening {formatMinorUnits(BALANCE_CHECK.opening, lang)} + movements{" "}
          {formatMinorUnits(BALANCE_CHECK.movements, lang)} ≠ closing {formatMinorUnits(BALANCE_CHECK.closing, lang)}
          {"  "}
          <span style={{ fontWeight: 500 }}>difference {formatMinorUnits(BALANCE_CHECK.difference, lang)}</span>
        </div>
      </div>

      <div
        style={{
          display: "grid",
          gridTemplateColumns: "100px 260px 1fr",
          height: 28,
          alignItems: "center",
          background: c.sunken,
          borderBottom: `1px solid ${c.border}`,
          fontSize: 11,
          color: c.subtle,
        }}
      >
        <div style={{ padding: "0 16px" }}>File line</div>
        <div style={{ padding: "0 12px" }}>Code</div>
        <div style={{ padding: "0 12px" }}>What it means</div>
      </div>

      {VALIDATION_ERRORS.map((error, index) => (
        <div
          key={error.code}
          style={{
            display: "grid",
            gridTemplateColumns: "100px 260px 1fr",
            minHeight: 32,
            alignItems: "center",
            borderTop: index === 0 ? undefined : `1px solid ${c.hairline}`,
          }}
        >
          <div className="vk-mono" style={{ padding: "0 16px", fontSize: 12 }}>
            {error.line.toLocaleString("en-GB")}
          </div>
          <div className="vk-mono" style={{ padding: "0 12px", fontSize: 12, color: c.muted }}>
            {error.code}
          </div>
          <div style={{ padding: "6px 12px", fontSize: 13 }}>{lang === "ru" ? error.ru : error.en}</div>
        </div>
      ))}

      <div
        style={{
          borderTop: `1px solid ${c.border}`,
          padding: "10px 16px",
          display: "flex",
          alignItems: "center",
          gap: 10,
          background: c.surface,
          flexWrap: "wrap",
        }}
      >
        <div
          style={{
            display: "flex",
            alignItems: "center",
            gap: 6,
            height: 28,
            padding: "0 12px",
            border: `1px solid ${c.borderStrong}`,
            borderRadius: "var(--vk-radius-control)",
            fontSize: 13,
            fontWeight: 500,
          }}
        >
          Download the error list
        </div>
        <span style={{ fontSize: 12, color: c.subtle }}>
          Line numbers refer to the original file, so the rows can be found in Excel.
        </span>
        <span style={{ marginLeft: "auto", fontSize: 12, color: c.subtle }}>
          Correctness errors cannot be overridden.
        </span>
      </div>
    </Panel>
  );
}
