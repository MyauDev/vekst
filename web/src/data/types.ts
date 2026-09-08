/**
 * The shapes the screens read.
 *
 * These are written to match the proto messages that will eventually carry
 * them, so connecting the real backend replaces a module body and touches no
 * component (`add-web-experience` design D1). Two rules make that true:
 *
 *   - **Money is an `int64` minor-unit string plus an ISO-4217 code**, never a
 *     JavaScript `number`, which cannot hold an `int64` exactly. Identical to
 *     the generated `vekst.type.v1.Money`.
 *   - **States are codes, never sentences.** The backend returns codes and the
 *     client owns every sentence, so a state that arrives as text has already
 *     lost the translation.
 */

/** Structurally identical to the generated `Money`. */
export interface Money {
  minorUnits: string;
  currencyCode: string;
}

/** `YYYY-MM`. Months are the grain of every report this product makes. */
export type Period = string;

/**
 * Where a batch came from. Chosen before the file is read, because it decides
 * the accounting basis and is not a property the parser can recover.
 */
export type SourceKind = "ledger" | "bank";

/**
 * Derived from `source_kind`, never chosen by a human. Bank-sourced lines are
 * cash-basis; ledger-sourced lines are accrual. A line drawing on both without
 * a confirmed match is neither, and is blocked rather than guessed.
 */
export type Basis = "cash" | "accrual";

/**
 * The three strings that make a March report reproduce in June. An accountant
 * will ask, and a report that cannot answer is a report that cannot be audited.
 */
export interface Provenance {
  taxonomyVersion: string;
  rulesetVersion: string;
  engineVersion: string;
}

/** Which engine layer decided a classification. L0 is deterministic; L2 is not. */
export type EngineLayer = "L0" | "L0.5" | "L1" | "L2" | "L3";
