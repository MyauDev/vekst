**Ordering.** §0 is done. Do §1 next and do not skip it: `make lint` is red until it lands,
and every section after it adds components that the check would otherwise let through dirty.
Then §2 (shell), §3 (data layer), §4 (P&L). §5–§7 have no forward dependency on each
other. §8 (charts) comes last: it reads the fixtures §4 already renders.

**Why the P&L before the landing.** The monochrome palette is easiest to make look good on a
marketing page and hardest on a dense twelve-column table. Building the table first means a
failure of the visual system surfaces while it is still cheap to change. A landing page that
looks right proves almost nothing about Register B.

**Blocked work.** §3's fixtures stand in for changes 2.1, 2.3, 3.3, 4.1 and 4.2. None of those
blocks this change; they block replacing the fixtures, which is not in scope. D-1 and D-2
being late does not block any task here.

**Both reviewers required** on §0.1 (`/deploy/k8s/base`) and §9.1 (`/proto`).

**Spec ordering — clear.** This change modifies one requirement in `platform-foundation` and
one in `identity-access`. Both requirements exist in `openspec/specs/` as of 2026-09-08:
`add-postgres-and-migrations`, `add-identity` and `add-tenancy-and-rls` are all archived, and
nothing is stacked ahead of this change. Both deltas resolve against what is there today.

That archival also unblocks §9.1: the `User` extension needs `organizations` and `memberships`,
and `tenancy` is now a synced capability rather than a pending change.

**Rule for this change.** No component is finished while `make lint` fails on it. The token
check is not a formatting preference — it is what holds the price of the second palette.

**Second rule, added 2026-09-08.** Every component is checked against `docs/DESIGN.md` §14
before it is called done. §14 exists because the first components built here reached for the
default shapes — a tinted pill behind a status word, a card around every group, an even grid
of rounded swatches — and read as generated rather than designed. The reference is financial
print, not a dashboard: hairline rules, a label column with the material hanging off it,
hard-edged swatches, and type carrying the hierarchy.

## 0. Routing and the token layer — done

- [x] 0.1 Route `/auth` to `core` in `deploy/k8s/base/ingress.yaml`, above the `/` rule.
      `core/internal/identity/identity.go:169-171` registers the three routes; the Ingress sent
      them to the static web container, so sign-in was unreachable in the cluster. **Both
      reviewers**
- [x] 0.2 Add `/auth` to `server.proxy` in `web/vite.config.ts`, so `vite dev` and the cluster
      agree on which prefixes belong to `core`. It was broken in dev for the same reason
- [x] 0.3 Write the token layer in `web/src/index.css`: two palettes, semantic names mapped
      through `@theme inline`, paired line heights and letter spacing, the 4px spacing base,
      radius, motion, breakpoints, the focus ring, tabular figures, `prefers-reduced-motion`
      and a print block that forces the light ground
- [x] 0.4 Measure the contrast of both palettes by converting oklch through OKLab to linear
      sRGB to relative luminance. Every text pair clears AA. Deepen the light state colours,
      which passed at 4.69/4.59/5.27 with no headroom
- [x] 0.5 Write `scripts/check-web-tokens.sh` and call it from `make lint`
- [x] 0.6 Amend `docs/DESIGN.md` §3, §4, §6, §9, §11, §12 and `docs/WORKFLOW.md` §0

## 1. The migration, and the check goes green — A

- [x] 1.1 Port `src/SignIn.tsx`, `src/SignedIn.tsx`, `src/HealthCard.tsx` and `src/router.tsx`
      onto semantic tokens. 18 lines, 22 classes. `./scripts/check-web-tokens.sh` names each
- [x] 1.2 Delete `App()` from `src/App.tsx`. It duplicates the shell markup in `router.tsx`'s
      root route, is unreachable from `main.tsx`, and survives only because `App.test.tsx`
      mounts it. Keep `Session`, which `router.tsx` imports
- [x] 1.3 Repoint `src/App.test.tsx` at `Session` rather than `App`. The five assertions are
      about session states and none of them is about the shell
- [x] 1.4 `make lint` green, `npx vitest run` green. This is the gate for §2
- [x] 1.5 Establish the source layout the rest of this change and `check-web-tokens.sh` both
      assume: `web/src/site/` is Register A, `web/src/app/` is Register B, `web/src/ui/` holds
      what both use, `web/src/data/` holds the data modules. The lint script already exempts
      `web/src/site/` from the spacing cap and task 5.9 tests `web/src/app/` — until the
      directories exist, the exemption is unreachable and the test has nothing to look at

## 2. The shell and the two registers — A

- [x] 2.1 Add a theme controller: `[data-theme]` on the document element, persisted, defaulting
      to the system preference. Three states — absent, `light`, `dark` — not two
- [x] 2.2 Add a locale controller on the same mechanism. `resolveLocale` already falls back to
      `navigator.language`; this makes the choice explicit and durable. Neither value goes in
      the URL (design D7)
- [x] 2.3 `PublicLayout` in `web/src/site/` for `/` and `/signin` — Register A scales, no
      rail, no top bar
- [x] 2.4 `AppLayout` in `web/src/app/` for `/app/*` — 208px rail, 48px top bar, outlet.
      Register B scales
- [x] 2.5 `Rail`: Imports · Review · Reports in pipeline order, 32px rows, `aria-current` on
      the active item, a count on Review and on nothing else
- [x] 2.6 `TopBar`: organisation, entity, period, language, theme. Organisation and entity
      render as labels until §8.1 extends `User`; the slot ships regardless — `DESIGN.md` §8
- [x] 2.7 Route tree per `INFORMATION_ARCHITECTURE.md` §1, with `/app` redirecting to
      `/app/reports/pnl` and `/app/reports` to the same. Do not define any route under `/rpc`
      or `/auth`
- [x] 2.8 Move sign-in gating from the index route to a guard on `/app/*`. The comment in
      `router.tsx` says the gating sits in the index route because one route exists; that
      stops being true here
- [x] 2.9 `StateChip` — nine states, three axes, one component, each with its word in `en` and
      `ru`. Never colour alone
- [x] 2.10 `EmptyState`, `Skeleton`, `ErrorState`. Written as what a customer sees, with no
      control offered for an unwired action
- [x] 2.11 `Money` — formats from `int64` minor-unit strings, sums in `BigInt`, tabular, right
      aligned, minus sign not parentheses, `0.00` distinct from `—`. `src/mock/money.ts` is the
      sketch of this and `src/money.test.ts` already exists and tests nothing
- [x] 2.12 Test: an amount above `Number.MAX_SAFE_INTEGER` renders exactly
- [x] 2.13 Test: a currency whose exponent is not 2 renders in its own denomination
- [x] 2.14 Dev-only `/tokens` route, outside `/app` and off the rail: both palettes, the type
      ramp, the spacing steps and all nine state chips, rendered from the real tokens
- [x] 2.15 Test: every interactive element in the shell shows a focus indicator
- [x] 2.16 Test: an unauthenticated visit to any `/app` route lands on sign-in. Assert it
      against the route tree rather than one route, so a screen added later is covered by
      having been added
- [x] 2.17 Test: no client route is defined under a prefix the Ingress routes to `core`. Read
      the prefixes from `deploy/k8s/base/ingress.yaml` rather than restating them, or the test
      and the manifest drift apart exactly the way §0 found them drifted

## 3. The data layer — A

- [x] 3.1 `src/data/` — one module per screen. Interfaces shaped like the proto messages that
      will carry them: money as `int64` minor-unit strings, states as codes, never a
      JavaScript `number` for an amount (design D1)
- [x] 3.2 Fixtures behind each module, internally consistent: every total, subtotal and
      percentage computed from the rows the table shows, never typed in. `src/mock/data.ts` is
      the precedent and it is worth reading before writing this
- [x] 3.3 Test: no screen component imports a transport or a generated client directly
- [x] 3.4 Test: fixture totals equal the sum of their rows

## 4. The report and the drill-down — A + B

- [x] 4.1 `PnlTable` — frozen category column, frozen header, one column per period, then Total
      and percent of revenue. Separate from `DataTable` by design D11
- [x] 4.2 The basis label, with the table rather than in a footnote, derived from `source_kind`
- [x] 4.3 The reconciliation strip: opening, in, out, transfers, closing
- [x] 4.4 A blocked line renders as blocked and names its reason in the same view. Never a
      computed figure
- [x] 4.5 Period range from `?from=`/`?to=` as `YYYY-MM`, defaulting to the current year to date
- [x] 4.6 `/app/reports/pnl/cell/$category/$period` — the drill-down as a nested route rendering
      a panel over the mounted report (design D8). 120ms, and the only shadow in Register B
- [x] 4.7 The panel names which figure it is. A shared link arrives with no memory of the click
- [x] 4.8 Transactions with category, engine layer, confidence and match evidence; and the
      `taxonomy_version` + `ruleset_version` + `engine_version` triple
- [x] 4.9 ~~Windowed~~ **scrolling** transaction list. One category in one month is tens of
      rows — below where windowing pays for itself, and windowing costs a measured viewport,
      so it renders nothing until layout arrives. See design D14
- [x] 4.10 Test: the back button closes the panel and leaves the report mounted
- [x] 4.11 Test: a link with no `from`/`to` renders the year-to-date default
- [x] 4.12 Test: a report renders correctly in both palettes and prints on a light ground
- [x] 4.13 The headline row: four stat tiles above the table — revenue, expenses, net,
      unreviewed amount. `WORKFLOW.md` §5.3 gives their colour job as **none**, which makes
      them the one element of that section needing no palette and no library
- [x] 4.14 Compute the tiles from the same fixture rows the table renders, so a tile cannot
      disagree with the column it sits above
- [x] 4.15 Stat-tile values use proportional figures, not tabular. Tabular is for columns that
      must align vertically; a single large number has no column to align with

## 5. The landing — A

- [x] 5.1 `/` — hero with one promise and one call to action, to `/signin`
- [x] 5.2 The problem: why a bank export and a ledger do not add up to a management report
- [x] 5.3 How it works, three steps, matching the fixed ingest pipeline
- [x] 5.4 The product itself — the real `PnlTable`, rendered from a sample that lives in
      `site/` so the directory stays self-contained, with its figures inert rather than
      linking into the application. Not a screenshot. It is the
      strongest asset the product has and it cannot go stale
- [x] 5.5 Closing call to action. The same one. Not a second
- [x] 5.6 Unlumen motion in §5.1, §5.2 and §5.5 only. Never in §5.4, which must read as the
      application does
- [x] 5.7 Fully responsive. It is the one page a stranger opens on a phone
- [x] 5.8 Test: `site/` makes no **runtime** import from `src/data/`, and none at all of the
      session or a transport. Type-only imports are erased at build and carry nothing, which
      is what lets §5.4 render the real table from a sample that lives in `site/` itself.
      As first written this task said "imports nothing from `src/data/`", which contradicted
      §5.4 outright — the landing cannot show the real component and import none of its
      shapes. The rule that matters is that `site/` reads no application state, so the
      Commercial extraction to `/site` stays a move rather than a rewrite
- [x] 5.9 Test: no Register A type size appears under `src/app/`

## 6. Imports — A

- [x] 6.1 `/app/imports` — batch list with state, `source_kind`, period, row count, time
- [x] 6.2 Upload asking for `ledger` or `bank` before the file is read. The tag decides the
      accounting basis and is not optional
- [x] 6.3 The counts: rows imported, duplicates skipped, internal transfers found, matches
      proposed. `DESIGN.md` §2 — these may never be removed
- [x] 6.4 `/app/imports/$batchId` — the outcome and its reason at the top
- [x] 6.5 The validation error list, keyed by **original file line number**, never the parsed
      row index. This is an invariant in `CLAUDE.md`, not a preference. Downloadable, and
      **capped rather than windowed** — nobody fixes five thousand errors by scrolling a box,
      and the download is the real answer at that size. See design D14
- [x] 6.6 Test: the error list renders original line numbers for a fixture whose parsed indices
      differ from them

## 7. Review — B

- [x] 7.1 `/app/review` — one counterparty group at a time, by amount then repeat count
- [x] 7.2 The keyboard legend, always visible, never behind a help control. Unlumen `Kbd`
- [x] 7.3 Digits pick a category, `Enter` approves the group, `T` internal transfer, `N` not in
      the P&L, arrows move, `Esc` clears
- [x] 7.4 36px rows — larger than the tables, because this screen takes keyboard focus
- [x] 7.5 Windowed. Twelve months of first-time bank data is thousands of rows and
      `@tanstack/react-virtual` is not installed (`FRONTEND_PLAN.md` gap 7)
- [x] 7.6 The rail's Review count follows the queue
- [x] 7.7 Test: the queue is fully operable from the keyboard with no pointer events

## 8. Charts — A + B

Charts were a Commercial item in `WORKFLOW.md` §0 and were brought into the Demo on
2026-09-08. They come after §4–§7 rather than inside §4 because they read the fixtures the
report already renders: the table has to be right before a picture of it is worth drawing.

The palette is decided and validated — `docs/DESIGN.md` §13, tokens in `web/src/index.css`.
Do not pick a colour here. Every rule below exists because the validator or `WORKFLOW.md`
§5.3 says so.

- [ ] 8.1 Add ECharts to `web/package.json`. It is named in `openspec/config.yaml:67` and has
      never been installed. Roughly 1 MB — import per chart, never the barrel
- [ ] 8.2 A chart shell: title, legend, table-view toggle, tooltip. Every chart is built from
      it, so the rules below are satisfied once rather than six times
- [ ] 8.3 **Every chart has a table view.** `WORKFLOW.md` §5.3 requires it. It is no longer
      also load-bearing for legibility — the palette was re-stepped so every slot clears 3:1
      on its own (§13.2) — so this is a reading affordance, not a patch over a weak colour
- [ ] 8.4 A legend whenever there are two or more series; four or fewer are also directly
      labelled. Identity is never carried by colour alone
- [ ] 8.5 Series colours are assigned in fixed slot order and never cycled. A ninth category
      folds into "Other" (`--color-series-other`), never a generated hue
- [ ] 8.6 Text — values, labels, axis ticks, legend text — wears text tokens, never a series
      colour. A coloured mark beside the label carries the identity
- [ ] 8.7 No chart carries two y-axes. `WORKFLOW.md` §5.3 and the visualization method agree,
      and it is the single most common charting error
- [ ] 8.8 **Money flow — Sankey.** Revenue sources → total in → expense categories. Top 8 plus
      Other. This is the picture customers screenshot
- [ ] 8.9 **Top expense categories — horizontal bar, sorted.** One hue, slot 1, every bar the
      same step. `WORKFLOW.md` §5.3 calls this "sequential, one hue"; the sharper rule is that
      bar length already encodes magnitude, so colouring bars by their value spends the
      identity channel re-encoding what length already shows
- [ ] 8.10 **Net result by month — diverging column, centred on zero.** Blue positive, orange
      negative, neutral at zero. Never red: `DESIGN.md` §6 forbids red for a negative figure,
      and every loss month would be one
- [ ] 8.11 **Revenue against expenses — two lines, one axis**, legend plus direct labels
- [ ] 8.12 **Category trend — small multiples**, one line each, one hue plus
      `--color-chart-deemph` for context. Small multiples are an all-pairs form, which caps
      distinguishable series at three; one hue per facet sidesteps the cap entirely
- [ ] 8.13 2px lines, ≥8px markers, 4px rounded data-ends anchored to the baseline, a 2px
      surface gap between adjacent fills, recessive grid and axes
- [ ] 8.14 Crosshair and tooltip on the line charts; per-mark tooltip on bar and Sankey. Hit
      targets larger than the marks
- [ ] 8.15 Charts honour `prefers-reduced-motion` and never animate a figure into place. A
      chart that draws itself in is fine; a number that counts up is not
- [ ] 8.16 Test: both palettes render every chart, and the dark steps are the documented dark
      values rather than a filter or an inversion
- [ ] 8.17 Test: a chart with nine or more categories folds the tail into "Other"
- [ ] 8.18 Test: no component references a raw series hex — `check-web-tokens.sh` fails on one
      already, so this asserts the charts went through the tokens

## 9. Contract, i18n and close — B for 9.1, else A

- [ ] 9.1 Extend `User` in `proto/vekst/v1/identity.proto` with the organisation identifier and
      name and the entity identifier; `make gen`; populate it **from the session** in the
      handler, never from a request field. **Both reviewers.** Apply after
      `add-tenancy-and-rls` (design D10).
      This is why the change carries an `identity-access` delta: the live requirement
      "Signing in grants no access to data" says the response carries *no* organisation, and
      that was correct until tenancy existed. No role field — roles are Product, and a field
      the front end can read is a field it starts trusting
- [ ] 9.2 Read organisation and entity from `GetCurrentUser` in `TopBar`, replacing the labels
      from §2.6
- [ ] 9.3 Decide whether `src/i18n.ts` stays or a library replaces it, then move every string
      this change added into whichever it is. The ingest error catalog stays out — blocked by
      2.3
- [ ] 9.4 Test: every message key present in one language is present in the other. The
      catalogue is typed against `en`, so a missing `ru` key is a runtime hole rather than a
      compile error
- [ ] 9.5 Have the `ru` state words in `DESIGN.md` §7 checked by a native speaker. They are a
      draft by a non-native writer and this change is the first to render them
- [ ] 9.6 Add the Inter woff2 subsets under `web/public/fonts/` and uncomment `index.css` §5
- [ ] 9.7 Delete `web/src/mock/` and `web/mock.html`. Nothing else references either
- [ ] 9.8 Add `/web` ownership lines to `.github/CODEOWNERS`
- [ ] 9.9 Test: no user-facing string spells the brand `Vekst`. The two spellings are
      deliberate — `Veekst` on screen, `vekst` in every identifier — and the only thing that
      keeps them from drifting back together is a check that knows which is which
- [ ] 9.10 Update `docs/FRONTEND_PLAN.md`: gaps 1–11 closed or reassigned, and §3's change list
      replaced by what was actually built
- [ ] 9.11 `make ci` green
