# Tasks — connect-app-end-to-end (change 5.3)

Two-hour chunks. Order is the one `openspec/config.yaml` fixes: migration, sqlc queries,
proto, Go service, Python service, tests, React, i18n keys.

Track ownership: §1–§7 are shared (the migration is Track A's week; the identity and
review halves are Track B's). §8–§9 are Track A. **Do not both edit
`proto/vekst/v1/identity.proto`.**

## 0. Decisions closed before code

- [x] 0.1 One organisation per user for the Demo, confirmed. Already decided in
  `add-web-experience/design.md:147` ("v1 gives one organisation per user and one entity
  per..."); `already_a_member` matches it. Not reopened here.
- [x] 0.2 The currencies gap gets a real table, not a form-only workaround. `2.5`
  (`add-transaction-ledger`) is merged and complete — "defer to 2.5" deferred to nothing,
  since 2.5 already validates currency by shape only and nobody is coming back to it.
  §1 below builds `currencies`, seeded with the full ISO-4217 list from
  `core/internal/money`'s exponent map, not only the three Demo pilot pairs (BY/BYN,
  KZ/KZT, PL/PLN) — those three remain what the first-run form's dropdown *offers*, which
  is a UX choice independent of what the schema and Go validation *accept*.
- [x] 0.3 D-11 confirmed: **Veekst** (double `e`), not Palm and not literal "Vekst" — see
  `add-web-experience/proposal.md` and its shipped use in `web/index.html` and
  `web/src/i18n.ts`. The first-run screen uses the same message-catalogue entry as every
  other screen. Update `docs/IMPLEMENTATION_PLAN.md`'s D-11 row to Closed, citing
  `add-web-experience`.
- [x] 0.4 §2.4's consequence, checked: none. `db.CreateOrganization` already contains the
  one counted `OrgIDForNewOrg()` call site (`createorg.go:46`), untouched since PR #5.
  This change adds a caller of `CreateOrganization`, not a second textual call to
  `OrgIDForNewOrg()`, so `scripts/check-db-entry-point.sh`'s committed count does not
  change. A diff showing up in review is worth a second look, not a routine bump.

## 1. Migration 00018 — `category_templates` and `currencies`

- [x] 1.1 Teach `eval/emit.py` to write `eval/out/industry_template.sql` from
  `industry_template.csv`, in the shape `seed_categories.sql` already has. Generated, not
  hand-written: the CSV is the source and the migration is a copy. `write_industry_template_sql()`
  reads the CSV directly (no `../docCl` dependency), verified by running it standalone.
- [x] 1.2 Small Go generator for the currency seed (design §2.2):
  `core/internal/money/cmd/gen-currency-seed`, plus an exported `money.Codes()` to
  enumerate the exponent map (nothing exported it before). No existing `go:generate`
  convention to reuse, confirmed by grep — this is new, not a copy of an existing pattern.
- [x] 1.3 Wrote `core/migrations/00018_category_templates_and_currencies.sql` — both DDLs
  (design §2.1 and §2.2), both `GRANT SELECT`s, the 60 `category_templates` rows, and the
  177-row `currencies` seed, both diffed byte-for-byte against their generators before
  being embedded.
- [x] 1.4 Extended `scripts/check-taxonomy-seed.sh` with a fourth check: the migration's
  copy of the industry template agrees with `eval/out/industry_template.sql`, row for
  row. Verified passing.
- [x] 1.5 New `scripts/check-currency-seed.sh`: runs the generator live and diffs its
  output against the migration's copy (no intermediate file needed — the source has no
  external dependency, unlike the taxonomy). Wired into `make lint`. Verified passing.
- [x] 1.6 Added `category_templates` and `currencies` to `deploy/db/rls-exempt-tables.txt`
  with their own explanatory paragraph, matching the file's existing style.
- [x] 1.7 Test: ran `TestRLSCoverageOfTheRealSchema` against a live database — passes with
  both new tables; removing the `currencies` allowlist line makes it fail with "currencies
  does not have row-level security enabled," confirmed and then restored. Allowlist is
  load-bearing.
- [x] 1.8 `migrate up`, `migrate down`, `migrate up` again against a live Postgres
  (docker, matching CI's `deploy/db/provision-migrator.sql` setup) — clean at every step,
  both tables confirmed dropped after `down` and 60/177 rows present after the second `up`.

## 2. sqlc queries

- [x] 2.1 `taxonomy.sql`: `IndustryTemplate :many` — every `category_templates` row for a
  taxonomy version, ordered by `level`, then `code`. Generated cleanly (`CategoryTemplate`
  model, `IndustryTemplate` query), verified with `go build`/`go vet`.
- [x] 2.2/2.3 **Deviation from the plan, found while implementing.** Neither
  `ListOrganisationsForUser` nor `CountMembershipsForUser` can be a plain sqlc query:
  `memberships`' RLS policy is scoped to `app_current_org()`, and at both call sites
  (`GetCurrentUser`, and the `already_a_member` check before a org exists) **there is no
  tenant context yet** — the exact reason `orgs_for_user()` is a `SECURITY DEFINER`
  function in the first place, and `tenancy.sql`'s own comment already documents that
  sqlc cannot compile against it. Built instead: `db.MembershipsForUser` in `orgid.go`,
  beside `OrgIDForSession`, using the identical `InSystemTx` + `orgs_for_user` pattern —
  not a new constructor, the same door, answering "which ones" instead of "is it this
  one". `GetCurrentUser` loops the result and fetches each org via the *existing*
  `GetOrganization`/`ListEntities` queries under proper per-org `InTx`; the
  `already_a_member` refusal just takes `len()`. This is a genuine new `InSystemTx` call
  site, so `scripts/check-db-entry-point.sh`'s committed count went from 8 to 9, with the
  reason recorded in both the script and `InSystemTx`'s own doc comment (now "four kinds",
  not three). Verified: `go build`, `go vet`, `check-db-entry-point.sh` all pass.
- [x] 2.4 `go tool sqlc generate` run — clean diff (`CategoryTemplate` model,
  `IndustryTemplate` query only), `go build`/`go vet` pass. `buf generate` deferred to
  §3, since no `.proto` file has changed yet.

## 3. Proto

- [x] 3.1 New `proto/vekst/v1/org.proto`: `OrgService`, `CreateOrganizationRequest`,
  `CreateOrganizationResponse`, exactly as design §3.1, comments included. **Needs both
  reviewers before merge.**
- [x] 3.2 Extended `proto/vekst/v1/identity.proto` with `Entity`, `Organisation` and
  `GetCurrentUserResponse.organisations = 2`. `User` itself untouched, but its doc comment
  was stale (predicted `User` would gain the field; fixed to say what actually happened).
  **Needs both reviewers before merge.**
- [x] 3.3 Extended `proto/vekst/v1/review.proto` with `ListCategories` and its three
  messages. **Needs both reviewers before merge.**
- [x] 3.4 `buf lint`: clean. `buf breaking --against '.git#branch=main'`: clean (every
  change is additive, as designed).
- [x] 3.5 `make gen` ran clean for Go, TypeScript and Python. Confirmed: no file under
  `classifier/` changed (`git status --porcelain classifier/` empty) — `org.proto` and
  `identity.proto` are browser-facing only, as expected. `go build ./...` now fails on
  `core/internal/server` (missing `ListCategories` on `reviewHandler`) — expected, that's
  §5's job.

## 4. Go — `core/internal/tenancy`

- [x] 4.1 New package `core/internal/tenancy`, doc comment included. **Deviation, found
  while implementing:** it does not itself hold the adoption code — see 4.2/4.3.
- [x] 4.2/4.3 **Deviation from the plan, found while implementing.**
  `AdoptIndustryTemplate` cannot live in `core/internal/tenancy` and be called directly
  from `db.CreateOrganization`: `tenancy` has to import `core/internal/db` (for `OrgID`,
  `DB`, `MembershipsForUser`), so `db` importing `tenancy` back would be a cycle. Built
  instead: `adoptIndustryTemplate(ctx, tx, org, taxonomyVersion)`, unexported, in
  `core/internal/db/adopttemplate.go` — called from `CreateOrganization` itself, after
  `InsertEntity` and before `InsertMembership`, inside the same transaction. `tenancy`
  still owns organisation creation as a product action (validation, the
  `already_a_member` refusal, calling `db.CreateOrganization`); this one piece has to
  live beside the transaction it runs in. Needed one addition §2 didn't list: a new sqlc
  query, `InsertCategory`. Verified against a live database: `TestAdoptIndustryTemplateResolvesEveryParent`
  (60 rows land, `030101` hangs under global `0301`, `0401010101` hangs under this
  organisation's own adopted `04010101`, every `parent_id` resolves within the read),
  `TestAdoptedCategoriesAreNotVisibleAcrossOrganisations` (task 7.1, cross-tenant),
  `TestAdoptionFailureRollsBackTheWholeOrganisation` (task 7.4, atomicity, via `InTx`'s
  ordinary rollback — `category_templates` is read-only to `vekst_app` so the real
  unresolvable-parent path can't be forced from a test, but the guarantee it depends on
  is exercised directly). All in `core/internal/db/adopttemplate_test.go`.
- [x] 4.4 `tenancy.Service.Create(ctx, userID, in)`: validates country against a Go
  allowlist (BY/KZ/PL, matching the three Demo pilot markets — there is no countries
  table either) and currency against `money.Exponent` (the database checks only the
  shape); refuses an existing member with `already_a_member`; calls
  `db.CreateOrganization`; reads the entity id back with one `InTx` under the new
  organisation (`CreateOrganization` itself still returns only the `OrgID`, unchanged for
  its nine existing test callers). Verified: `TestSecondOrganizationIsRefused` (7.5),
  `TestUnsupportedCurrencyIsRefusedBeforeAnythingIsWritten` (7.6, using `XXX`),
  `TestUnsupportedCountryIsRefusedBeforeAnythingIsWritten` — all against a live database,
  all confirming nothing is written on refusal.
- [x] 4.5 Error codes: `org_name_required`, `org_entity_name_required`,
  `org_unsupported_country`, `org_unsupported_currency`, `org_already_a_member`.
- [x] 4.6 `core/internal/server/org.go`: the Connect handler. Session only, no membership
  resolution — `tenancy.Service.Create` takes a bare `userID`, never calls
  `db.OrgIDForSession`.
- [x] 4.7 Registered `OrgService` in `server.go`, nil-guarded like `reviewer`/`reporter`;
  threaded a `*tenancy.Service` through `server.New`'s signature and `main.go`'s
  construction (`tenancy.NewService(database)`). Fixed the four test call sites this
  broke (`auth_test.go`, `health_test.go` x3, `readyz_test.go`) — an extra `nil` argument
  each, matching how the two existing optional services are already tested.
- [x] 4.8 Confirmed: no edit needed. `OrgIDForNewOrg`'s committed count stayed `1`
  (`check-db-entry-point.sh` passes unchanged) — exactly the 0.4 finding. **One count did
  change, for a different, real reason**: `db.MembershipsForUser` (§2.2/2.3's deviation)
  is a genuine new `InSystemTx` call site, so that committed count went `8` → `9`, with
  the reason recorded in the script and in `InSystemTx`'s own doc comment.

## 5. Go — identity and review

- [x] 5.1 `identity.Service.Organisations` (new file `organisations.go`): `MembershipsForUser`
  then `GetOrganization`/`ListEntities` per membership, each under its own `InTx`. Wired
  into `identityHandler` (`server/identity.go`), which now holds `svc *identity.Service`
  instead of `struct{}`. Empty list is a success. Verified against a live database:
  `TestOrganisationsIsolatesOtherOrganisations` (task 7.2 — a second, unrelated
  organisation exists and is absent from the caller's list) and
  `TestOrganisationsIsEmptyForAFirstTimeCaller`.
- [x] 5.2 `review.Service.Categories(ctx, userID, orgID)` over the existing
  `ClassifiableCategories` query, `path` built by walking `EffectiveTaxonomy`'s
  `parent_id` chain. Verified: `TestCategoriesOffersNoComputedLineAndNoSection` (task
  7.7 — GM/NM/CM/IBT/NI and two sections all confirmed absent, against the fixture's real
  adopted taxonomy).
- [x] 5.3 `core/internal/server/review.go`: the `ListCategories` handler, resolving the
  organisation through `caller()`, the same door every other handler in this file uses.

## 6. Python

- [x] 6.1 Nothing, confirmed. `make gen` (§3.5) left every file under `classifier/`
  untouched — `git status --porcelain classifier/` was empty after the run.

## 7. Tests

- [x] 7.1 Done under 4.2/4.3 — `TestAdoptedCategoriesAreNotVisibleAcrossOrganisations`,
  `core/internal/db/adopttemplate_test.go`.
- [x] 7.2 Done under 5.1 — `TestOrganisationsIsolatesOtherOrganisations`,
  `core/internal/identity/organisations_test.go`.
- [x] 7.3 Done under 4.2/4.3 — `TestAdoptIndustryTemplateResolvesEveryParent`.
- [x] 7.4 Done under 4.2/4.3 — `TestAdoptionFailureRollsBackTheWholeOrganisation` (see
  4.2/4.3's note on how it stands in for the literal unresolvable-parent path).
- [x] 7.5 Done under 4.4 — `TestSecondOrganizationIsRefused`,
  `core/internal/tenancy/tenancy_test.go`.
- [x] 7.6 Done under 4.4 — `TestUnsupportedCurrencyIsRefusedBeforeAnythingIsWritten`.
- [x] 7.7 Done under 5.2 — `TestCategoriesOffersNoComputedLineAndNoSection`,
  `core/internal/review/categories_test.go`.
- [x] 7.8 **End-to-end, against a live database and object store**:
  `TestEndToEndOrganisationToReport`, `core/internal/ingest/endtoend_test.go`. No import
  profile needed — `CreateImportBatch` parses the Priorbank format directly. Deviation
  from the plan: does not use `classify.Unavailable{}` (found while writing this — its
  error is not a `classify.IsRejected` rejection, so `classifyrun` treats it as
  retryable and the run never leaves `running`; every other test in this package that
  touches classification only checks that persist *enqueued* it, for the same reason).
  Uses a small local `noProposalsClassifier` instead, added via a new
  `newTestEnvWithClassifier` (the existing `newTestEnvWithConfig` now calls it with
  `classify.Unavailable{}`, unchanged for every other test in the package) — answers
  every chunk instantly with no proposals, a real "classified, nothing met the
  threshold" outcome on the wire, reaching `classified` deterministically. Verified:
  organisation created → industry template adopted → file uploaded → validated →
  deduped → persisted (`StatusImported`) → classification run finishes `classified` →
  `GetManagementPNL`'s `BucketUnclassified` total is non-zero (711,086 NOK minor units
  across 12 lines) — a real figure the pipeline computed, not invented by the test.
- [x] 7.9 `make ci` green, literally: `DATABASE_URL_MIGRATOR=... DATABASE_URL=... make ci`
  exits 0 (`lint` then `test`, per the Makefile). Found and fixed one **pre-existing,
  unrelated** issue blocking it: `TestOrgIDCannotBeForgedOutsideThisPackage/test_constructor.go`
  asserted on a compiler error message (`"want (testing.TB, uuid.UUID)"`) that this Go
  toolchain no longer produces verbatim — it now fully qualifies the type
  (`"github.com/google/uuid".UUID`) rather than using the short alias. Narrowed the
  assertion to `"want (testing.TB, "`, which still proves the complaint is about the
  missing `testing.TB` parameter (the whole point of the test) without depending on how
  a given Go version renders the second argument's type. Confirmed via `git status`
  before touching it that this was pre-existing drift, not something introduced here.
  Also fixed `TestRequiredVersionSeesEverySQLAndGoMigration` (17 → 18, a real
  consequence of the new migration). All three test suites pass under `make ci`: Go
  (every package, live database), Python (35 passed), web (141 passed, 19 files).

## 8. React

- [x] 8.1 `web/src/data/session.ts` — `setSession`, `clearSession`, `requireSession`, and
  a coded throw (`SessionNotSetError`) when unset.
- [x] 8.2 `web/src/data/org.ts` — `createOrganisation`, a real client, shaped like the
  other three real modules. Owns decoding `ConnectError` into a plain
  `CreateOrganisationError { code }`, since a screen must never import
  `@connectrpc/connect` directly (`data.test.ts`'s seam test) — found while wiring
  `FirstRunScreen`, which first imported it directly and failed that test.
- [x] 8.3 `AppLayout`: `setSession` called synchronously in the render body (not a
  `useEffect` — child effects run before parent effects on mount, which would let a
  child's own query fire before the session was set), after `GetCurrentUser` resolves,
  before rendering children. `clearSession` moved into `data/auth.ts`'s `signOut` itself,
  so the cleanup lives in one place regardless of which UI element triggers sign-out.
- [x] 8.4 `AppLayout`: branches on `organisations.length === 0` to `FirstRunScreen`.
- [x] 8.5 `web/src/app/FirstRunScreen.tsx` — four fields (name, entity name, one market
  select fixing country+currency together as one of BY/BYN, KZ/KZT, PL/PLN), one button,
  `useMutation`, invalidates `["currentUser"]`. Replaces `SignedIn.tsx`, which was already
  orphaned (nothing imported it) and is now deleted; its `signedIn.noOrganisation` i18n
  key removed with it.
- [x] 8.6 Rewrote `imports.ts` against `ListImportBatches`, `GetImportBatch`,
  `GetValidationReport` and `GetDedupSummary` (composed into the one `BatchDetail` shape).
  Deleted `resetFixtures()`. **Deviation, found while implementing:** the exported shape
  could not stay identical — `matchesProposed` (D4 match proposals) and `periodFrom`/
  `periodTo` have no backend source anywhere (not even at single-batch granularity), so
  they are dropped rather than always-undefined. `docs/DESIGN.md` §7's four-word batch
  state vocabulary is kept as-is rather than grown to the real 11-value `ImportStatus`:
  collapsed conservatively (before parsing → pending, mid-pipeline → parsing, every bad
  ending — rejected/abandoned/failed → rejected, clean finish → imported), consistent
  with `ui/StateChip.tsx`'s own stated reluctance to add a fourth tone casually.
  `ImportsScreen`/`BatchScreen` updated for the dropped fields (removed the period column
  and the matches tile). Tests: `imports.test.tsx` (component, `vi.mock("../transport")`
  per 8.11) and a new `imports.test.ts` (data-module, the money round trip per 8.12).
- [x] 8.7 Rewrote `review.ts` against `ListReviewGroups`, `ListGroupTransactions`,
  `ResolveGroup`, `UndoDecision`, `ListCategories`. **Deviations, found while
  implementing:** (1) per the earlier decision to drop rather than fake the suggestion
  badge, `suggestedCategoryId`/`suggestedLayer`/`suggestedConfidence` are gone —
  `ReviewScreen`'s badge removed with them. (2) `ReviewGroup` no longer carries its own
  `transactions`: `ListReviewGroups` never returned them, only a per-group summary —
  `listGroupTransactions(counterpartyKey)` is the real, separate call, and `ReviewScreen`
  now fetches it for whichever group is open via its own `useQuery`, not for every group
  eagerly. (3) `ReviewDecision`'s classify variant carries `categoryCode`, not
  `categoryId` — forced by `ResolveGroupRequest`'s own stated reason: a code is what the
  client was given, an id is one it could invent. (4) `review.proto`'s requests spell the
  field `organization_id`, not `org_id` like `import.proto` — a real naming inconsistency
  between the two protos, not a choice; `Session.orgId` itself is unaffected. `Category`
  keeps its fixture field name `label` internally (mapped from the wire's `name`) since
  nothing about the rename would change what `ReviewScreen` shows.
- [~] 8.11 (review) Moved `review.test.tsx` to `vi.mock("../transport")`. Needed
  `vi.hoisted()` for mutable state (`resolvedKeys`) shared between the mock factory and
  `beforeEach`, since a resolved group has to disappear from subsequent
  `listReviewGroups` calls the way the deleted `decided` fixture Set did, and a `vi.mock`
  factory only runs once per file. `router.test.tsx` and `home.test.tsx` also needed
  `ReviewService` stubs added, for the same underlying reason as their `imports.ts` fix
  under 8.6: `AppLayout` calls `reviewSummary()` on every authenticated render, and it is
  real now too. `router.test.tsx` needed a further restructure beyond a plain
  `vi.mock`: it renders many different sessions (signed out, signed in, first-run) from
  one mock factory that only runs once, so its mocked router reads `getCurrentUser`'s
  answer from `vi.hoisted` state that `renderAt` sets before each render, rather than
  building a fresh transport per call the way it used to.
- [x] 8.12 (review) `review.test.ts`: a KWD (not the fixture's EUR) group total and
  `reviewSummary()` total both survive the round trip as strings with the right currency
  code; `reviewSummary` asserted to read the server's own aggregate rather than a
  client-side sum (there is only one group in the stub, so a bug that summed rows instead
  of reading `totalRowCount` could not be told apart otherwise — deliberately using
  `rowCount: 3` on a group whose `transactions` this test never fetches).
- [x] 8.8 Rewrote `report.ts` against `GetManagementPNL` and `ListLineTransactions`.
  **The largest deviation in this change, found while implementing, and discussed before
  starting:** the fixture's `sections: ReportSection[]` (three fixed groups) has no
  backend equivalent. Verified against `core/internal/report/pnl.go` before touching any
  component: `Order` is exactly twelve entries (seven sections, five computed lines,
  interleaved per design D3), never one row per leaf category — so `lines: ReportLine[]`
  (flat, in print order) is a smaller change than it first looked, and `PnlTable` needed
  no section-grouping/subtotal-row logic at all, just a per-line `computed` style. Added
  `buckets: ReportBucket[]` — new, and required: CLAUDE.md's own invariant ("below the
  table: unclassified, excluded non-P&L, unallocated, other basis") was never rendered by
  the fixture because it never had the data. `revenueTotal`, `expensesTotal`, `netTotal`,
  `netByPeriod`, `unreviewedAmount` and `reconciliation` kept their exact old shape and
  meaning (derived from `lines`/`buckets`: NET SALES for revenue, NI for net, the
  unclassified bucket for unreviewed, `revenueTotal − CM` for expenses since no single
  "total expenses" line exists) — **`StatTiles.tsx` and `Reconciliation.tsx` needed zero
  changes.** The reconciliation strip is aggregated from the wire's one-per-period array
  (opening = first period's, in/out/transfers summed, closing derived — never read off
  any period's own field, even at the aggregate) and `out`/`transfers` are negated from
  the wire's positive magnitudes to match this product's existing outflow convention.
  Per-line `blockedReason` (mixed sources) has no wire field — a report is basis-locked
  for its whole duration, so the refusal moved to the whole report: `getReport` derives
  the basis from the entity's imported batches (design §2.5) via `listBatches`, and
  throws `MixedBasisError` on a genuine mix, which `ReportScreen` renders as one blocked
  state replacing the old per-line one. `getDrilldown` now handles both
  `ReportAnswerKind`s: `TRANSACTIONS` (a section or a bucket) and `OPERANDS` (a computed
  line has none of its own — GM is NET SALES minus CS, and both of those have
  transactions) — `DrilldownPanel` gained an operands view, each operand itself a link
  into another drill-down. `rowsUnavailable` (fixture-coverage) is gone; every request
  now returns what is really there, including zero, rendered as a genuine empty state.
  `confidence` is optional now (absent on an unclassified row, which bucket drill-downs
  can reach and line ones could not). `EngineLayer` widened to include `"human"`, the
  wire's real value for a person's own decision. All 5 chart components, `sample.ts` (the
  landing page's illustrative data), `PnlTable`, `ReportScreen` and `DrilldownPanel`
  updated for the new shape; `ExpenseCategoriesChart` and `MoneyFlowChart` now chart
  section-level totals rather than the fixture's invented per-leaf-category breakdown,
  since no endpoint returns that.
- [x] 8.9 Resolved more strongly than asked: `ReportScreen` never learns about
  `ReportBasis` at runtime at all — `getReport` derives and sends it internally
  (`deriveBasis()`), so `REPORT_BASIS_UNSPECIFIED` is structurally unreachable from the
  screen rather than merely avoided by convention.
- [x] 8.10 `TopBar` reads the organisation and entity names from `AppLayout`'s resolved
  session/`GetCurrentUser` data (`add-web-experience` 9.1, 9.2 — closed here, mark them
  closed there).
- [x] 8.11 Moved `imports.test.tsx`, `review.test.tsx` and `report.test.tsx` to
  `vi.mock("../transport")`, the way `dedup.test.ts` does — and had to, not by choice:
  found (on `imports.test.tsx` first) that `stubTransport` passed only to `makeRouter`
  never reaches a data module's own static `transport` import, so a real data module's
  calls went to the real, unmocked transport and every affected test timed out at 5s
  instead of failing clearly. The same fix was then needed on `home.test.tsx` (broke once
  `imports.ts` went real, broke again once `review.ts` and `report.ts` did) and on
  `router.test.tsx`, which needed a further restructure beyond a plain `vi.mock`: it
  renders many different sessions from one mock factory that only runs once, so its
  mocked router reads `getCurrentUser`'s answer from `vi.hoisted` state `renderAt` sets
  before each render. `review.test.tsx` similarly needed `vi.hoisted` state
  (`resolvedKeys`) so a resolved group can disappear from subsequent `listReviewGroups`
  calls the way the deleted `decided` fixture Set did, and `report.test.tsx` needed it
  (`batchSourceKinds`) so one dedicated test can trigger the mixed-basis refusal without
  every other test in the file inheriting it. `stubTransport` gained an `extend(router)`
  hook so a screen test can add its own service stubs onto the same Health+Identity
  router instead of rebuilding it.
- [x] 8.12 **Money test, per rewritten module**, done for all three: `imports.test.ts` (a
  BYN, not the fixture's EUR, balance-check amount survives the round trip, `closing`
  computed distinct from `declared` the wire's own field so the test can't pass by
  coincidence), `review.test.ts` (a KWD group total and `reviewSummary`'s own total both
  survive, and `reviewSummary` is asserted to read the server's aggregate rather than a
  client-side sum), `report.test.ts` (a KWD report: `revenueTotal`/`netTotal`/
  `expensesTotal`/`unreviewedAmount` all derived correctly from real lines and buckets
  rather than typed in, and the reconciliation strip's `closing` is recomputed by the
  test from the other three parts and asserted equal — not merely equal to the wire's own
  field, which this design deliberately never reads).
- [x] 8.13 `data.test.ts`'s "no screen imports a transport" test still passes, unchanged.
  Its *other* describe blocks were not unchanged, and per its own text could not be:
  "shapes the screens depend on" and "fixtures compute rather than assert" called fixture
  functions directly (`imports.ts`, then `review.ts`, then `report.ts`, as each went
  real), and were removed once each one broke — their coverage already exists in the
  corresponding module's own `imports.test.tsx`/`review.test.tsx`/`report.test.tsx` and
  `*.test.ts` files, which is where it belongs now that these are real RPCs rather than
  shared fixture state.
- [x] 8.14 Negative scenario, in `router.test.tsx`: `GetCurrentUser` answering an empty
  list renders `firstRun.heading` and makes no tenant-scoped data call — asserted by the
  absence of `report.title` and `topbar.entity`, which a real call would have produced.

## 9. i18n and close-out

- [x] 9.1 Message keys added throughout §8 as each piece needed them (firstRun.*, the
  five `org_*` refusal codes, `report.bucket.*`, `report.blocked.mixed_basis`,
  `drilldown.operands`, `drilldown.empty`), in `en` and `ru` together each time, plus
  removed keys whose only consumer was deleted (`signedIn.noOrganisation`,
  `report.blocked.mixed_sources_no_match`, `drilldown.unavailable`). Verified by exact
  count, not just spot-checked: both blocks hold precisely 206 keys, zero in one and not
  the other.
- [ ] 9.2 Update `docs/ARCHITECTURE.md` with the onboarding path and the template
  adoption, and `docs/IMPLEMENTATION_PLAN.md` §3 with this change as 5.3.
- [ ] 9.3 Update the capability spec and run the full suite.
- [x] 9.4 Both doc fixes made: `docs/IMPLEMENTATION_PLAN.md`'s D-11 row marked Closed,
  citing where "Veekst" was actually decided and shipped; CLAUDE.md's Money invariant
  corrected to say `transactions` already stores amounts (change 2.5, merged), and that
  this change is what actually added the `currencies` table the old text pointed at.
