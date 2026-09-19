/**
 * The router's fallback when nothing matches.
 *
 * Global rather than per-register: an unmatched path can land here before the
 * session is known, so this renders without `Rail`, `TopBar` or
 * `PublicLayout` -- any of those would assume an answer this screen does not
 * have yet. Shape follows `docs/DESIGN.md` §14, the same as `ui/feedback.tsx`:
 * a rule, a line of type, one action -- no card, no illustration.
 */
import { t } from "../i18n";
import { useLocale } from "../ui/preferences";

export function NotFound() {
  const [locale] = useLocale();

  return (
    <div className="mx-auto flex min-h-dvh max-w-lg flex-col justify-center px-6">
      <div className="border-t border-border pt-4">
        <p className="text-base font-medium text-text">{t("notFound.title", locale)}</p>
        <p className="mt-1 text-sm text-text-muted">{t("notFound.detail", locale)}</p>
        <a
          href="/"
          className="mt-4 inline-block border-b border-text pb-0.5 text-2xs font-semibold uppercase tracking-widest text-text"
        >
          {t("notFound.cta", locale)}
        </a>
      </div>
    </div>
  );
}
