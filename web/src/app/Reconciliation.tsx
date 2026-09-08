/**
 * Opening + in + out + transfers = closing.
 *
 * `WORKFLOW.md` §5.1: "this is what proves to an accountant that nothing was
 * dropped". `DESIGN.md` §2 lists it among the things a minimal pass may never
 * remove, and it is the first thing such a pass deletes.
 *
 * The closing figure is derived in the data layer from the other four, never
 * stated -- a strip whose closing figure was typed in proves nothing.
 */
import { NO_DATA, exponentOf, formatMinorUnits } from "../money";
import { t } from "../i18n";
import type { Locale, MessageKey } from "../i18n";
import type { Money } from "../data/types";
import type { Reconciliation as Recon } from "../data/report";

function Cell({
  labelKey,
  value,
  locale,
  strong,
}: {
  labelKey: MessageKey;
  value: Money;
  locale: Locale;
  strong?: boolean;
}) {
  const exp = exponentOf(value.currencyCode);
  const text = exp === undefined ? NO_DATA : formatMinorUnits(value.minorUnits, exp, locale);
  return (
    <div className="flex min-w-32 flex-col gap-0.5">
      <span className="text-2xs uppercase tracking-widest text-text-subtle">
        {t(labelKey, locale)}
      </span>
      <span className={`tabular text-sm ${strong ? "font-semibold" : ""} text-figure`}>{text}</span>
    </div>
  );
}

export function Reconciliation({ recon, locale }: { recon: Recon; locale: Locale }) {
  return (
    <section className="mt-6 border-t-2 border-border-strong pt-4">
      <h2 className="text-2xs font-semibold uppercase tracking-widest text-text">
        {t("recon.title", locale)}
      </h2>
      <div className="mt-3 flex flex-wrap gap-x-8 gap-y-4">
        <Cell labelKey="recon.opening" value={recon.opening} locale={locale} />
        <Cell labelKey="recon.in" value={recon.moneyIn} locale={locale} />
        <Cell labelKey="recon.out" value={recon.moneyOut} locale={locale} />
        <Cell labelKey="recon.transfers" value={recon.transfers} locale={locale} />
        <Cell labelKey="recon.closing" value={recon.closing} locale={locale} strong />
      </div>
    </section>
  );
}
