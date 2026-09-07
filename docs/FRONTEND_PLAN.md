# Vekst — frontend plan, reviewed and extended

Date: 2026-09-07 · Reviewed against `web/` at `add-tenancy-and-rls`.
Companion documents: `DESIGN.md` (how it looks), `IMPLEMENTATION_PLAN.md` §3
(the Demo change list), `WORKFLOW.md` §5 (what the dashboard contains).

---

## 0. Verdict

The Demo budgets **4 person-days** of web work: 5.1 (2) and 5.2 (2).

Reviewed against what the three screens actually need, it is **9**. The gap is
not polish. It is five pieces of work that no change owns: the web half of
sign-in, the read model for asynchronous ingest, the money formatter, the state
components, and the empty, loading and error states of every screen.

At the 2.12× overrun measured on changes 0.1 and 0.2, 9 planned days is ≈ 19.
Section 6 says what to cut.

> **Correction, 2026-09-07.** Change 1.2 `add-identity` shipped complete
> (55 of 55 tasks) between this review and today, and it carried its web half
> with it: `src/SignIn.tsx`, `src/SignedIn.tsx`, sign-in gating in
> `src/router.tsx`, a key-based `src/i18n.ts` in `en` + `ru` with a
> code→message mapper, and `src/testTransport.ts`. Gaps 1, 8 and most of 4 are
> closed, and 5.1c is done. **The remaining web work is 7.5 days, not 9.** The
> budget it is measured against is still 4.
>
> Note what that means for the order: 1.2 was on this plan's own cut list, and
> 1.1 `add-tenancy-and-rls` — which blocks every tenant table after it — is
> still at 0 of 30.

---

## 1. What exists today

| Present | Note |
| --- | --- |
| Vite 7, React 19, TypeScript 5.9 | `web/package.json` |
| Tailwind v4.1.13, CSS-first | `index.css` holds one `@import`; the token layer is 5.1a |
| TanStack Router 1.132 | One route. `rootRoute` + `indexRoute` |
| TanStack Query 5.90 | Present, unused beyond the health card |
| Connect + connect-web 2.1 | `transport.ts`, no interceptors |
| Vitest + Testing Library | 4 test files, no shared render helper |
| `web/src/gen` | `health_pb.ts` and `money_pb.ts` only |

`router.tsx` already carries the comment "Change 5.2 adds the session here."
Nothing adds it — see gap 1.

---

## 2. The review — eleven gaps

| # | Gap | Why it matters |
| --- | --- | --- |
| 1 | ~~The web half of identity has no change.~~ **Closed 2026-09-07** | 1.2 shipped `SignIn.tsx`, `SignedIn.tsx` and sign-in gating in `router.tsx`. The gating sits in the index route rather than a guard, deliberately, while one route exists |
| 2 | **No read model for asynchronous ingest.** Upload → parse → validate → persist runs as a River job | The browser has no defined way to learn that a batch finished. This is an architecture decision, not a screen detail. See §5 |
| 3 | **No money formatter.** `money.test.ts` exists; `money.ts` does not | Every figure on every screen goes through it, in two locales, from `int64` minor units. It is shared code, not screen code |
| 4 | **Mostly closed 2026-09-07.** `src/i18n.ts` is a typed key catalog in `en` + `ru` with `authErrorMessage` mapping backend codes | It calls itself a placeholder for 5.1. What remains: decide whether to keep it or adopt a library, and the ingest error catalog, which is still blocked by **2.3** |
| 5 | **ECharts is named in `ARCHITECTURE.md`, absent from `package.json`** | And the Demo does not need it: the deliverable is the P&L table plus drill-down. Charts are Commercial (`WORKFLOW.md` §0) |
| 6 | **Organisation, entity and period are not URL state** | If they live in React state, a drill-down link is not shareable and the back button does not work. They belong in TanStack Router search params |
| 7 | **The review queue has no windowing** | 12 months of first-time bank data is thousands of rows. `@tanstack/react-virtual` is not installed |
| 8 | ~~No test harness for a Connect transport.~~ **Closed 2026-09-07** | `src/testTransport.ts` stubs Health and Identity over `createRouterTransport`, and reports the unauthenticated state as core does rather than as an empty response |
| 9 | **Empty, loading and error states are unbudgeted** | Three per screen, nine in total. They are most of what a reviewer sees on a Demo day, because the happy path is fast |
| 10 | **No lint rule against literal colour classes** | `DESIGN.md` §3 prices dark mode at 0.5 days. That price holds only while no component writes `text-slate-900` |
| 11 | **The landing page has no change and no owner** | `/site` is in the monorepo layout in `openspec/config.yaml`. The directory does not exist |

---

## 3. The extended change list

| # | Change | Days | Blocked by |
| --- | --- | --- | --- |
| 5.1a | `add-web-app-shell` — token layer, shell, nav, org/entity/period as search params, the state components from `DESIGN.md` §7, the money formatter, the literal-colour lint rule | 1.5 | nothing |
| 5.1b | `add-i18n-and-error-catalog` — the i18n mechanism and the `ru` plumbing; then the code→sentence catalog | 1.5 | mechanism: nothing · catalog: **2.3** |
| ~~5.1c~~ | ~~`add-web-session`~~ — **delivered inside 1.2 on 2026-09-07**. What is left is folding it into the shell when 5.1a lands | 0 | — |
| 5.2a | `add-imports-screen` — batch list, upload with the `source_kind` choice, the validation report, the error list by original line number | 1.5 | **2.1, 2.3** |
| 5.2b | `add-review-screen` — counterparty groups by amount, keyboard handling, windowing | 1.5 | **3.3** |
| 5.2c | `add-pnl-and-drilldown` — the report table, basis label, reconciliation strip, the drill-down panel | 2 | **4.1, 4.2** |

**7.5 person-days** after the 2026-09-07 correction above (9 as first reviewed). The plan budgets 4.

Product, unchanged in scope but now itemised: `add-dark-mode` (0.5),
`add-charts-echarts` (1), `add-column-mapping-ui` (3), `add-landing-page`
(unowned).

---

## 4. What can be built while the two inputs are late

D-1 (the category list) and D-2 (the seven export files) block Track A at 2.2
and Track B at 3.1. They do **not** block:

1. **5.1a** — every task, today.
2. **5.1b, the mechanism only** — the library, the locale switch, the `ru`
   plumbing, the number and date formats. Stop before the catalog.
3. **The test harness** (gap 8) — a fake Connect transport needs a service
   definition, not a server.

That is about 3 days of real, non-blocked frontend work. It is the honest
answer to "what do I do while I wait", and it is also the whole of what the
mock screens in the design canvas can be built into without a backend.

---

## 5. Decisions this plan takes

| Decision | Instead of | Reason |
| --- | --- | --- |
| **Poll the batch with TanStack Query `refetchInterval`** | Connect server-streaming | A poll survives every proxy and needs no reconnect logic. Ingest finishes in seconds to minutes, not milliseconds. Revisit if a customer file makes a poll feel slow |
| **Organisation, entity and period in router search params** | React state or a context | Shareable drill-down links, a working back button, and one source of truth for every query key |
| **A fake Connect transport in tests** | MSW | Connect ships `createRouterTransport`. It types against the generated service, so a proto change breaks the test rather than the demo |
| **Defer ECharts to Product** | Installing it in 5.1 | The Demo deliverable is a table and a drill-down. ECharts is roughly 1 MB and buys nothing before Commercial |
| **Copy primitives from unlumen, do not adopt its scales** | Installing a component library wholesale | Its defaults are Register A. Behind sign-in they cost rows per screen — `DESIGN.md` §1.1 |
| **The money formatter is shared code with its own tests** | Formatting inside components | It is the one place a wrong number can be introduced after the backend got it right |

---

## 6. If 1 October has to hold

The Demo definition of done is: each customer uploads their own files, sees a
validation result they trust, and reads their own Management P&L with every
figure openable. The frontend cuts that keep that sentence true:

| Cut | Saves | Cost |
| --- | --- | --- |
| 5.2b `add-review-screen`, if 3.3 is also cut | 1.5 | You resolve low-confidence rows in SQL before the Demo |
| 5.1c `add-web-session`, if 1.2 is also cut | 1 | No sign-in. You open the browser yourself |
| ECharts, dark mode, the landing page | — | Already out of the Demo. Keep them out |

That leaves **6.5 days** of web work against a budgeted 4. Nothing in the
definition of done is lost, because neither the review queue nor sign-in appears
in it.

What must not be cut from the screens: the basis label, the reconciliation
strip, the blocked-line reason, the engine layer and confidence in the
drill-down, and the original file line number in the error list. Each one is
what makes the number believable, and `DESIGN.md` §2 explains why a minimal
pass deletes them first.

---

## 7. Open questions

1. **Does the Demo need sign-in at all?** The answer decides 5.1c and 1.2
   together. Nobody has asked it.
2. **Which i18n library.** The choice is cheap; the timing is not — it lands
   before any screen writes a string.
3. **Where the landing page lives** — Product work or marketing work.
4. **The Russian state words** in `DESIGN.md` §7 need a native check before
   5.1b freezes the catalog.
5. **Does a viewer see the review queue read-only in the Demo?**
   `WORKFLOW.md` says yes; roles and permissions are Product. Cheapest answer:
   hide the queue for non-approvers rather than build a read-only mode.
