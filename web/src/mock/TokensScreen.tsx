import { t } from "./data";
import type { Locale } from "./money";
import { Chip, Panel, ScreenHeading, c } from "./ui";

const SURFACES = [
  { name: "surface", value: c.surface, note: "99% 0.002 95" },
  { name: "surface-raised", value: c.raised, note: "100% 0 0" },
  { name: "surface-sunken", value: c.sunken, note: "97% 0.003 95" },
  { name: "border", value: c.border, note: "91% 0.004 95" },
  { name: "border-strong", value: c.borderStrong, note: "84% 0.005 95" },
  { name: "accent", value: c.accent, note: "48% 0.13 250 · 6.36:1" },
];

const INKS = [
  { name: "text", value: c.text, note: "24% · 15.98:1" },
  { name: "text-muted", value: c.muted, note: "48% · 6.35:1" },
  { name: "text-subtle", value: c.subtle, note: "56% · 4.52:1" },
  { name: "ok", value: c.ok, note: "52% 0.11 150 · 4.69:1" },
  { name: "warn", value: c.warn, note: "54% 0.12 75 · 4.59:1" },
  { name: "danger", value: c.danger, note: "52% 0.17 27 · 5.27:1" },
];

const TYPE = [
  { size: 24, weight: 600, sample: "−6,812.40" },
  { size: 20, weight: 600, sample: "Management P&L" },
  { size: 16, weight: 600, sample: "Отчёт о прибылях и убытках" },
  { size: 14, weight: 400, sample: "Body — the default in the application" },
  { size: 13, weight: 400, sample: "1,284,600.00 · table numerals, tabular" },
  { size: 12, weight: 400, sample: "Secondary — dates, counts, column meta" },
  { size: 11, weight: 400, sample: "Provenance — taxonomy v3 · ruleset v11" },
];

const NEVER_REMOVE = [
  "The basis label on every report line",
  "The reconciliation strip",
  "The reason a figure is blocked",
  "The engine layer and the confidence",
  "The original file line number in an error",
  "The counts in an import summary",
];

function Swatch({ name, value, note }: { name: string; value: string; note: string }) {
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 5 }}>
      <div style={{ height: 44, borderRadius: "var(--vk-radius-control)", border: `1px solid ${c.border}`, background: value }} />
      <span style={{ fontSize: 12, fontWeight: 500 }}>{name}</span>
      <span className="vk-mono" style={{ fontSize: 11, color: c.subtle }}>
        {note}
      </span>
    </div>
  );
}

export function TokensScreen({ lang }: { lang: Locale }) {
  return (
    <div style={{ padding: "20px 24px", display: "flex", flexDirection: "column", gap: 16, minWidth: 0, maxWidth: 1200 }}>
      <ScreenHeading
        title={t("tokens", lang)}
        meta="docs/DESIGN.md §3 · these values live in src/mock/mock.css until change 5.1a moves them into src/index.css"
      />

      <Panel style={{ padding: 16, display: "flex", flexDirection: "column", gap: 12 }}>
        <div style={{ fontSize: 15, fontWeight: 600 }}>Surfaces, lines and ink</div>
        <div style={{ display: "grid", gridTemplateColumns: "repeat(6, minmax(0, 1fr))", gap: 10 }}>
          {SURFACES.map((s) => (
            <Swatch key={s.name} {...s} />
          ))}
        </div>
        <div style={{ display: "grid", gridTemplateColumns: "repeat(6, minmax(0, 1fr))", gap: 10 }}>
          {INKS.map((s) => (
            <Swatch key={s.name} {...s} />
          ))}
        </div>
      </Panel>

      <div style={{ display: "grid", gridTemplateColumns: "repeat(2, minmax(0, 1fr))", gap: 16 }}>
        <Panel style={{ padding: 16, display: "flex", flexDirection: "column", gap: 12 }}>
          <div style={{ fontSize: 15, fontWeight: 600 }}>Type</div>
          <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
            {TYPE.map((row) => (
              <div key={row.size} style={{ display: "flex", alignItems: "baseline", gap: 12 }}>
                <span className="vk-mono" style={{ width: 74, fontSize: 11, color: c.subtle }}>
                  {row.size} / {row.weight}
                </span>
                <span style={{ fontSize: row.size, fontWeight: row.weight }}>{row.sample}</span>
              </div>
            ))}
          </div>
        </Panel>

        <Panel style={{ padding: 16, display: "flex", flexDirection: "column", gap: 12 }}>
          <div style={{ fontSize: 15, fontWeight: 600 }}>State vocabulary</div>
          <div style={{ display: "flex", flexDirection: "column", gap: 10 }}>
            <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
              <span style={{ fontSize: 11, color: c.subtle }}>Import batch</span>
              <div style={{ display: "flex", gap: 6, flexWrap: "wrap" }}>
                <Chip tone="outline">{t("pending", lang)}</Chip>
                <Chip tone="neutral">{t("parsing", lang)}</Chip>
                <Chip tone="danger">{t("rejected", lang)}</Chip>
                <Chip tone="ok">{t("imported", lang)}</Chip>
              </div>
            </div>
            <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
              <span style={{ fontSize: 11, color: c.subtle }}>Report readiness</span>
              <div style={{ display: "flex", gap: 6, flexWrap: "wrap" }}>
                <Chip tone="ok">{t("ready", lang)}</Chip>
                <Chip tone="warn">{t("partial", lang)}</Chip>
                <Chip tone="danger">{t("blocked", lang)}</Chip>
              </div>
            </div>
            <span style={{ fontSize: 12, color: c.muted }}>
              Switch the language in the top bar — every chip and every figure on every screen changes with it. The
              Russian words are a draft and need a native check.
            </span>
          </div>
        </Panel>
      </div>

      <Panel style={{ padding: 16, display: "flex", flexDirection: "column", gap: 12, background: c.sunken }}>
        <div style={{ fontSize: 15, fontWeight: 600 }}>What a minimal pass may never remove</div>
        <div style={{ display: "grid", gridTemplateColumns: "repeat(3, minmax(0, 1fr))", gap: "8px 20px" }}>
          {NEVER_REMOVE.map((item) => (
            <span key={item} style={{ fontSize: 13 }}>
              · {item}
            </span>
          ))}
        </div>
      </Panel>
    </div>
  );
}
