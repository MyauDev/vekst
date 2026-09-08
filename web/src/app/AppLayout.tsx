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
import { useQuery } from "@tanstack/react-query";
import { Code, ConnectError, createClient, type Transport } from "@connectrpc/connect";

import { IdentityService } from "../gen/vekst/v1/identity_pb";
import { reviewSummary } from "../data/review";
import { t } from "../i18n";
import { NO_DATA } from "../money";
import { defaultRange } from "../ui/period";
import { useLocale, useTheme } from "../ui/preferences";
import { ErrorState, Loading } from "../ui/feedback";
import { Rail } from "./Rail";
import { TopBar } from "./TopBar";

async function signOut() {
  // A plain fetch, not an RPC: sign-out clears a cookie in the same response
  // that ends the session, and it belongs with the other two auth routes.
  await fetch("/auth/logout", { method: "POST" });
  window.location.assign("/");
}

export function AppLayout({
  transport,
  children,
  onUnauthenticated,
}: {
  transport: Transport;
  children: ReactNode;
  onUnauthenticated: () => ReactNode;
}) {
  const [locale] = useLocale();
  const [theme] = useTheme();
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
    if (ConnectError.from(error).code === Code.Unauthenticated) return onUnauthenticated();
    return (
      <div className="mx-auto max-w-lg p-8">
        <ErrorState message={ConnectError.from(error).message} />
      </div>
    );
  }
  if (!data.user) return onUnauthenticated();

  const range = defaultRange();

  return (
    <div className="flex min-h-dvh">
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
              className="w-fit text-2xs uppercase tracking-widest text-text-subtle"
            >
              {t("signedIn.signOut", locale)}
            </button>
          </div>
        }
      />

      <div className="flex min-w-0 grow flex-col">
        <TopBar
          // NO_DATA, not a guessed name. The `User` message carries no
          // organisation until §9.1 extends it, and this product's own
          // convention is that a dash means "nothing was loaded" while a zero
          // means "nothing happened". Inventing a label here would be the one
          // kind of wrong this interface must never be.
          organisation={NO_DATA}
          entity={NO_DATA}
          from={range.from}
          to={range.to}
          locale={locale}
          theme={theme}
        />
        <main className="min-w-0 grow px-6 py-5">{children}</main>
      </div>
    </div>
  );
}
