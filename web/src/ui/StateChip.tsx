/**
 * The state vocabulary from `docs/DESIGN.md` §7: three axes, nine states, one
 * component.
 *
 * §7's rule is the whole point of the component existing: **state is never
 * communicated by colour alone.** Every chip carries a word and an icon. That
 * is what survives a greyscale print (§9 requires the report to print in black
 * and white), what a reader who cannot separate the hues relies on, and what
 * makes the monochrome chrome safe -- these three colours are the only hue in
 * the interface, so they must not be the only signal either.
 */
import type { Locale, MessageKey } from "../i18n";
import { t } from "../i18n";

/**
 * Tones map to tokens, never to a literal colour.
 *
 * No tinted pill. A word on a rounded, lightly-tinted background is the default
 * shape of every component library's badge, and it does two things this product
 * does not want: it spends a surface on a two-word label, and it makes nine
 * states look like nine buttons. The state reads as a mark plus a word set in
 * the state's own colour -- three channels (mark, word, hue) in the height of a
 * line of text, which is what a dense table can afford.
 */
type Tone = "ok" | "warn" | "danger" | "neutral";

export type BatchState = "pending" | "parsing" | "rejected" | "imported";
export type ReportState = "ready" | "partial" | "blocked";
export type RowState = "classified" | "needsReview" | "blocked";

type Shape = "check" | "alert" | "cross" | "dot" | "clock" | "half";

const TONE: Record<Tone, string> = {
  ok: "text-ok",
  warn: "text-warn",
  danger: "text-danger",
  neutral: "text-text-muted",
};

/**
 * `parsing` resolves to the neutral tone rather than an "info" one. §7 names an
 * `info` token that the palette does not carry, and adding a fourth state hue
 * to an interface whose only colour is state is a bigger decision than this
 * component should take. Pending and parsing are separated by their word and
 * their icon, which §7 requires anyway.
 */
const BATCH: Record<BatchState, [Tone, Shape]> = {
  pending: ["neutral", "clock"],
  parsing: ["neutral", "dot"],
  rejected: ["danger", "cross"],
  imported: ["ok", "check"],
};
const REPORT: Record<ReportState, [Tone, Shape]> = {
  ready: ["ok", "check"],
  partial: ["warn", "half"],
  blocked: ["danger", "alert"],
};
const ROW: Record<RowState, [Tone, Shape]> = {
  classified: ["neutral", "check"],
  needsReview: ["warn", "alert"],
  blocked: ["danger", "alert"],
};

const PATHS: Record<Shape, string> = {
  check: "M3 8.5 6.5 12 13 4",
  alert: "M8 3.5v5.5M8 12v.01",
  cross: "M4.5 4.5l7 7M11.5 4.5l-7 7",
  dot: "M8 8v.01",
  clock: "M8 4.5V8l2.5 1.5",
  half: "M8 2.5a5.5 5.5 0 0 1 0 11z",
};

function Icon({ shape }: { shape: Shape }) {
  const filled = shape === "half";
  return (
    <svg
      width="12"
      height="12"
      viewBox="0 0 16 16"
      fill={filled ? "currentColor" : "none"}
      stroke="currentColor"
      strokeWidth="1.6"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      className="shrink-0"
    >
      {shape === "clock" || shape === "dot" ? <circle cx="8" cy="8" r="5.5" /> : null}
      <path d={PATHS[shape]} />
    </svg>
  );
}

function Chip({ tone, shape, label }: { tone: Tone; shape: Shape; label: string }) {
  return (
    <span
      className={`inline-flex items-center gap-1.5 text-2xs font-medium uppercase tracking-wider whitespace-nowrap ${TONE[tone]}`}
    >
      <Icon shape={shape} />
      {label}
    </span>
  );
}

export function BatchStateChip({ state, locale }: { state: BatchState; locale?: Locale }) {
  const [tone, shape] = BATCH[state];
  return <Chip tone={tone} shape={shape} label={t(`state.batch.${state}` as MessageKey, locale)} />;
}

export function ReportStateChip({ state, locale }: { state: ReportState; locale?: Locale }) {
  const [tone, shape] = REPORT[state];
  return <Chip tone={tone} shape={shape} label={t(`state.report.${state}` as MessageKey, locale)} />;
}

export function RowStateChip({ state, locale }: { state: RowState; locale?: Locale }) {
  const [tone, shape] = ROW[state];
  return <Chip tone={tone} shape={shape} label={t(`state.row.${state}` as MessageKey, locale)} />;
}
