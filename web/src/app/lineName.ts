/**
 * What a P&L line is called, as opposed to what it is keyed by.
 *
 * The wire sends the taxonomy's own label, and the taxonomy's own label is an
 * abbreviation: `core/internal/report/pnl.go`'s `sectionLabels` is "NET SALES",
 * "CS", "OCS", "OPEX", "OIE", "FR", "CIT", and its `Chain` is "GM", "NM", "CM",
 * "IBT", "NI". That is deliberate on the backend's side — `00014_pnl_sections`
 * says it outright: a section is identified by its code because "renaming NET
 * SALES would move a figure", so the stored name is structure, not prose. It is
 * also unreadable to the person this product is for. An owner looking at their
 * own business does not know what OCS or IBT is, and a table of twelve rows
 * where eight of them are initialisms is a table nobody checks.
 *
 * So the name is resolved here, per locale, from the code — never from the
 * label, which is a string the backend is free to change and which exists in
 * one language. The abbreviation is not thrown away: `PnlTable` prints it beside
 * the name, because an accountant reads down the codes and would lose their
 * place without them.
 *
 * Unknown codes fall back to the wire's label. A taxonomy that grows a section
 * this build has never heard of renders as whatever the backend called it,
 * which is worse than a translation and much better than blank — the same call
 * `i18n.ts`'s own `authErrorMessage` makes for a code from the future.
 */
import { t } from "../i18n";
import type { Locale, MessageKey } from "../i18n";
import type { BucketKind } from "../data/report";

/** Every code `report.Order` and `report.NonPNLSections` can produce. */
const NAMED = new Set([
  "01", "02", "03", "04", "05", "06", "07", "08", "09",
  "91", "92", "93", "94", "95",
]);

/**
 * The readable name for a P&L line. `fallback` is the wire's own label, used
 * for any code this build does not know.
 */
export function lineName(code: string, fallback: string, locale: Locale): string {
  if (!NAMED.has(code)) return fallback;
  return t(`line.${code}` as MessageKey, locale);
}

/** Case and spacing are not a difference between two names. */
function same(a: string, b: string): boolean {
  const flat = (x: string) => x.toLowerCase().replace(/\s+/g, " ").trim();
  return flat(a) === flat(b);
}

/**
 * The abbreviation to print beside the name, and "" where there is nothing to
 * add. Two cases produce nothing, for the same reason: the cell would say one
 * thing twice.
 *
 * An unknown code has no translation, so its label *is* the name already. And
 * some labels are not abbreviations at all — the wire's own label for '01' is
 * "NET SALES", which is the name in capitals, not a code that anchors anything.
 * The ones worth keeping are the ones a reader could not reconstruct: "CS",
 * "OCS", "OIE", "IBT".
 */
export function lineAbbreviation(code: string, label: string, locale: Locale): string {
  if (!NAMED.has(code)) return "";
  return same(label, lineName(code, label, locale)) ? "" : label;
}

/**
 * The four exclusion buckets, which have no human name on the wire at all --
 * only a kind. `PnlTable` prints these under the table and the drill-down
 * panel opens onto them by the same name, so the mapping lives here with the
 * line names rather than in whichever component happened to need it first.
 */
export const BUCKET_KEY: Record<BucketKind, MessageKey> = {
  unclassified: "report.bucket.unclassified",
  non_pnl: "report.bucket.non_pnl",
  unallocated: "report.bucket.unallocated",
  other_basis: "report.bucket.other_basis",
};

/**
 * What a drill-down is titled. `ListLineTransactionsRequest.line` takes two
 * closed, disjoint vocabularies -- a line's code ('01', '91') or a bucket's
 * kind ('unclassified') -- and the panel is opened by URL, so it arrives with
 * one of the two and no idea which. Both have to resolve to something a person
 * can read; a bucket in particular has no label on the wire to fall back to,
 * which is why a panel opened on one used to be headed "unclassified".
 */
export function cellName(id: string, fallback: string, locale: Locale): string {
  if (id in BUCKET_KEY) return t(BUCKET_KEY[id as BucketKind], locale);
  return lineName(id, fallback, locale);
}
