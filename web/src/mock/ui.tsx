/* Shared chrome for the mock screens. Inline styles on purpose: the mock must
   not depend on the app's Tailwind entry. Tokens come from mock.css. */
import type { CSSProperties, ReactNode } from "react";

import type { Locale } from "./money";
import { ORG, t } from "./data";

export const c = {
  surface: "var(--vk-surface)",
  raised: "var(--vk-surface-raised)",
  sunken: "var(--vk-surface-sunken)",
  text: "var(--vk-text)",
  muted: "var(--vk-text-muted)",
  subtle: "var(--vk-text-subtle)",
  inverse: "var(--vk-text-inverse)",
  border: "var(--vk-border)",
  borderStrong: "var(--vk-border-strong)",
  hairline: "var(--vk-hairline)",
  accent: "var(--vk-accent)",
  accentSurface: "var(--vk-accent-surface)",
  focus: "var(--vk-focus)",
  ok: "var(--vk-ok)",
  okBg: "var(--vk-ok-surface)",
  warn: "var(--vk-warn)",
  warnBg: "var(--vk-warn-surface)",
  danger: "var(--vk-danger)",
  dangerBg: "var(--vk-danger-surface)",
} as const;

export type Screen = "reports" | "imports" | "review" | "tokens";

export type Tone = "neutral" | "outline" | "ok" | "warn" | "danger" | "accent";

const TONES: Record<Tone, CSSProperties> = {
  neutral: { background: c.sunken, color: c.muted },
  outline: { border: `1px solid ${c.border}`, color: c.muted },
  ok: { background: c.okBg, color: c.ok },
  warn: { background: c.warnBg, color: c.warn },
  danger: { background: c.dangerBg, color: c.danger },
  accent: { background: c.accentSurface, color: c.accent },
};

export function Chip({ tone = "outline", children }: { tone?: Tone; children: ReactNode }) {
  return (
    <span
      style={{
        display: "inline-flex",
        alignItems: "center",
        gap: 5,
        height: 20,
        padding: "0 7px",
        borderRadius: "var(--vk-radius-control)",
        fontSize: 11,
        fontWeight: 500,
        whiteSpace: "nowrap",
        ...TONES[tone],
      }}
    >
      {children}
    </span>
  );
}

export function Panel({ children, style }: { children: ReactNode; style?: CSSProperties }) {
  return (
    <div
      style={{
        border: `1px solid ${c.border}`,
        borderRadius: "var(--vk-radius-panel)",
        background: c.raised,
        overflow: "hidden",
        ...style,
      }}
    >
      {children}
    </div>
  );
}

export function Key({ children }: { children: ReactNode }) {
  return (
    <span
      className="vk-mono"
      style={{
        minWidth: 18,
        height: 18,
        display: "inline-flex",
        alignItems: "center",
        justifyContent: "center",
        padding: "0 4px",
        border: `1px solid ${c.borderStrong}`,
        borderBottomWidth: 2,
        borderRadius: "var(--vk-radius-control)",
        background: c.raised,
        color: c.muted,
        fontSize: 11,
      }}
    >
      {children}
    </span>
  );
}

const ICONS: Record<Screen, ReactNode> = {
  imports: (
    <>
      <path d="M8 2v7" />
      <path d="M5.2 6.4 8 9.2l2.8-2.8" />
      <path d="M2.5 10.5v2a1 1 0 0 0 1 1h9a1 1 0 0 0 1-1v-2" />
    </>
  ),
  review: (
    <>
      <path d="M2.5 4.5h6" />
      <path d="M2.5 8h6" />
      <path d="M2.5 11.5h4" />
      <path d="M10.5 10.6 12 12.1l2.5-3" />
    </>
  ),
  reports: (
    <>
      <rect x="2.5" y="2.5" width="11" height="11" rx="1.5" />
      <path d="M2.5 6.5h11" />
      <path d="M6.5 6.5v7" />
    </>
  ),
  tokens: (
    <>
      <circle cx="6" cy="6" r="3.5" />
      <circle cx="10" cy="10" r="3.5" />
    </>
  ),
};

function Icon({ screen }: { screen: Screen }) {
  return (
    <svg
      width="16"
      height="16"
      viewBox="0 0 16 16"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.4"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      {ICONS[screen]}
    </svg>
  );
}

export function Rail({
  screen,
  onNavigate,
  lang,
  pending,
}: {
  screen: Screen;
  onNavigate: (next: Screen) => void;
  lang: Locale;
  pending: number;
}) {
  const items: readonly Screen[] = ["imports", "review", "reports", "tokens"];
  return (
    <nav
      style={{
        width: 208,
        flexShrink: 0,
        borderRight: `1px solid ${c.border}`,
        display: "flex",
        flexDirection: "column",
      }}
    >
      <div
        style={{
          height: 48,
          display: "flex",
          alignItems: "center",
          padding: "0 16px",
          borderBottom: `1px solid ${c.border}`,
          fontSize: 16,
          fontWeight: 600,
          letterSpacing: "-0.015em",
        }}
      >
        Veekst
      </div>
      <div style={{ display: "flex", flexDirection: "column", gap: 2, padding: "12px 8px" }}>
        {items.map((item) => {
          const active = item === screen;
          return (
            <button
              key={item}
              type="button"
              onClick={() => onNavigate(item)}
              aria-current={active ? "page" : undefined}
              style={{
                display: "flex",
                alignItems: "center",
                gap: 8,
                height: 32,
                padding: "0 8px",
                borderRadius: "var(--vk-radius-control)",
                background: active ? c.sunken : "transparent",
                color: active ? c.text : c.muted,
                fontWeight: active ? 500 : 400,
                fontSize: 13,
              }}
            >
              <Icon screen={item} />
              <span>{item === "tokens" ? t("tokens", lang) : t(item, lang)}</span>
              {item === "review" && pending > 0 ? (
                <span style={{ marginLeft: "auto" }}>
                  <Chip tone="warn">{pending}</Chip>
                </span>
              ) : null}
            </button>
          );
        })}
      </div>
      <div
        style={{
          marginTop: "auto",
          padding: "12px 16px",
          borderTop: `1px solid ${c.border}`,
          display: "flex",
          alignItems: "center",
          gap: 8,
        }}
      >
        <div
          style={{
            width: 24,
            height: 24,
            borderRadius: 12,
            background: c.sunken,
            border: `1px solid ${c.border}`,
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            fontSize: 11,
            fontWeight: 500,
            color: c.muted,
          }}
        >
          AG
        </div>
        <div style={{ display: "flex", flexDirection: "column" }}>
          <span style={{ fontSize: 12 }}>A. Gorshkov</span>
          <span style={{ fontSize: 11, color: c.subtle }}>Approver</span>
        </div>
      </div>
    </nav>
  );
}

function Selector({ children }: { children: ReactNode }) {
  return (
    <div
      style={{
        display: "flex",
        alignItems: "center",
        gap: 6,
        height: 28,
        padding: "0 10px",
        border: `1px solid ${c.border}`,
        borderRadius: "var(--vk-radius-control)",
        fontSize: 13,
      }}
    >
      <span>{children}</span>
      <svg width="10" height="10" viewBox="0 0 10 10" fill="none" stroke={c.subtle} strokeWidth="1.3" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
        <path d="M2.5 4 5 6.5 7.5 4" />
      </svg>
    </div>
  );
}

export function TopBar({ lang, onLang }: { lang: Locale; onLang: (next: Locale) => void }) {
  return (
    <div
      style={{
        height: 48,
        flexShrink: 0,
        borderBottom: `1px solid ${c.border}`,
        background: c.raised,
        display: "flex",
        alignItems: "center",
        gap: 8,
        padding: "0 24px",
      }}
    >
      <Selector>{ORG}</Selector>
      <Selector>Entity: {ORG}</Selector>
      <Selector>Jan – Aug 2026</Selector>
      <div
        style={{
          marginLeft: "auto",
          display: "flex",
          alignItems: "center",
          height: 28,
          border: `1px solid ${c.border}`,
          borderRadius: "var(--vk-radius-control)",
          overflow: "hidden",
          fontSize: 12,
        }}
      >
        {(["en", "ru"] as const).map((code) => (
          <button
            key={code}
            type="button"
            onClick={() => onLang(code)}
            aria-pressed={lang === code}
            style={{
              padding: "0 9px",
              height: 26,
              background: lang === code ? c.sunken : "transparent",
              color: lang === code ? c.text : c.subtle,
              fontWeight: lang === code ? 500 : 400,
              borderLeft: code === "ru" ? `1px solid ${c.border}` : undefined,
            }}
          >
            {code.toUpperCase()}
          </button>
        ))}
      </div>
    </div>
  );
}

export function ScreenHeading({ title, meta, right }: { title: string; meta?: string; right?: ReactNode }) {
  return (
    <div style={{ display: "flex", alignItems: "flex-start", gap: 12 }}>
      <div style={{ display: "flex", flexDirection: "column", gap: 3 }}>
        <div style={{ fontSize: 20, fontWeight: 600, letterSpacing: "-0.01em" }}>{title}</div>
        {meta ? <div style={{ fontSize: 11, color: c.subtle, fontVariantNumeric: "tabular-nums" }}>{meta}</div> : null}
      </div>
      {right ? <div style={{ marginLeft: "auto", display: "flex", alignItems: "center", gap: 8 }}>{right}</div> : null}
    </div>
  );
}
