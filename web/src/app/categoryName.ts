/**
 * What a leaf category is called, as opposed to what the wire sent.
 *
 * `categories.name` is stored in whatever language the source data was in --
 * `eval/`'s own README is explicit that these are real vendor and department
 * names out of the founder's accounting, not prose this product authored, so
 * the stored value stays English and never gets a rename migration. Category
 * names are `lineName.ts`'s own problem one level down the tree, and the same
 * answer applies: the name shown on screen is resolved here, per locale, from
 * the code, never from the wire's own label.
 *
 * Unknown codes fall back to the wire's label, the same call `lineName` and
 * `i18n.ts`'s own `authErrorMessage` make for a code this build has never
 * heard of -- an org-scoped leaf a customer's own data adds after this
 * catalogue was written renders as whatever the backend called it, which is
 * worse than a translation and much better than blank.
 */
import { t } from "../i18n";
import type { Locale, MessageKey } from "../i18n";

/** Every leaf code the shared skeleton and the industry template seed --
 *  `eval/out/taxonomy.csv`'s own leaf rows, both scopes. */
const NAMED = new Set([
  "0101", "02", "030101",
  "0401010101", "0401010201", "0401010202", "0401010203", "0401010204", "0401010205", "0401010206",
  "0401020101", "0401020201", "0401020202", "0401020203", "0401020204", "0401020205", "0401020206",
  "0401020207", "0401020208", "0401020209", "0401020210", "0401020211", "0401020212",
  "0401030101",
  "0401040101", "0401040102", "0401040103", "0401040104", "0401040105",
  "0401050101", "0401050102",
  "0402010101", "0402010201", "0402010202", "0402010203", "0402010204", "0402010205", "0402010206", "0402010207",
  "0402020101", "0402020201", "0402020202",
  "0403", "0404",
  "0405010101", "0405010102", "0405010201", "0405010202", "0405010203",
  "0406",
  "050101", "050102",
  "05010301", "05010302", "05010303", "05010304", "05010305", "050104",
  "050201", "050202",
  "060101", "060102", "060201",
  "07", "08", "09",
]);

/**
 * The readable name for a leaf category. `fallback` is the wire's own
 * `name`, used for any code this build does not know.
 */
export function categoryName(code: string, fallback: string, locale: Locale): string {
  if (!NAMED.has(code)) return fallback;
  return t(`category.${code}` as MessageKey, locale);
}
