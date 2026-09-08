# Information Architecture — Vekst web

Date: 2026-09-07 · Phase 3 of the design flow.
Companion documents: `DECISIONS.md` (this folder), `docs/DESIGN.md` §8,
`docs/FRONTEND_PLAN.md` §5, `docs/WORKFLOW.md` §5, `openspec/config.yaml`.

> Two spaces in one Vite bundle: a public marketing page and an authenticated
> dashboard, sharing one `@theme` token layer. `DECISIONS.md` §9 records why
> they are one build rather than two, and what would have to stay true for the
> `/site` extraction scheduled at Commercial to remain cheap.

---

## 0. A blocking defect this document uncovered

`deploy/k8s/base/ingress.yaml` routes **only** `/rpc` to `core`. Every other
path, including `/auth`, goes to the `web` container.

`CLAUDE.md` names `/auth/google/start`, `/auth/google/callback` and
`/auth/logout` as the one non-Connect browser surface. `src/SignIn.tsx:26`
links to the first and `src/App.tsx:22` posts to the third. Both currently
reach the static web server, which does not serve them.

`web/vite.config.ts` proxies only `/rpc`, so the same is true under
`vite dev`. **Sign-in does not work in either environment.** Change 1.2
`add-identity` is recorded as complete at 55 of 55 tasks.

Two one-line fixes, in the two places routing is declared:

| File | Add |
| --- | --- |
| `deploy/k8s/base/ingress.yaml` | a `/auth` Prefix path to the `core` service, above the `/` rule |
| `web/vite.config.ts` | `/auth` alongside `/rpc` in `server.proxy` |

`deploy/k8s/base/` needs both reviewers per `.github/CODEOWNERS`.

This is an IA constraint as well as a bug: see §7, reserved prefixes.

---

## 1. Site map

```
PUBLIC — Register A, no session required
  Landing                       /
  Sign in                       /signin

RESERVED — served by core. The router must never claim these.
  Connect RPC                   /rpc/*
  OIDC start                    /auth/google/start
  OIDC callback                 /auth/google/callback
  Sign out                      /auth/logout

APP — Register B, session required
  (entry)                       /app            → /app/reports/pnl
  Imports                       /app/imports
    Batch detail                /app/imports/$batchId
  Review                        /app/review
  Reports                       /app/reports    → /app/reports/pnl
    Management P&L              /app/reports/pnl
      Drill-down                /app/reports/pnl/cell/$category/$period

DEVELOPMENT ONLY — not on any rail, absent from production builds
  Token sheet                   /tokens
```

Four notes on the shape.

**`/app` redirects rather than renders.** `openspec/config.yaml` states "THE MVP
IS THE MANAGEMENT P&L AND NOTHING ELSE", and the Demo's definition of done is
reading it. A separate overview would be a page every user passes through on the
way to the only thing they came for, and `DESIGN.md` §8's rail has no slot for
it. Reconsider at Commercial, when `WORKFLOW.md` §5.3's charts and the three
further reports give an overview something to hold.

**`/app/reports` redirects too, and keeps the segment.** One report exists.
Sales, OPEX and Cash Flow are Commercial (`WORKFLOW.md` §0), and they arrive as
siblings of `pnl` without moving it. The cost of the extra segment now is one
redirect; the cost of adding it later is every saved link.

**The drill-down is a route, not a state flag.** It renders as a panel over the
report, which stays mounted. This is the one place `DESIGN.md` §8 (a panel) and
`FRONTEND_PLAN.md` §5 (shareable links, working back button) could have
conflicted, and a nested route satisfies both. The panel closes by navigating to
the parent route, so the back button closes the panel rather than leaving the
report.

**`/tokens` sits outside `/app`.** It documents both registers, so it does not
belong inside one of them, and it must not inherit the app shell's chrome — the
point is to see the tokens, not the chrome. `DECISIONS.md` §2.6.

## 2. Navigation model

### Primary — the left rail, `/app` only

Three items, in pipeline order: **Imports · Review · Reports**. Fixed at three
for the Demo. `DESIGN.md` §8.

The order is the order of `WORKFLOW.md`'s pipeline — data in, data corrected,
data read — so the rail teaches the product's shape by being read top to bottom.
Reports is the destination and the default, which is why it is last rather than
first: a rail is a map, not a ranking.

**Review carries a count.** The number of items awaiting review, as a chip on
the rail item, from the mock. It is the only badge in the interface. It is there
because the review queue is the one screen with work that expires: an unreviewed
row is a wrong number in a report, and `WORKFLOW.md` §5.3 lists "unreviewed
amount" among the headline figures for the same reason.

208 px wide, 32 px rows, a 48 px header holding the wordmark, the account at the
foot. Measurements from `src/mock/ui.tsx`, which is a visual specification even
though it is not portable code (`DECISIONS.md` §1).

### Contextual — the top bar, `/app` only

48 px. Left to right: **organisation · entity · period · language · theme**.

| Control | Source | Notes |
| --- | --- | --- |
| Organisation | `GetCurrentUser`, once `User` is extended (`DECISIONS.md` §2.3) | One value in v1. Renders as a label, not a picker |
| Entity | Same | One entity per organisation in v1. The slot ships anyway — `DESIGN.md` §8: "the slot is cheap now and a retro-fit is not" |
| Period | URL search params | §7 |
| Language | Stored preference | `DECISIONS.md` §2.4. Not a URL param |
| Theme | Stored preference | Same mechanism as language |

Organisation and entity are **not** in the URL. `FRONTEND_PLAN.md` §5 puts them
there for shareable drill-down links, and that reasoning holds the day a user
belongs to two organisations. It does not hold in v1, where a user belongs to
one and the value is therefore not a choice. Reading them from the session now
and promoting them to search params when membership becomes plural is a smaller
change than shipping a param that can only ever hold one value — and a URL that
carries an organisation id the session already fixes invites the mistake of
trusting it. **Tenant context comes from the session, never from the URL.**

Period is in the URL because it is genuinely a choice, and it is the thing a
shared link is about.

### Utility

At the foot of the rail: the signed-in account, and sign out. Sign out is a form
POST to `/auth/logout`, not a link and not an RPC — it clears a cookie in the
same response that ends the session.

### Public navigation

The landing has no navigation bar in the ordinary sense: a wordmark, and one
call to action that goes to `/signin`. `DESIGN.md` §1 — "one promise, one call
to action". Section links within the page are allowed; routes are not.

### Mobile

**Not a Demo target.** `ARCHITECTURE.md` puts mobile at the Intelligence
milestone. What must still be true:

- The landing is fully responsive. It is the one page a stranger opens on a
  phone.
- `/app` degrades rather than breaks: the rail collapses to a menu below
  1024 px, the top bar wraps, and the P&L scrolls horizontally as it already
  does on a desktop — `DESIGN.md` §5 calls horizontal scroll "expected, not a
  bug", which makes the table the one part of the app that already behaves
  correctly on a narrow screen.
- No interaction is hover-only, so nothing is unreachable by touch.
  `DESIGN.md` §9 already requires this for accessibility; it pays for mobile
  twice.

## 3. Content hierarchy

### Landing `/`

Five sections, one page, one primary action. `DECISIONS.md` and `DESIGN.md` §1.

1. **Hero — the promise and the call to action.** One sentence saying what the
   product does for an owner, not what it is built from. The CTA goes to
   `/signin`.
2. **The problem.** Why a bank export and an accountant's ledger do not add up
   to a management report. This is the section that earns the rest.
3. **How it works — three steps.** Upload, we classify, you read the P&L. The
   ingest pipeline is fixed (`openspec/config.yaml`) and it is the honest story.
4. **The product itself.** A real P&L, rendered with the real components and
   fixture data — not a screenshot. It is the strongest asset the product has,
   it costs nothing extra once `/app` exists, and it never goes stale.
5. **Closing call to action.** The same action as the hero. No second one.

Unlumen's motion pieces belong in 1, 2 and 5. They do not belong in 4, which
must read as the application does.

### Imports `/app/imports`

1. **The batch list.** State, `source_kind` tag, period covered, row count, when.
   `DESIGN.md` §8.
2. **Upload.** Asks for `ledger` or `bank` *before* the file is read — the tag
   decides the accounting basis and is not optional.
3. **Per-batch counts.** Rows imported, duplicates skipped, internal transfers
   found, matches proposed. `DESIGN.md` §2 lists these among the things that may
   never be removed.
4. Filters and history.

Empty state: a real one. No fake upload control while change 2.1 is unbuilt
(`DECISIONS.md` §2.5).

### Batch detail `/app/imports/$batchId`

1. **The outcome, and the reason.** A rejected batch says why, at the top.
2. **The validation error list**, keyed by the **original file line number**.
   Never the parsed row index. This is an invariant in `CLAUDE.md`, not a
   preference, and the list is downloadable.
3. The counts from the list view, in full.
4. The file's own metadata — name, size, charset and delimiter detected.

### Review `/app/review`

1. **The current counterparty group**, sorted by amount then repeat count.
2. **The keyboard legend, always visible.** Never behind a help icon.
   `DESIGN.md` §8. Digits pick a category, `Enter` approves, `T` internal
   transfer, `N` not in the P&L.
3. Queue position and what remains.
4. The transactions in the group.

### Management P&L `/app/reports/pnl`

1. **The basis label** — cash-basis or accrual, derived from `source_kind` and
   never chosen by a human. At the top of the table, not in a footnote.
2. **The table.** Categories by section down; periods across; then Total and
   percent of revenue. Frozen category column, frozen header row.
3. **The reconciliation strip** — opening, in, out, transfers, closing. At the
   bottom. `WORKFLOW.md` §5.1: "this is what proves to an accountant that
   nothing was dropped."
4. **Blocked lines, with their reason named.** A line computed from mixed
   `source_kind` without a confirmed D4 match is blocked, not guessed.
5. The period range control, in the top bar.

Charts are Commercial (`WORKFLOW.md` §5.3). The Demo dashboard is this table.

### Drill-down `/app/reports/pnl/cell/$category/$period`

1. **Which figure this is** — category, period, amount. The panel must say what
   was clicked, because a shared link arrives with no memory of the click.
2. **The transactions**, each with category, engine layer, confidence, and D4
   match evidence where there is any.
3. Provenance: `taxonomy_version`, `ruleset_version`, `engine_version`. These
   three strings are what make a March report reproduce in June.

## 4. User flows

### First run

1. A stranger lands on `/`.
2. Presses the call to action → `/signin`.
3. Presses "Continue with Google" → top-level navigation to
   `/auth/google/start` (see §0 — this is currently broken).
4. Google returns to `/auth/google/callback`; core sets the session and
   redirects to `/app`.
   - Failure returns `/?auth_error=<code>`; the client translates the code.
     The backend returns codes, never sentences.
5. `/app` redirects to `/app/reports/pnl`.
6. No data exists → the P&L's empty state, which names what is missing and
   points at Imports.

### Monthly close — the product's real loop

1. Imports → upload, choosing `ledger` or `bank`.
2. Validation is blocking and atomic. A failed file persists nothing.
   - Rejected → batch detail, error list by original line number, fix, re-upload.
   - Imported → counts shown; the rail's Review badge increases.
3. Review → clear the queue by keyboard.
4. Reports → read the P&L.
5. A figure looks wrong → click it → drill-down.

### Sharing a figure

1. Open a drill-down.
2. Copy the URL. It carries the report, the cell and the period range.
3. A colleague opens it, signs in if needed, and lands on the same panel with
   the same range — in their own language and theme, which are theirs and not
   the sender's (`DECISIONS.md` §2.4).

Tenant context is not in that link. It comes from the recipient's session, and
a recipient without access to the organisation gets nothing.

## 5. Naming conventions

One word per concept, everywhere. The Russian column is a draft by a non-native
writer and needs a native check before change 5.1b freezes the catalog —
`DESIGN.md` §12 Q1, still open.

| Concept | UI label (en) | UI label (ru, draft) | Note |
| --- | --- | --- | --- |
| The product | Veekst | Veekst | **Two spellings, on purpose.** The brand is `Veekst`, because `veekst.com` is the domain. Every code identifier stays `vekst` — the Go module, `@vekst/web`, `vekst_app` and `vekst_migrator`, the Kubernetes Services. Renaming those buys nothing and touches the database roles. The brand is a string in the message catalogue; it is not an identifier |
| The customer company | Organisation | Организация | Never "company", "account" or "tenant". "Account" is a bank account here |
| A legal entity within it | Entity | Юр. лицо | One per organisation in v1. The word ships anyway |
| A bank or ledger account | Account | Счёт | The third tenancy level. Not a login |
| One uploaded file and its result | Batch | Пакет | Not "import", which is the action, and not "upload", which is one step of it |
| Where a batch came from | Ledger / Bank | Учёт / Банк | `source_kind`. Chosen before the file is read |
| The accounting basis | Cash-basis / Accrual | Кассовый / Начисление | Derived from `source_kind`, never chosen |
| The other party | Counterparty | Контрагент | The review queue groups by it |
| A classification bucket | Category | Категория | Not "tag", not "label" |
| Awaiting a human decision | Needs review | На проверку | `DESIGN.md` §7 |
| Cannot be computed honestly | Blocked | Заблокировано | Always with its reason in the same view |
| The report | Management P&L | Управленческий ОПиУ | Not "income statement", not "P&L" alone |
| Opening the figures behind a number | Drill-down | Детализация | |

`DESIGN.md` §7's nine state words are the authority for state. This table adds
only the nouns around them.

## 6. Component reuse map

| Component | Used on | Variations |
| --- | --- | --- |
| `PublicLayout` | `/`, `/signin` | Register A scales. No rail, no top bar |
| `AppLayout` | every `/app/*` route | Rail + top bar + outlet. Register B scales |
| `Rail` | `AppLayout` | Review carries a count; nothing else does |
| `TopBar` | `AppLayout` | Organisation and entity render as labels while v1 has one of each |
| `DataTable` | Imports list, review group, drill-down list | 32 px rows. Not the P&L — see below |
| `PnlTable` | P&L only | Frozen column and header, a column per period. Distinct from `DataTable` because freezing and horizontal scroll are its whole job |
| `Panel` | Drill-down | The only overlay in the Demo, and one of the few places a shadow is allowed (`DESIGN.md` §5) |
| `StateChip` | Imports, batch detail, review, P&L | Nine states, three axes, one component. `DESIGN.md` §7 |
| `Money` | Everywhere a figure appears | Right-aligned, tabular numerals, formatted from `int64` minor units. Never a JS `number` |
| `EmptyState` | All three sections | Names what is missing; offers an action only where one is wired |
| `Kbd` | Review legend | From Unlumen (`DECISIONS.md` §5) |
| `Skeleton` | Every async view | From Unlumen. Retires `FRONTEND_PLAN.md` gap 9's loading third |

`DataTable` and `PnlTable` being separate is deliberate. A single table
component that also freezes columns and scrolls horizontally becomes the most
complex thing in the codebase and is used correctly in exactly one place.

## 7. URL strategy

### Pattern

`/app/<section>/<resource>/<sub-resource>`, lower-case, hyphenated. Segments
name things; search params carry view state.

### Dynamic segments

| Segment | Holds | Example |
| --- | --- | --- |
| `$batchId` | Opaque batch identifier | `/app/imports/9f3c…` |
| `$category` | Category slug, stable across taxonomy versions | `logistics` |
| `$period` | `YYYY-MM` | `2026-03` |

A category slug rather than an id, because these URLs are read by people and
appear in messages. It must be stable across `taxonomy_version`, which is a
constraint on change 3.1.

### Search params

| Param | On | Values | Default |
| --- | --- | --- | --- |
| `from` | any report route | `YYYY-MM` | January of the current year |
| `to` | any report route | `YYYY-MM` | The current month |
| `auth_error` | `/` | An error code from core | absent |

Year-to-date by default. Months are the grain, so month precision is the honest
unit and the URL stays readable. `DESIGN.md` §5 already expects a frozen
category column and horizontal scroll, so a wide default range costs nothing.

Rejected: a trailing-12-month default. Better for trend, but it crosses the year
boundary, which is where an accountant's mental model breaks.

Rejected: named tokens such as `?period=ytd`. Shorter, and shareable as intent
rather than as dates — but a link then means something different next month,
and this product's entire claim is that a figure is reproducible.

**Never in the URL:** organisation, entity (§2), language, theme
(`DECISIONS.md` §2.4), or any tenant identifier. Tenant context comes from the
session.

### Reserved prefixes

`/rpc` and `/auth` belong to `core`. The React router must not define a route
under either, and the Ingress and the dev proxy must both send them to `core` —
which, as §0 records, `/auth` currently does not.

## 8. Content growth

| Surface | Grows by | Accommodation |
| --- | --- | --- |
| Imports list | One row per uploaded file, forever | Newest first, paginated. Filter by state and `source_kind` |
| Batch error list | Up to one row per line in a rejected file | Windowed, and downloadable — the download is the real answer for a large file |
| Review queue | Every low-confidence row from every import; drains as it is worked | **Windowed.** `FRONTEND_PLAN.md` gap 7: twelve months of first-time bank data is thousands of rows, and `@tanstack/react-virtual` is not installed |
| P&L | **Horizontally**, one column per period | Frozen category column and header row. The one table that grows sideways, which is why it is its own component |
| Drill-down list | Transactions behind one cell | Windowed |
| Landing | New sections over time, toward Commercial's "full site" | One page until it is genuinely two. Routes are the last resort, not the first |

The P&L growing sideways is the structural fact that most shapes this IA.
`DESIGN.md` §1.1 names the failure mode precisely: a minimal redesign that buys
white space with padding lowers the rows per screen, "and an accountant who
compares twelve periods then scrolls to compare."

## 9. Open

| # | Item | Owner |
| --- | --- | --- |
| 1 | The `/auth` routing defect in §0 | Needs both reviewers; blocks sign-in |
| 2 | The Russian column in §5 and `DESIGN.md` §7 needs a native check | Before 5.1b |
| 3 | Category slug stability across `taxonomy_version` | Change 3.1 |
| 4 | Whether a `viewer` sees the review queue read-only or not at all | `FRONTEND_PLAN.md` §7 Q5. Cheapest answer: hide it |
