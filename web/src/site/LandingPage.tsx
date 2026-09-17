/**
 * The landing page. Register A — and, by explicit direction, a deliberate
 * departure from `docs/DESIGN.md` §14 for this file only. That section
 * rejects pills, cards-around-everything and decorative motion because the
 * product's reference is financial print; this page instead follows the
 * `gpt-taste` skill's playbook (cinematic hero, GSAP motion, a bento grid).
 * The override is scoped here on purpose: `SignIn.tsx` and `PublicLayout.tsx`
 * are untouched, and every figure this page shows is still the real
 * `PnlTable` on sample data, still `linked={false}` for the same reason as
 * before — a marketing page that drops the reader into a sign-in wall
 * mid-scroll has spent their attention badly.
 *
 * The token rule in `index.css` is NOT part of the override: every colour
 * here is still a semantic token or a `color-mix` of one (see index.css §6),
 * so `scripts/check-web-tokens.sh` stays clean and dark mode stays free.
 *
 * Motion is real GSAP (`gsap` + `@gsap/react`), not ScrollTrigger — this page
 * renders inside `router.test.tsx` under jsdom, which has neither
 * `matchMedia` nor the layout APIs ScrollTrigger needs, so scroll-linked
 * motion is driven by `IntersectionObserver` instead, the same proven,
 * jsdom-safe trigger `Reveal.tsx` already used. `prefersReducedMotionOrNoJS`
 * guards every animated entrance: when it is true, elements simply keep the
 * visible state they were server-rendered with, because nothing ever calls
 * `gsap.set` to hide them first.
 */
import { useRef } from "react";
import type { ReactNode } from "react";
import gsap from "gsap";
import { useGSAP } from "@gsap/react";

import { t } from "../i18n";
import type { Locale, MessageKey } from "../i18n";
import { useLocale } from "../ui/preferences";
import { PnlTable } from "../app/PnlTable";
import { SAMPLE_REPORT } from "./sample";

gsap.registerPlugin(useGSAP);

/** True when motion should not run at all: honours the reader's preference
 *  and, just as importantly, fails safe in any environment (jsdom, an old
 *  engine) that lacks `matchMedia`. */
function prefersReducedMotionOrNoJS(): boolean {
  try {
    return (
      typeof window === "undefined" ||
      typeof window.matchMedia !== "function" ||
      window.matchMedia("(prefers-reduced-motion: reduce)").matches
    );
  } catch {
    return true;
  }
}

/* ---------------------------------------------------------------------------
 * Nav — a floating split bar. Brand left, the one action right, repeated.
 * ------------------------------------------------------------------------ */

function Nav({ locale }: Readonly<{ locale: Locale }>) {
  return (
    <header className="landing-nav sticky top-4 z-40 mx-auto flex w-full max-w-landing items-center justify-between border border-border bg-surface-raised/80 px-5 py-3 backdrop-blur">
      <span className="text-md font-semibold tracking-tight">{t("app.title", locale)}</span>
      <nav className="flex items-center gap-6">
        <a href="#how" className="hidden text-sm text-text-muted hover:text-text sm:inline">
          {t("landing.nav.how", locale)}
        </a>
        <a href="#product" className="hidden text-sm text-text-muted hover:text-text sm:inline">
          {t("landing.nav.product", locale)}
        </a>
        <a
          href="/signin"
          className="inline-flex items-center bg-accent px-4 py-1.5 text-sm font-medium text-accent-text hover:bg-accent-hover"
        >
          {t("landing.cta", locale)}
        </a>
      </nav>
    </header>
  );
}

/* ---------------------------------------------------------------------------
 * Hero — Cinematic Center. One promise, two CTAs, a photograph behind it.
 * ------------------------------------------------------------------------ */

function Cta({
  variant,
  href,
  children,
}: Readonly<{
  variant: "primary" | "secondary";
  href: string;
  children: ReactNode;
}>) {
  const className =
    variant === "primary"
      ? "inline-flex items-center justify-center bg-accent px-6 py-3 text-sm font-semibold text-accent-text hover:bg-accent-hover"
      : "inline-flex items-center justify-center border border-border-strong px-6 py-3 text-sm font-semibold text-text hover:bg-surface-sunken";
  return (
    <a href={href} className={className}>
      {children}
    </a>
  );
}

function Hero({ locale }: Readonly<{ locale: Locale }>) {
  const scope = useRef<HTMLDivElement>(null);

  useGSAP(
    () => {
      if (prefersReducedMotionOrNoJS()) return;
      const targets = gsap.utils.toArray<HTMLElement>("[data-hero-in]", scope.current);
      gsap.set(targets, { opacity: 0, y: 16 });
      gsap.to(targets, {
        opacity: 1,
        y: 0,
        duration: 0.7,
        ease: "power3.out",
        stagger: 0.09,
        delay: 0.15,
      });
    },
    { scope },
  );

  return (
    <section
      ref={scope}
      className="landing-mesh landing-grain relative overflow-hidden border-b border-border"
    >
      {/* Full-bleed photograph, desaturated so it reads as texture rather
          than stock imagery, fading into the page surface at the foot. */}
      <div
        aria-hidden="true"
        className="absolute inset-0 bg-cover bg-center opacity-[0.14] grayscale contrast-125"
        style={{ backgroundImage: "url(https://picsum.photos/seed/vekst-ledger/1920/1080)" }}
      />
      <div className="pointer-events-none absolute inset-x-0 bottom-0 h-40 bg-gradient-to-b from-transparent to-surface" />

      <div className="relative mx-auto max-w-landing px-6 py-24 sm:py-32">
        <h1
          data-hero-in
          className="max-w-5xl text-3xl font-semibold leading-tight tracking-tight sm:text-4xl"
        >
          {t("landing.promise", locale)}
        </h1>
        <p data-hero-in className="mt-6 max-w-prose text-md leading-relaxed text-text-muted">
          {t("landing.sub", locale)}
        </p>
        <div data-hero-in className="mt-10 flex flex-wrap gap-3">
          <Cta variant="primary" href="/signin">
            {t("landing.cta", locale)}
          </Cta>
          <Cta variant="secondary" href="#product">
            {t("landing.cta.secondary", locale)}
          </Cta>
        </div>
      </div>
    </section>
  );
}

/* ---------------------------------------------------------------------------
 * Interest — a gapless three-column bento. Problem statement, step count,
 * three steps. col-span-2 + col-span-1 fills row one; three col-span-1 cards
 * fill row two. No cell is empty.
 * ------------------------------------------------------------------------ */

function BentoCard({
  className = "",
  children,
}: Readonly<{ className?: string; children: ReactNode }>) {
  return (
    <div className={`rounded-panel border border-border p-6 ${className}`}>{children}</div>
  );
}

function StepCard({
  n,
  titleKey,
  bodyKey,
  locale,
}: Readonly<{ n: number; titleKey: MessageKey; bodyKey: MessageKey; locale: Locale }>) {
  return (
    <BentoCard className="group overflow-hidden transition-colors duration-300 hover:border-border-strong">
      <span className="tabular text-2xs font-semibold uppercase tracking-widest text-text-subtle">
        {String(n).padStart(2, "0")}
      </span>
      <h3 className="mt-3 text-md font-semibold tracking-tight transition-transform duration-500 ease-out group-hover:translate-x-1">
        {t(titleKey, locale)}
      </h3>
      <p className="mt-2 text-sm leading-relaxed text-text-muted">{t(bodyKey, locale)}</p>
    </BentoCard>
  );
}

function Interest({ locale }: Readonly<{ locale: Locale }>) {
  return (
    <section id="how" className="mx-auto max-w-landing px-6 py-24 sm:py-32">
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-3 [grid-auto-flow:dense]">
        <BentoCard className="sm:col-span-2">
          <h2 className="text-xl font-semibold leading-snug tracking-tight">
            {t("landing.problem.title", locale)}
          </h2>
          <p className="mt-3 max-w-prose text-md leading-relaxed text-text-muted">
            {t("landing.problem.body", locale)}
          </p>
        </BentoCard>

        <BentoCard className="flex flex-col justify-center">
          <span className="tabular text-4xl font-semibold tracking-tight text-text">01–03</span>
          <p className="mt-2 text-sm leading-relaxed text-text-muted">
            {t("landing.how.title", locale)}
          </p>
        </BentoCard>

        {([1, 2, 3] as const).map((n) => (
          <StepCard
            key={n}
            n={n}
            titleKey={`landing.how.${n}.title` as MessageKey}
            bodyKey={`landing.how.${n}.body` as MessageKey}
            locale={locale}
          />
        ))}
      </div>
    </section>
  );
}

/* ---------------------------------------------------------------------------
 * Desire, part 1 — an infinite marquee built entirely from words the product
 * already uses (nav labels, the two accounting bases). No invented copy, no
 * fabricated brand logos: there is no customer list yet to show honestly.
 * ------------------------------------------------------------------------ */

function Marquee({ locale }: Readonly<{ locale: Locale }>) {
  const words = [
    t("nav.imports", locale),
    t("nav.review", locale),
    t("nav.reports", locale),
    t("report.basis.cash", locale),
    t("report.basis.accrual", locale),
  ];
  const track = [...words, ...words];

  return (
    <div className="overflow-hidden border-y border-border py-6" aria-hidden="true">
      <div className="landing-marquee-track flex w-max gap-12">
        {track.map((w, i) => (
          <span
            key={i}
            className="whitespace-nowrap text-xl font-medium tracking-tight text-text-subtle"
          >
            {w}
          </span>
        ))}
      </div>
    </div>
  );
}

/* ---------------------------------------------------------------------------
 * Desire, part 2 — the real report. Scale-and-fade in on scroll, driven by
 * IntersectionObserver rather than ScrollTrigger (see file header).
 * ------------------------------------------------------------------------ */

function ScaleReveal({ children }: Readonly<{ children: ReactNode }>) {
  const ref = useRef<HTMLDivElement>(null);

  useGSAP(() => {
    const node = ref.current;
    if (!node || prefersReducedMotionOrNoJS() || typeof IntersectionObserver === "undefined") {
      // No observer, or motion declined: stays exactly as rendered. Content
      // that depends on a trigger to become visible is content that can
      // vanish -- the same reasoning `Reveal.tsx` uses.
      return;
    }
    gsap.set(node, { opacity: 0, scale: 0.94 });
    const io = new IntersectionObserver(
      ([entry]) => {
        if (entry?.isIntersecting) {
          gsap.to(node, { opacity: 1, scale: 1, duration: 0.9, ease: "power2.out" });
          io.disconnect();
        }
      },
      { rootMargin: "-80px" },
    );
    io.observe(node);
    return () => io.disconnect();
  }, []);

  return <div ref={ref}>{children}</div>;
}

function Desire({ locale }: Readonly<{ locale: Locale }>) {
  return (
    <>
      <Marquee locale={locale} />
      <section id="product" className="mx-auto max-w-landing px-6 py-24 sm:py-32">
        <h2 className="text-xl font-semibold tracking-tight">{t("landing.product.title", locale)}</h2>
        <p className="mt-3 max-w-prose text-sm leading-relaxed text-text-muted">
          {t("landing.product.body", locale)}
        </p>
        <ScaleReveal>
          {/* `surface-raised`, matching what PnlTable's frozen column paints:
              a mismatch here shows up as a colour seam when it scrolls. */}
          <div className="mt-8 rounded-panel border border-border bg-surface-raised p-6">
            <PnlTable
              report={SAMPLE_REPORT}
              locale={locale}
              from={SAMPLE_REPORT.periods[0]!}
              to={SAMPLE_REPORT.periods.at(-1)!}
              linked={false}
            />
          </div>
        </ScaleReveal>
      </section>
    </>
  );
}

/* ---------------------------------------------------------------------------
 * Action — the same action, not a second one, at full scale, and a footer
 * with no links to pages that do not exist yet.
 * ------------------------------------------------------------------------ */

function Action({ locale }: Readonly<{ locale: Locale }>) {
  return (
    <section className="border-t border-border">
      <div className="mx-auto max-w-landing px-6 py-24 text-center sm:py-32">
        <h2 className="mx-auto max-w-3xl text-2xl font-semibold tracking-tight sm:text-3xl">
          {t("landing.close.title", locale)}
        </h2>
        <div className="mt-10 flex justify-center">
          <Cta variant="primary" href="/signin">
            {t("landing.cta", locale)}
          </Cta>
        </div>
      </div>
      <footer className="mx-auto w-full max-w-landing px-6 py-10">
        <p className="border-t border-border pt-4 text-2xs text-text-subtle">
          {t("landing.rights", locale)}
        </p>
      </footer>
    </section>
  );
}

export function LandingPage() {
  const [locale] = useLocale();

  return (
    <div className="flex min-h-dvh flex-col overflow-x-hidden">
      <a
        href="#main-content"
        className="sr-only focus:not-sr-only focus:absolute focus:left-6 focus:top-6 focus:z-50 focus:border focus:border-border-strong focus:bg-surface-raised focus:px-4 focus:py-2 focus:text-sm focus:font-medium focus:text-text"
      >
        {t("a11y.skipToContent", locale)}
      </a>

      <Nav locale={locale} />

      <main id="main-content" className="grow">
        <Hero locale={locale} />
        <Interest locale={locale} />
        <Desire locale={locale} />
        <Action locale={locale} />
      </main>
    </div>
  );
}
