/**
 * The top bar: organisation, entity, period, language, theme. 48px.
 *
 * Organisation and entity render as **labels rather than pickers**. v1 gives a
 * user one organisation and an organisation one entity, so there is nothing to
 * pick; `docs/DESIGN.md` §8 still wants the entity slot present because "the
 * slot is cheap now and a retro-fit is not". They become pickers when
 * membership becomes plural, and not before -- a dropdown holding one item is a
 * control that teaches the reader it does nothing.
 *
 * Neither is read from the URL. Tenant context comes from the session
 * (`add-web-experience` design D7); the period does live in the URL, because it
 * is the thing a shared link is about.
 *
 * §14: no segmented controls and no pills. A field is a small-caps label and a
 * value; a choice is an underline.
 */
import { t } from "../i18n";
import type { Locale } from "../i18n";
import { formatPeriodRange } from "../ui/period";
import { setLocale, setTheme, type ThemeChoice } from "../ui/preferences";

function Field({ label, value }: Readonly<{ label: string; value: string }>) {
  return (
    <div className="flex items-baseline gap-1.5">
      <span className="text-2xs uppercase tracking-widest text-text-subtle">{label}</span>
      <span className="text-sm text-text">{value}</span>
    </div>
  );
}

function Divider() {
  return <span aria-hidden="true" className="h-4 w-px shrink-0 bg-border" />;
}

function Choice<T extends string>({
  label,
  values,
  current,
  onPick,
}: Readonly<{
  label: string;
  values: readonly T[];
  current: T;
  onPick: (v: T) => void;
}>) {
  return (
    <div className="flex items-baseline gap-2">
      <span className="sr-only">{label}</span>
      {values.map((v) => (
        <button
          key={v}
          type="button"
          onClick={() => onPick(v)}
          aria-pressed={current === v}
          className={
            current === v
              ? "border-b border-text pb-px text-2xs font-semibold uppercase tracking-widest text-text"
              : "border-b border-transparent pb-px text-2xs uppercase tracking-widest text-text-subtle"
          }
        >
          {v}
        </button>
      ))}
    </div>
  );
}

export function TopBar({
  organisation,
  entity,
  from,
  to,
  locale,
  theme,
}: Readonly<{
  organisation: string;
  entity: string;
  from: string;
  to: string;
  locale: Locale;
  theme: ThemeChoice;
}>) {
  return (
    <header className="flex h-12 shrink-0 flex-wrap items-center gap-4 border-b border-border bg-surface-raised px-6">
      <Field label={t("topbar.organisation", locale)} value={organisation} />
      <Divider />
      <Field label={t("topbar.entity", locale)} value={entity} />
      <Divider />
      <Field label={t("topbar.period", locale)} value={formatPeriodRange(from, to, locale)} />

      <div className="ml-auto flex items-center gap-5">
        <Choice
          label={t("topbar.language", locale)}
          values={["en", "ru"] as const}
          current={locale}
          onPick={setLocale}
        />
        <Divider />
        <Choice
          label={t("topbar.theme", locale)}
          values={["light", "dark", "system"] as const}
          current={theme}
          onPick={setTheme}
        />
      </div>
    </header>
  );
}
