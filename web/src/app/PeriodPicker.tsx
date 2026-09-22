/**
 * The month range a report covers.
 *
 * Second attempt, after two browser-native ones. `<input type="month">`'s
 * year is a plain spinner, one click per year; `<input type="date">` gave a
 * real calendar but made a person pick a day for a value that is only ever a
 * month. `react-datepicker`'s `showMonthYearPicker` is a grid of the current
 * year's twelve months, and its header arrows step by a whole *year* in that
 * mode -- Jan 2026 to Jan 2023 is three clicks, not thirty-six. MIT-licensed,
 * actively maintained, and the one open-source month/year picker that
 * matched this product's own grain (`ui/period.ts`'s comment) rather than a
 * day-grid pressed into service for one.
 *
 * A controlled pair of fields, not a URL-bound one: `TopBar` renders this on
 * every screen now (`ui/periodPreference.ts`), and the Report screen renders
 * the very same component bound to its own URL search params, so the two
 * have to agree on what "changed" means without either one importing the
 * other's idea of where the range lives. `onChange` is that agreement --
 * each caller decides whether a change means `navigate()`, `setPeriodRange`,
 * or (in `ReportScreen`'s case) both.
 *
 * `toLocalMonth`/`fromLocalMonth` construct and read `Date`s with the local
 * getters/constructor throughout, deliberately not `ui/period.ts`'s UTC ones:
 * `react-datepicker` navigates, highlights "today" and compares `minDate`/
 * `maxDate` in the browser's local time zone, so feeding it a UTC-built date
 * and reading it back with local getters (or the reverse) is exactly the
 * class of bug that shows the wrong month to anyone west of UTC at certain
 * hours. The two conventions never meet: this file's dates never leave it
 * except as the "YYYY-MM" strings the rest of the app already uses.
 */
import DatePicker, { registerLocale } from "react-datepicker";
import { ru } from "date-fns/locale/ru";
import "react-datepicker/dist/react-datepicker.css";

import { t } from "../i18n";
import type { Locale } from "../i18n";

registerLocale("ru", ru);

function toLocalMonth(month: string): Date {
  const [y, m] = month.split("-").map(Number);
  return new Date(y!, m! - 1, 1);
}

function fromLocalMonth(date: Date): string {
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, "0")}`;
}

const inputClass =
  "w-28 rounded border border-border-strong bg-surface px-2 py-1 text-sm text-text focus:border-text";

function MonthField({
  labelKey,
  value,
  onChange,
  minMonth,
  maxMonth,
  locale,
}: Readonly<{
  labelKey: "report.period.from" | "report.period.to";
  value: string;
  onChange: (month: string) => void;
  minMonth?: string;
  maxMonth?: string;
  locale: Locale;
}>) {
  return (
    <label className="flex items-center gap-1.5 text-2xs uppercase tracking-widest text-text-subtle">
      {t(labelKey, locale)}
      <DatePicker
        className={inputClass}
        selected={toLocalMonth(value)}
        onChange={(date: Date | null) => date && onChange(fromLocalMonth(date))}
        showMonthYearPicker
        dateFormat="LLL yyyy"
        locale={locale === "ru" ? "ru" : undefined}
        minDate={minMonth ? toLocalMonth(minMonth) : undefined}
        maxDate={maxMonth ? toLocalMonth(maxMonth) : undefined}
      />
    </label>
  );
}

export function PeriodPicker({
  from,
  to,
  locale,
  onChange,
}: Readonly<{
  from: string;
  to: string;
  locale: Locale;
  onChange: (range: { from: string; to: string }) => void;
}>) {
  return (
    <div className="vk-datepicker flex flex-wrap items-center gap-x-4 gap-y-1">
      <MonthField
        labelKey="report.period.from"
        value={from}
        maxMonth={to}
        locale={locale}
        onChange={(value) => onChange({ from: value, to })}
      />
      <MonthField
        labelKey="report.period.to"
        value={to}
        minMonth={from}
        locale={locale}
        onChange={(value) => onChange({ from, to: value })}
      />
    </div>
  );
}
