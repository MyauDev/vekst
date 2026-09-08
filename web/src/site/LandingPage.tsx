/**
 * The landing page. Register A.
 *
 * One page, five sections, **one call to action repeated once** — `DESIGN.md`
 * §1: one promise, one action. Register A scales apply here and only here: type
 * to 48px, sections at 64–96px, which is why this file lives in `site/` where
 * `check-web-tokens.sh` exempts the 32px cap.
 *
 * §5.4 shows the real `PnlTable` rather than a screenshot. It costs nothing
 * once the application exists, it cannot go stale, and it is the strongest
 * asset this product has. Its figures are inert here (`linked={false}`): a
 * marketing page that drops the reader into a sign-in wall mid-scroll has spent
 * their attention badly.
 */
import { t } from "../i18n";
import type { Locale, MessageKey } from "../i18n";
import { useLocale } from "../ui/preferences";
import { PnlTable } from "../app/PnlTable";
import { PublicLayout } from "./PublicLayout";
import { Reveal } from "./Reveal";
import { SAMPLE_REPORT } from "./sample";

function Cta({ locale, large }: Readonly<{ locale: Locale; large?: boolean }>) {
  return (
    <a
      href="/signin"
      className={`inline-block border-b-2 border-text pb-1 font-semibold uppercase tracking-widest text-text ${
        large ? "text-base" : "text-sm"
      }`}
    >
      {t("landing.cta", locale)}
    </a>
  );
}

/** A numbered step. The rule and the numeral carry the sequence; §14 keeps the
 *  icon out, because a number already says "first, second, third". */
function Step({
  n,
  titleKey,
  bodyKey,
  locale,
}: Readonly<{
  n: number;
  titleKey: MessageKey;
  bodyKey: MessageKey;
  locale: Locale;
}>) {
  return (
    <div className="flex flex-col gap-2 border-t border-border pt-4">
      <span className="tabular text-2xs font-semibold uppercase tracking-widest text-text-subtle">
        {String(n).padStart(2, "0")}
      </span>
      <h3 className="text-md font-semibold tracking-tight">{t(titleKey, locale)}</h3>
      <p className="text-sm leading-relaxed text-text-muted">{t(bodyKey, locale)}</p>
    </div>
  );
}

export function LandingPage() {
  const [locale] = useLocale();

  return (
    <PublicLayout>
      {/* 1 — the promise, and the one action */}
      <section className="py-20 sm:py-24">
        <Reveal>
          <h1 className="max-w-4xl text-3xl font-semibold leading-tight tracking-tight sm:text-4xl">
            {t("landing.promise", locale)}
          </h1>
        </Reveal>
        <Reveal delay={80}>
          <p className="mt-6 max-w-prose text-md leading-relaxed text-text-muted">
            {t("landing.sub", locale)}
          </p>
          <div className="mt-10">
            <Cta locale={locale} large />
          </div>
        </Reveal>
      </section>

      {/* 2 — the problem that earns the rest */}
      <section className="border-t border-border py-20">
        <Reveal>
          <div className="grid gap-6 sm:grid-cols-[16rem_1fr]">
            <h2 className="text-xl font-semibold leading-snug tracking-tight">
              {t("landing.problem.title", locale)}
            </h2>
            <p className="max-w-prose text-md leading-relaxed text-text-muted">
              {t("landing.problem.body", locale)}
            </p>
          </div>
        </Reveal>
      </section>

      {/* 3 — how it works, in the order the pipeline actually runs */}
      <section className="border-t border-border py-20">
        <h2 className="text-xl font-semibold tracking-tight">{t("landing.how.title", locale)}</h2>
        <div className="mt-10 grid gap-8 sm:grid-cols-3">
          {([1, 2, 3] as const).map((n, i) => (
            <Reveal key={n} delay={i * 70}>
              <Step
                n={n}
                titleKey={`landing.how.${n}.title` as MessageKey}
                bodyKey={`landing.how.${n}.body` as MessageKey}
                locale={locale}
              />
            </Reveal>
          ))}
        </div>
      </section>

      {/* 4 — the product itself. No motion here: it must read as the
          application does, and the application does not move. */}
      <section className="border-t border-border py-20">
        <h2 className="text-xl font-semibold tracking-tight">
          {t("landing.product.title", locale)}
        </h2>
        <p className="mt-3 max-w-prose text-sm leading-relaxed text-text-muted">
          {t("landing.product.body", locale)}
        </p>
        <div className="mt-8 border border-border bg-surface p-4">
          <PnlTable
            report={SAMPLE_REPORT}
            locale={locale}
            from={SAMPLE_REPORT.periods[0]!}
            to={SAMPLE_REPORT.periods.at(-1)!}
            linked={false}
          />
        </div>
      </section>

      {/* 5 — the same action, not a second one */}
      <section className="border-t border-border py-20">
        <Reveal>
          <h2 className="max-w-3xl text-2xl font-semibold tracking-tight">
            {t("landing.close.title", locale)}
          </h2>
          <div className="mt-8">
            <Cta locale={locale} large />
          </div>
        </Reveal>
      </section>
    </PublicLayout>
  );
}
