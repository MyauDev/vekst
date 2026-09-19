/**
 * Register A. The landing page and sign-in, and nothing that needs a session.
 *
 * **This file and everything under `site/` must not read application state** --
 * no data module, no transport, no session. `ARCHITECTURE.md` §8 schedules a
 * static marketing bundle at `/site` for Commercial, and a page that reaches
 * into the application cannot be lifted into one without a rewrite. The test in
 * `add-web-experience` §5.8 is what keeps that true.
 *
 * Register A scales: type up to 48px, sections at 64-96px. The 32px spacing cap
 * that `check-web-tokens.sh` enforces on the application does not apply here,
 * and this is the only rule it is exempt from.
 */
import type { ReactNode } from "react";

import { t } from "../i18n";
import { useLocale } from "../ui/preferences";

export function PublicLayout({ children }: Readonly<{ children: ReactNode }>) {
  const [locale] = useLocale();

  return (
    <div className="flex min-h-dvh flex-col">
      {/* Visible only on keyboard focus -- DESIGN.md §9's floor applies here
          too, and a page with one call to action still has a header to skip. */}
      <a
        href="#main-content"
        className="sr-only focus:not-sr-only focus:absolute focus:left-6 focus:top-6 focus:z-50 focus:border focus:border-border-strong focus:bg-surface-raised focus:px-4 focus:py-2 focus:text-sm focus:font-medium focus:text-text"
      >
        {t("a11y.skipToContent", locale)}
      </a>

      <header className="mx-auto flex w-full max-w-landing items-baseline justify-between px-6 py-6">
        <span className="text-md font-semibold tracking-tight">{t("app.title", locale)}</span>
      </header>

      <main id="main-content" className="mx-auto w-full max-w-landing grow px-6">{children}</main>

      <footer className="mx-auto w-full max-w-landing px-6 py-10">
        <p className="border-t border-border pt-4 text-2xs text-text-subtle">
          {t("app.tagline", locale)}
        </p>
      </footer>
    </div>
  );
}
