/**
 * The identity summary: name/email, sign out. Deliberately not
 * `AccountCard*Screen*.tsx` -- `data/data.test.ts`'s "no screen imports a
 * transport or a generated client" draws its boundary at screens exactly so
 * a small, honest exception like this one has somewhere to live, the same
 * reasoning that already lets `AppLayout` fetch the session for the rail.
 *
 * `HomeScreen` resolves `transport` itself via `useRouteContext` (a router
 * hook, not a connect import) and passes it in here, so the screen file
 * itself still imports nothing from `@connectrpc` or `../gen/`.
 */
import { useQuery } from "@tanstack/react-query";
import { createClient } from "@connectrpc/connect";
import type { Transport } from "@connectrpc/connect";

import { IdentityService } from "../gen/vekst/v1/identity_pb";
import { signOut } from "../data/auth";
import { t } from "../i18n";
import type { Locale } from "../i18n";
import { NO_DATA } from "../money";

export function AccountCard({
  transport,
  locale,
}: Readonly<{ transport: Transport; locale: Locale }>) {
  // Same queryKey AppLayout already uses for the rail's own account slot --
  // react-query dedupes this rather than firing a second request.
  const { data } = useQuery({
    queryKey: ["currentUser"],
    queryFn: () => createClient(IdentityService, transport).getCurrentUser({}),
    retry: false,
  });

  return (
    <section className="flex flex-col gap-3 rounded-panel border border-border bg-surface-raised p-6">
      <h2 className="text-2xs font-semibold uppercase tracking-widest text-text-subtle">
        {t("home.account.title", locale)}
      </h2>
      <p className="truncate text-sm text-text">
        {data?.user?.email || data?.user?.name || NO_DATA}
      </p>
      <button
        type="button"
        onClick={signOut}
        className="w-fit border-b border-text pb-px text-2xs font-semibold uppercase tracking-widest text-text hover:border-text-muted hover:text-text-muted active:text-text-subtle"
      >
        {t("signedIn.signOut", locale)}
      </button>
    </section>
  );
}
