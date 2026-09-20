# Design — connect-app-end-to-end (change 5.3)

## 0. Why this is one change and not three

`openspec/config.yaml` says a change touching more than two capabilities should propose a
split. This one touches three. The argument for the exception, rather than a request to
waive the rule:

| Split | What it delivers alone |
| --- | --- |
| tenancy only | An organisation nobody can create from a browser, read by screens that show fixtures. |
| identity-access only | A `GetCurrentUser` that returns an empty list for every user, forever. |
| web-app only | Three screens calling RPCs with an organisation identifier that does not exist. |

Each half needs the other two to be observable. The change is also the first one in the
project whose acceptance test is the product working, so its test cannot live in a split
half. The split that *is* honest — onboarding, then wiring — is a strict sequence of two
one-day pieces with a shared migration and a shared proto change, which costs a merge and
buys nothing at nine working days from the Demo.

If a reviewer disagrees, the cut line is §3 (the browser) against §1–§2 (the backend).
Nothing in §3 is imported by §1 or §2.

## 1. What is disconnected today — evidence, not recollection

| Claim | How it was checked |
| --- | --- |
| `db.CreateOrganization` has no production caller | `grep -rn "CreateOrganization(" --include=*.go core`, excluding `_test.go` and its own definition: **0 results** |
| No RPC creates an organisation | all 23 `rpc` lines in `proto/vekst/v1/*.proto` read |
| Sign-in creates no organisation | `identity/signin.go` `resolveOrProvision`: `InsertUser` + `InsertIdentity`, nothing else |
| `GetCurrentUser` carries no organisation | `identity.proto`: `User` is `{id, email, name, locale}`; the comment says "1.1 extends this message" and 1.1 did not |
| Three data modules are fixtures | `imports.ts`, `report.ts`, `review.ts` import no transport and no generated client; `imports.ts` exports `resetFixtures()` |
| No category list RPC | `grep -rn "ListCategories" proto/ core/internal/server/`: **0 results**; `web/src/data/review.ts:99` `listCategories` is a fixture |
| The 60 industry leaves are never adopted | `eval/out/industry_template.csv` has 60 rows; migration 005 seeds 48 `'global'` rows and no adoption query exists in `taxonomy.sql` |
| One organisation per user is already decided, not open | `add-web-experience/design.md:147`: "v1 gives one organisation per user and one entity per..." — task list item 0.1 needs a citation, not a decision |
| `add-transaction-ledger` (2.5) is merged and complete, not a place to defer to | `openspec list --json`: status `complete`, 38/38 tasks, merged PR #10; `core/migrations/00007` already has `amount_minor`, `currency`, `fx_rate`, `fx_rate_on`, `base_amount_minor`, `base_currency` — no `currencies` table, by 2.5's own choice |

Already connected, and this change must not re-do it: upload (`importUpload.ts`),
validation (`validation.ts`), dedup (`dedup.ts`), the classify enqueue inside the
`imported` transaction (change 3.4), account auto-creation (`ingest/account.go`), and the
classifier gRPC dial (`main.go` + `VEKST_CLASSIFIER_ADDR`).

## 2. Data model

### 2.1 Migration 00018 — `category_templates`

```sql
-- The industry template: the 60 leaves eval/out/industry_template.csv holds.
-- It belongs to nobody. It is NOT a tenant table and never holds a tenant row,
-- which is why it carries no org_id, no policy and a line in
-- deploy/db/rls-exempt-tables.txt. A customer's copy of a template row is an
-- ordinary categories row with scope='org' and that customer's org_id, written
-- by AdoptIndustryTemplate under that customer's own policy.
CREATE TABLE category_templates (
    taxonomy_version text     NOT NULL,
    code             text     NOT NULL,
    parent_code      text     NULL,
    level            smallint NOT NULL CHECK (level BETWEEN 1 AND 5),
    name             text     NOT NULL,
    is_leaf          boolean  NOT NULL,
    is_pnl           boolean  NOT NULL,

    PRIMARY KEY (taxonomy_version, code),

    -- A code is hierarchical, two characters per level, exactly as migration
    -- 005 resolves parents. Stating it here stops a hand-edited row from
    -- carrying a parent_code that its own code contradicts.
    CONSTRAINT category_templates_code_encodes_level
        CHECK (length(code) = level * 2),
    CONSTRAINT category_templates_parent_is_prefix
        CHECK (parent_code IS NULL OR code LIKE parent_code || '__')
);

GRANT SELECT ON category_templates TO vekst_app;
```

`vekst_app` gets `SELECT` and nothing else. The seed is generated into
`eval/out/industry_template.sql` by `eval/emit.py` and copied into the migration, the
same arrangement as 005 and 006, so `scripts/check-taxonomy-seed.sh` can diff it.

**No change to `categories`.** No new column, no widened `CHECK`, no changed policy.

### 2.2 Migration 00018 — `currencies`

Open question 3 below used to describe this as "not yet closed by 2.5" — it isn't
pending, it's orphaned. `add-transaction-ledger` (2.5) is merged and complete
(`openspec list --json`: `complete`, 38/38), and it validates `transactions.currency`
and `transactions.base_currency` by shape only (`CHECK (currency ~ '^[A-Z]{3}$')`), the
same as `organizations.base_currency`. Nobody is coming back to add a reference table as
part of 2.5, because 2.5 is closed. This change adds one, in the same migration as
`category_templates` since both are schema opened this week.

```sql
-- The ISO-4217 codes this product accepts, and the minor-unit digits each
-- divides into. Not a tenant table -- no org_id, no policy, a line in
-- rls-exempt-tables.txt, exactly like category_templates: it belongs to
-- nobody and never holds a tenant row.
CREATE TABLE currencies (
    code     text     NOT NULL PRIMARY KEY,  -- ISO-4217
    exponent smallint NOT NULL,

    CONSTRAINT currencies_code_shape CHECK (code ~ '^[A-Z]{3}$')
);

GRANT SELECT ON currencies TO vekst_app;
```

Seeded with the full ISO-4217 list (~155 rows), not only the three Demo pairs (BY/BYN,
KZ/KZT, PL/PLN): a partial seed reproduces the exact problem this table exists to close
— it would need a row added every time a new pilot country shows up — and generating all
of them from the existing source costs the same script as generating three.

The source is `core/internal/money/exponents.go`'s `exponent` map — already the
project's canonical currency data, transcribed from the ISO 4217 Maintenance Agency's
list and explicitly marked "not a generated artefact" from `/proto`. It is Go data, not
`eval/`'s domain (`eval/emit.py` reads a taxonomy source outside the repository and has
no reason to know about money), so the generator is new: a small Go program —
`core/internal/money/cmd/gen-currency-seed`, or a `go:generate` directive on
`exponents.go` — that writes migration 00018's `INSERT` rows from the map. Same
relationship `category_templates` has to `industry_template.csv`, but Go-to-SQL rather
than Python-to-SQL, because no `go:generate` convention exists in this codebase yet to
reuse (checked: `grep -rn "go:generate" core` returns nothing). A `check-currency-seed.sh`
diffs generator output against the migration's copy, alongside `check-taxonomy-seed.sh`
in `make lint`.

`organizations.base_currency` and `transactions.currency`/`base_currency` are not
touched — no new foreign key, no widened `CHECK`. Upgrading those into a real reference
is a separate, low-risk change; bundling it here risks exactly the scope creep this
change's own non-goals warn against.

### 2.3 Adoption

`AdoptIndustryTemplate(ctx, tx, org)` runs inside `CreateOrganization`'s existing
transaction, under that organisation's own `categories_write` policy.

Ordering is by `level` ascending, because a level-5 leaf's parent is a level-4 row this
same loop inserted. `parent_id` resolves as: **this organisation's row with
`parent_code`, else the global row with `parent_code`.** Both are needed — `030101`
hangs under global `0301`, and `0401010101` hangs under the org's own `04010101`.

A template row whose parent resolves to neither **fails the transaction**. An
organisation with a dangling leaf is not a state worth being able to represent, and
migration 005's `AFTER` trigger already refuses a leaf under another organisation's
parent.

Verified before writing this, so the task list is not planning around a guess: the 60
template rows name 13 parent codes that are not themselves template rows — `0301`, `04`,
`0401`, `040101`, `04010102`, `040102`, `04010202`, `04010401`, `0402`, `0405`, `0501`,
`050103`, `0502` — and **all 13 are seeded as `scope = 'global'` rows by migration 005**.
Every remaining parent is another template row at one level up. All 60 rows satisfy
`length(code) = level * 2` and `code = parent_code || '<two chars>'`, which is what makes
the two CHECK constraints above safe to state rather than aspirational.

Adoption is not idempotent and does not need to be: it runs once, inside the transaction
that creates the organisation, and `categories`' `UNIQUE NULLS NOT DISTINCT
(taxonomy_version, org_id, code)` refuses a second run.

### 2.4 Tenancy callout

`CreateOrganization` is the **only** RPC that runs without a prior membership. It reaches
`db.OrgIDForNewOrg`, the third of the three sanctioned `OrgID` constructors — but the
committed call-site count in `scripts/check-db-entry-point.sh` does **not** change.
`db.CreateOrganization` already exists and already calls `OrgIDForNewOrg()` exactly once
(`core/internal/db/createorg.go:46`), and that is the call site the script's committed
`1` already counts — both lines have sat untouched since PR #5 ("Add tenancy and rls"),
through every Demo change merged since. This change adds a caller of
`db.CreateOrganization`; it does not add a second textual call to `OrgIDForNewOrg()`. No
diff to the script is expected, and one showing up in review is worth a second look
rather than a rubber stamp.

RLS is unchanged and unweakened. `CreateOrganization` inserts under
`organizations`' own `id = app_current_org()` policy exactly as it does today, because
the identifier is minted before the transaction opens.

### 2.5 Money callout

**This change adds no money field and no money column.** `currencies` (§2.2) stores an
exponent, not an amount. It does move money across a
seam that currently carries none: `report.ts` and `review.ts` stop computing
`Money` in the browser and start decoding `vekst.type.v1.Money` off the wire. The
existing invariant applies unchanged — minor units are a string, never a JavaScript
`number` — and the existing `web/src/money.ts` helpers are what the rewritten modules
must use. A non-base-currency test and a no-float test are in the task list for each
rewritten module, not only for the backend.

**Aside, not this change's job:** CLAUDE.md's own Money invariant says "no table stores
an amount yet... [multi-currency storage] is change 2.5." That is stale —
`add-transaction-ledger` merged in PR #10, and `transactions.amount_minor` and
`base_amount_minor` have stored real amounts since. Worth a one-line correction to
CLAUDE.md, separately from this change, so it stops telling the next reader that no
table stores money.

### 2.6 `source_kind` callout

`report.ts`'s fixture picks no basis. `GetManagementPNLRequest` refuses
`REPORT_BASIS_UNSPECIFIED`, deliberately, so the browser must now send one. For the Demo
the report screen sends the basis of the entity's imported batches, and refuses to render
when an entity holds both kinds — which is the invariant, not a limitation of this
change. Deriving a default is not this change's work.

## 3. API

### 3.1 New — `proto/vekst/v1/org.proto`

Browser-facing, Connect.

```proto
service OrgService {
  // The one call a signed-in caller with no membership may make.
  //
  // Authenticated, and deliberately not authorised against a membership: a
  // caller who has one is refused with already_a_member rather than admitted,
  // because the Demo gives a person exactly one organisation and a second
  // would be a second tenant nobody asked for.
  rpc CreateOrganization(CreateOrganizationRequest) returns (CreateOrganizationResponse);
}

message CreateOrganizationRequest {
  string name          = 1;
  string country       = 2;  // ISO-3166-1 alpha-2
  string base_currency = 3;  // ISO-4217, validated in Go against money.Exponent
  string entity_name   = 4;  // the single entity every organisation has in the Demo
}

message CreateOrganizationResponse {
  string organization_id = 1;
  string entity_id       = 2;
}
```

Error codes, never sentences: `invalid_argument`, `unsupported_currency`,
`unsupported_country`, `already_a_member`.

### 3.2 Modified — `proto/vekst/v1/identity.proto` (additive)

```proto
message Entity {
  string id   = 1;
  string name = 2;
}

message Organisation {
  string id            = 1;
  string name          = 2;
  string base_currency = 3;
  string role          = 4;  // the caller's own membership role
  repeated Entity entities = 5;
}

message GetCurrentUserResponse {
  User user = 1;
  // Empty means the caller has no organisation. That is the first-run signal,
  // and it is a fact rather than an error: a person who has just signed in for
  // the first time has not failed at anything.
  repeated Organisation organisations = 2;
}
```

`User` is untouched. Field 2 is an addition, so `buf breaking` passes; the task list runs
it anyway, because the rule is per-change and not per-judgement.

`memberships` is read under RLS for the organisation identifiers, then organisations and
entities are fetched by key. No join to `users`: `scripts/check-identity-queries.sh`
keeps the four identity tables reachable from one query file, and this read does not need
them.

### 3.3 Modified — `proto/vekst/v1/review.proto` (additive)

```proto
  // The categories a human may choose in the review queue. Backed by the
  // existing ClassifiableCategories query: computed lines and non-leaf nodes
  // are not choices, and a picker offering them produces a classification the
  // report cannot place.
  rpc ListCategories(ListCategoriesRequest) returns (ListCategoriesResponse);

message ListCategoriesRequest  { string organization_id = 1; }
message Category {
  string id     = 1;
  string code   = 2;
  string name   = 3;
  string path   = 4;  // "OPEX > Administration > Finance", for a searchable picker
  bool   is_pnl = 5;
}
message ListCategoriesResponse { repeated Category categories = 1; }
```

No new table and no new query: `taxonomy.sql`'s `ClassifiableCategories` already returns
this.

## 4. The browser

### 4.1 The session seam

`add-web-experience` design D1 and its test in `web/src/data/data.test.ts`: **no screen
imports a transport or a generated client**, and connecting the backend replaces a module
body without touching a component. `imports.ts`, `report.ts` and `review.ts` take no
organisation argument today, and giving them one would push the identifier through every
screen and break that test.

So the identifier becomes ambient, in one new module:

```ts
// web/src/data/session.ts
export interface Session { orgId: string; entityId: string; baseCurrency: string }
export function setSession(s: Session): void
export function clearSession(): void
export function requireSession(): Session  // throws a coded error if unset
```

`AppLayout` calls `setSession` once, after `GetCurrentUser` resolves and before it renders
any screen. The data modules call `requireSession()`. Screens stay ignorant, the seam test
passes unchanged, and there is exactly one place that knows which organisation is on
screen.

A data call before `setSession` is a programming error and throws, rather than sending an
empty `organization_id` that the server would answer `ErrNotAMember` to — a wrong error in
front of a right one is the harder bug.

### 4.2 The first-run screen

`AppLayout` already gates the whole subtree on `GetCurrentUser`. It gains one branch:
`organisations.length === 0` renders `FirstRunScreen` instead of the children. Four
fields, one button, `useMutation`, then invalidate `["currentUser"]`. No router change —
first-run is a state of the shell, not a route, and a route would be reachable by a user
who already has an organisation.

### 4.3 The three modules

| Module | Replaced by | Notes |
| --- | --- | --- |
| `imports.ts` | `ListImportBatches`, `GetImportBatch` | `resetFixtures()` is deleted, and its callers in tests move to a router transport, the way `dedup.test.ts` already does |
| `report.ts` | `GetManagementPNL`, `ListLineTransactions` | The reconciliation strip's closing figure stays derived, never read from a field |
| `review.ts` | `ListReviewGroups`, `ListGroupTransactions`, `ResolveGroup`, `UndoDecision`, `ListCategories` | `listCategories` stops being a fixture |

The view interfaces these modules export do not change shape. That is the whole point of
the seam, and a diff that changes a component is a sign the seam was crossed.

`TopBar` takes its organisation and entity names from `GetCurrentUser` instead of
labels — `add-web-experience` tasks 9.1 and 9.2, pulled in here because this change
supplies the data they were waiting for. Nothing else from that change's eleven open
tasks is pulled in.

## 5. Rejected alternatives

1. **React context plus a `useOrg()` hook in each screen.** Rejected: it breaks
   `add-web-experience` design D1 and the test that enforces it, and it edits every
   screen to deliver a value none of them displays.
2. **Widen `categories.scope` to `'template'` and copy within the one table.** Rejected:
   every existing taxonomy query becomes responsible for excluding a row kind, and one
   miss puts a template row on a customer's P&L line. A separate table cannot be read by
   accident.
3. **Auto-create the organisation inside `resolveOrProvision`.** Rejected: it writes
   tenant rows on the login path, which is the path the schema was shaped to keep
   writes off, and the customer gets a name taken from a Google profile.
4. **An operator CLI subcommand.** Rejected: cheapest to build and untestable through the
   browser, so the Demo's own acceptance path would stay unexercised until the Demo.
5. **A SQL seed fixture, like the pilot import profiles.** Rejected for the same reason,
   plus it leaves the product with no way to onboard anybody at Product.
6. **Adopt the template lazily, on first classification.** Rejected: two organisations
   created on the same day would hold different taxonomies depending on when each first
   imported a file.
7. **Keep `report.ts` on fixtures behind a flag, to de-risk the Demo.** Rejected: a flag
   that can show invented numbers to a customer is a flag that eventually does.

## 6. Open questions

Resolved during design review, 2026-09-19:

- **D-11 (Vekst or Palm).** Already decided, and already shipped: "Veekst" (double `e`,
  `veekst.com`), per `add-web-experience/proposal.md` and live today in `web/index.html`
  and `web/src/i18n.ts`. The first-run screen uses the same message-catalogue entry as
  every other screen — no new copy decision. `docs/IMPLEMENTATION_PLAN.md`'s D-11 row is
  a doc-hygiene gap, not an open decision: it should be marked Closed, citing
  `add-web-experience`.
- **Base-currency validation.** No longer Go-only against a three-pair allowlist. §2.2
  adds `currencies`, seeded from `core/internal/money`'s full exponent map.
  `organizations.base_currency` and `transactions.currency`/`base_currency` keep their
  existing `CHECK`-only validation unchanged — upgrading those to a foreign key is a
  separate, later change.

Still open:

- **The end-to-end test needs a running classifier.** `make ci`'s Go job has no Python
  service. Task 7.3 resolves it by asserting on the classification *run outcome* rather
  than on a category, with the full-stack assertion behind the existing live-database
  job — a decision to confirm in review.
