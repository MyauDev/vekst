import { t } from "./i18n";
import type { User } from "./gen/vekst/v1/identity_pb";

/**
 * What a signed-in person sees today: who they are, that they belong to no
 * organisation, and a way out.
 *
 * The "no organisation" line is not a placeholder -- it is the correct end
 * state of this change. Change 1.1 adds organisations and memberships; until it
 * lands, an authenticated person can reach no tenant data at all, and saying so
 * plainly is what stops that from looking broken.
 */
export function SignedIn({ user, onSignOut }: { user: User; onSignOut: () => void }) {
  return (
    <section className="flex flex-col gap-4">
      <div>
        <p className="text-sm text-text-muted">{t("signedIn.greeting")}</p>
        <p className="font-medium text-text">{user.email || user.name || user.id}</p>
      </div>

      <p className="text-sm text-text-muted">{t("signedIn.noOrganisation")}</p>

      <button
        type="button"
        onClick={onSignOut}
        className="inline-flex w-fit items-center rounded border border-border-strong px-4 py-2 text-sm font-medium text-text hover:bg-surface-sunken"
      >
        {t("signedIn.signOut")}
      </button>
    </section>
  );
}
