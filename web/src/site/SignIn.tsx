import { authErrorMessage, t } from "../i18n";

/**
 * The sign-in screen: one button, which leaves the single-page app entirely.
 *
 * A plain link rather than a fetch, because the OIDC flow is a browser
 * redirect: core answers /auth/google/start with a 302 to Google, and only a
 * top-level navigation can follow it and come back with cookies intact.
 *
 * Deliberately unstyled beyond what makes it usable. `add-web-app-shell` (5.1)
 * establishes the token layer, and this markup is expected to be thrown away.
 */
export function SignIn({ authError }: Readonly<{ authError?: string | null }>) {
  const message = authErrorMessage(authError ?? null);

  return (
    <section className="flex flex-col gap-4">
      <h2 className="text-lg font-medium text-text">{t("signIn.heading")}</h2>
      <p className="text-sm text-text-muted">{t("signIn.blurb")}</p>

      {message && (
        <p role="alert" className="text-sm text-danger">
          {message}
        </p>
      )}

      <a
        href="/auth/google/start"
        className="inline-flex w-fit items-center rounded border border-border-strong px-4 py-2 text-sm font-medium text-text hover:bg-surface-sunken"
      >
        {t("signIn.google")}
      </a>
    </section>
  );
}
