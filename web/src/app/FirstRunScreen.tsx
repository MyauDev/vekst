/**
 * What a signed-in person with no organisation sees.
 *
 * Rendered by `AppLayout` in place of the application shell's children --
 * first-run is a state of the shell, not a route, because a route would be
 * reachable by a person who already has an organisation (design §4.2).
 *
 * Four fields, one button. The market select fixes country and base currency
 * together as one of the three Demo pilot pairs (BY/BYN, KZ/KZT, PL/PLN):
 * `CreateOrganizationRequest` carries them as two independent fields, but
 * offering them as two independent pickers would let a person choose a
 * combination the backend refuses, for no benefit -- there is no `currencies`
 * table's worth of countries to pick from yet, only these three.
 */
import { useState, type FormEvent, type ReactNode } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";

import { createOrganisation, CreateOrganisationError } from "../data/org";
import { authErrorMessage, t } from "../i18n";
import type { Locale, MessageKey } from "../i18n";
import { ErrorState } from "../ui/feedback";

const MARKETS = [
  { country: "BY", baseCurrency: "BYN", labelKey: "firstRun.market.by" satisfies MessageKey },
  { country: "KZ", baseCurrency: "KZT", labelKey: "firstRun.market.kz" satisfies MessageKey },
  { country: "PL", baseCurrency: "PLN", labelKey: "firstRun.market.pl" satisfies MessageKey },
] as const;

function Field({
  labelKey,
  locale,
  children,
}: Readonly<{ labelKey: MessageKey; locale: Locale; children: ReactNode }>) {
  return (
    <label className="flex flex-col gap-1.5">
      <span className="text-2xs uppercase tracking-widest text-text-subtle">{t(labelKey, locale)}</span>
      {children}
    </label>
  );
}

const inputClass = "rounded border border-border-strong bg-surface px-3 py-2 text-sm text-text focus:border-text";

export function FirstRunScreen({ locale }: Readonly<{ locale: Locale }>) {
  const qc = useQueryClient();
  const [name, setName] = useState("");
  const [entityName, setEntityName] = useState("");
  const [market, setMarket] = useState<(typeof MARKETS)[number]>(MARKETS[0]);

  const mutation = useMutation({
    mutationFn: () =>
      createOrganisation({
        name,
        entityName,
        country: market.country,
        baseCurrency: market.baseCurrency,
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["currentUser"] }),
  });

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    if (!name.trim() || !entityName.trim() || mutation.isPending) return;
    mutation.mutate();
  }

  const errorMessage = mutation.error
    ? mutation.error instanceof CreateOrganisationError
      ? authErrorMessage(mutation.error.code, locale)
      : t("error.unknown", locale)
    : null;

  return (
    <div className="mx-auto flex max-w-md flex-col gap-6 p-8">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">{t("firstRun.heading", locale)}</h1>
        <p className="mt-2 text-sm text-text-muted">{t("firstRun.blurb", locale)}</p>
      </div>

      <form onSubmit={onSubmit} className="flex flex-col gap-4">
        <Field labelKey="firstRun.name" locale={locale}>
          <input
            className={inputClass}
            value={name}
            onChange={(e) => setName(e.target.value)}
            required
            autoFocus
          />
        </Field>

        <Field labelKey="firstRun.entityName" locale={locale}>
          <input
            className={inputClass}
            value={entityName}
            onChange={(e) => setEntityName(e.target.value)}
            required
          />
        </Field>

        <Field labelKey="firstRun.market" locale={locale}>
          <select
            className={inputClass}
            value={market.country}
            onChange={(e) => {
              const next = MARKETS.find((m) => m.country === e.target.value);
              if (next) setMarket(next);
            }}
          >
            {MARKETS.map((m) => (
              <option key={m.country} value={m.country}>
                {t(m.labelKey, locale)}
              </option>
            ))}
          </select>
        </Field>

        {errorMessage ? <ErrorState message={errorMessage} /> : null}

        <button
          type="submit"
          disabled={mutation.isPending}
          className="mt-2 w-fit rounded border border-text bg-text px-5 py-2 text-sm font-medium text-surface hover:bg-text-muted disabled:opacity-50"
        >
          {t(mutation.isPending ? "firstRun.submitting" : "firstRun.submit", locale)}
        </button>
      </form>
    </div>
  );
}
