/**
 * Register B, and the gate in front of it.
 *
 * Sign-in is checked **here, once, for the whole subtree** rather than screen by
 * screen. `add-web-experience` §2.8 and the spec requirement it satisfies: a
 * screen added under `/app` is protected by having been added, not by somebody
 * remembering to protect it. The previous arrangement put the check inside the
 * one screen that existed, which was correct while one screen existed.
 */
import type { ReactNode } from "react";
import { useMatch, useNavigate } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { Code, ConnectError, createClient, type Transport } from "@connectrpc/connect";

import { IdentityService } from "../gen/vekst/v1/identity_pb";
import { signOut } from "../data/auth";
import { reviewSummary } from "../data/review";
import { setSession } from "../data/session";
import { t } from "../i18n";
import { useLocale, useTheme } from "../ui/preferences";
import { usePeriodRange, type PeriodRange } from "../ui/periodPreference";
import { ErrorState, Loading } from "../ui/feedback";
import { FirstRunScreen } from "./FirstRunScreen";
import { Rail } from "./Rail";
import { TopBar } from "./TopBar";

export function AppLayout({
  transport,
  children,
  unauthenticated,
}: Readonly<{
  transport: Transport;
  children: ReactNode;
  unauthenticated: ReactNode;
}>) {
  const [locale] = useLocale();
  const [theme] = useTheme();
  const [rememberedRange, setPeriodRange] = usePeriodRange();
  // While the Report screen is the active route, its own URL search params
  // are the range actually on screen -- which a shared link can set to
  // something the remembered preference below knows nothing about, and the
  // header has to show what is being viewed, not a stale preference beside
  // it. Off that route there is nothing on screen to reflect, so the
  // remembered range is the only honest answer.
  const reportMatch = useMatch({ from: "/app/reports/pnl", shouldThrow: false });
  const range = reportMatch ? { from: reportMatch.search.from, to: reportMatch.search.to } : rememberedRange;
  const navigate = useNavigate();
  function onPeriodChange(next: PeriodRange) {
    setPeriodRange(next);
    // The match's own current search, not a `prev` updater -- reportMatch
    // already holds it, strongly typed, and reading through it here keeps
    // the tab (table/charts) untouched without threading it as a parameter.
    if (reportMatch) {
      void navigate({
        to: "/app/reports/pnl",
        search: { ...next, view: reportMatch.search.view },
        replace: true,
      });
    }
  }
  const { data, error, isPending } = useQuery({
    queryKey: ["currentUser"],
    queryFn: () => createClient(IdentityService, transport).getCurrentUser({}),
    retry: false,
  });
  // The rail's badge is the only one in the interface, and it has to follow the
  // queue rather than a number fetched once: approving a group empties it, and a
  // badge that still says 2 is a badge nobody trusts again.
  const review = useQuery({ queryKey: ["reviewSummary"], queryFn: reviewSummary });

  if (isPending) {
    return (
      <div className="mx-auto max-w-lg p-8">
        <Loading label="…" rows={2} />
      </div>
    );
  }

  // Unauthenticated is the ordinary state of a browser that has not signed in,
  // and core reports it with that code by design. Anything else is a real
  // failure and says so rather than pretending nobody is signed in.
  if (error) {
    if (ConnectError.from(error).code === Code.Unauthenticated) return unauthenticated;
    return (
      <div className="mx-auto max-w-lg p-8">
        <ErrorState message={ConnectError.from(error).message} />
      </div>
    );
  }
  if (!data.user) return unauthenticated;

  // Empty is the first-run signal, and a fact rather than an error: a person
  // who has just signed in for the first time has not failed at anything.
  // Rendered in place of the shell's children -- not a route, because a route
  // would be reachable by a person who already has an organisation -- and no
  // tenant-scoped data call happens: the session is never set, so any screen
  // that tried would throw from requireSession() rather than send an empty
  // organization_id.
  if (data.organisations.length === 0) {
    return <FirstRunScreen locale={locale} />;
  }

  // v1 gives a person exactly one organisation (add-web-experience
  // design.md:147; already_a_member refuses a second), so the first is the
  // only one. Set once per render, synchronously, before any screen below
  // renders -- not in an effect, whose ordering (children run before
  // parents) would let a child's own query fire first.
  const organisation = data.organisations[0]!;
  const entity = organisation.entities[0];
  setSession({
    orgId: organisation.id,
    entityId: entity?.id ?? "",
    baseCurrency: organisation.baseCurrency,
  });

  return (
    <div className="flex min-h-dvh">
      {/* Visible only on keyboard focus. The review queue is keyboard-first
          (DESIGN.md §9); skipping the rail and top bar matters most exactly
          where a mouse is least likely to be in use. */}
      <a
        href="#main-content"
        className="sr-only focus:not-sr-only focus:absolute focus:left-4 focus:top-4 focus:z-50 focus:border focus:border-border-strong focus:bg-surface-raised focus:px-4 focus:py-2 focus:text-sm focus:font-medium focus:text-text"
      >
        {t("a11y.skipToContent", locale)}
      </a>

      <Rail
        locale={locale}
        awaitingReview={review.data?.groups ?? 0}
        account={
          <div className="flex flex-col gap-1">
            <span className="truncate text-2xs text-text-muted">
              {data.user.email || data.user.name || data.user.id}
            </span>
            <button
              type="button"
              onClick={signOut}
              className="w-fit text-2xs uppercase tracking-widest text-text-subtle hover:text-text active:text-text-muted"
            >
              {t("signedIn.signOut", locale)}
            </button>
          </div>
        }
      />

      <div className="flex min-w-0 grow flex-col">
        <TopBar
          organisation={organisation.name}
          entity={entity?.name ?? ""}
          from={range.from}
          to={range.to}
          onPeriodChange={onPeriodChange}
          locale={locale}
          theme={theme}
        />
        <main id="main-content" className="min-w-0 grow bg-surface-sunken px-10 py-8">
          {children}
        </main>
      </div>
    </div>
  );
}
