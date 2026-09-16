# Vekst — Implementation Plan v4

Date: **2026-08-20**
Supersedes v3. Product shape: the **Palm v0.3** founder spec. Technical shape: this
document and `docs/ARCHITECTURE.md`.
Team: **2 developers**.

---

## 0. What changed in v4

| # | Change | Effect |
| --- | --- | --- |
| 1 | **Two developers, not one** | Capacity roughly 1.6×, not 2×. See section 2 |
| 2 | **An explicit validation stage** between parse and persist — correctness and completeness | New capability `ingest-validation`, +2.5 days. Worth every hour |
| 3 | **The canonical grain is fixed**: one row per payment or posting | See `ARCHITECTURE.md` section 5 |
| 4 | Dates corrected. v1–v3 were dated 15 August; today is 20 August | 5 working days already gone |

### 0.1 Milestones

| Name | Date | What it means |
| --- | --- | --- |
| **Demo** | 2026-10-01 | Each pilot customer uploads their own files and sees their own Management P&L, with every figure openable to the transaction. You may still write their column mapping by hand |
| **Product** | 2026-10-23 | Fully self-serve. Mapping screen, roles, XLSX export, D4 matching, L3 fuzzy. **Invoice by hand from here — you do not need a payment processor to take money** |
| **Commercial** | 2026-11-27 | Wizard, chat, consultant queue, Paddle, dunning, three more reports, deviation highlighting, PDF, German, marketing site, Python classifier split |
| **Intelligence** | 2027-Q1 | The L4 model layer, Xero and QuickBooks connectors, mobile, remaining templates |

---

## 1. The pipeline, as decided

```
documents in
   │
   ▼
① PARSE        charset · delimiter · header row · number locale · date format
   │
   ▼
② VALIDATE     correctness (per row) + completeness (per file)
   │           → valid · valid_with_warnings · rejected
   │           A rejection persists nothing. The import is atomic
   ▼
③ PERSIST      canonical rows — ONE ROW PER PAYMENT OR POSTING
   │           dedup D1/D2/D3 · internal-transfer detection
   ▼
④ CLASSIFY     L0 vendor · L0.5 account code · L1 org rules · L2 seed
   │           below 0.80 → review queue
   ▼
⑤ REPORT       Management P&L · drill-down
```

Stage ② is new in v4 and it is the most valuable thing added since the first draft.
Without it the product produces numbers that *look* right. With it the product produces
numbers it can *prove* are complete.

### 1.1 What stage ② checks

**Correctness — per row, blocking:**

| Check | Fails when |
| --- | --- |
| Date parses and is plausible | Outside [today − 10 years, today + 1 day] |
| Amount parses to `int64` minor units | Non-numeric, or the locale was guessed wrong |
| Currency is a valid ISO-4217 code | Unknown or empty |
| Debit and credit are mutually exclusive | Both **non-zero** on one row — corrected 2026-09-13 (change 2.2 design D5): in every real Priorbank export both columns are populated on every row, one of them `0,00`, so "both populated" as originally written rejects every row of every file. Change 2.3's spec was drafted from the earlier, wrong wording and should be checked against this one |
| No U+FFFD replacement characters | The charset detection was wrong |
| Description is present | Empty after trimming |
| The account identifier resolves | Unknown account and not creatable |

**Completeness — per file, blocking unless the customer overrides:**

| Check | Why it matters |
| --- | --- |
| **Balance reconciliation: opening + Σ movements = closing** | The strongest check that exists. Most bank statements declare both balances. If it passes, no row was lost, silently truncated or mis-signed. Tolerance is zero |
| Declared row count matches parsed row count | 1C exports often declare a count |
| The declared statement period is covered continuously | Catches a file that starts mid-period |
| One currency per account within the file | Catches a mis-mapped currency column |
| No duplicate bank references inside the file | Catches a double-appended export |

**What the user sees:**

> **Validation failed.** 3,182 rows read · 3,179 valid · **3 errors** ·
> Balance check: opening 412,300.00 + movements −18,442.15 ≠ closing 393,860.00
> (difference 2.15). Nothing was imported. [Download the error list]

Line numbers in the error list refer to the original file, not to the parsed rows.

### 1.2 The grain, fixed

`transactions` holds **one row per payment or posting**.

- A bank payment → one row, `document_ref` null.
- A ledger document with five postings → five rows sharing one `document_ref`.

This satisfies "one line per payment" and keeps ledger detail. It also makes D4
(ledger↔bank matching) possible, because a payment can link to the postings of the
invoice it settles.

---

## 2. Two developers — the honest arithmetic

**Two developers are not twice one developer.** [Certain, and standard] Expect about
**1.6×** in the first month. The loss goes to: agreeing on the schema, review latency,
migration collisions, and the fact that the first few days of a greenfield repo do not
parallelise — one person builds the skeleton while the other has little to stand on.

Capacity to 1 October:

| | |
| --- | --- |
| Working days, 24 August → 1 October | 29 |
| Two developers, raw | 58 person-days |
| × 0.8 for real-world days (email, thinking, one bad day) | 46 |
| − foundation days that do not parallelise | −5 |
| **Usable** | **≈ 41 person-days** |

The Demo scope below is **37 person-days**. It fits, with 4 days of slack. That slack is
the whole margin — do not spend it in advance.

> **Stale, 2026-09-02.** The line above no longer holds. §3's own note records 0.1 and 0.2
> at **≈13 person-days actual against 6 budgeted** here — a **7-day overrun** on the first
> two of eighteen changes, before either track has written a line of ingest or
> classification code. If every remaining change tracks its own budget exactly (the
> optimistic case — 0.1 and 0.2 did not), total actual cost is 37 + 7 = **44 person-days**
> against the 41 usable computed above: a **3-day shortfall**, not 4 days of slack, and
> before the unbudgeted provisioning change §3 also names. This table needs a real
> re-baseline, not a footnote; recomputing it is not this task's job, only flagging that
> the number above is now wrong.

### 2.1 The work split

Two tracks, one seam. The seam is the `transactions` table.

**Days 1–2, both developers together:** the `transactions` schema, the `Money` type, the
tenancy model, and the repo skeleton. Agree these once, in the same room. After that, do
not both edit the same file.

| | **Track A — Ingest** | **Track B — Meaning** |
| --- | --- | --- |
| Owns | `/core/ingest`, `/core/dedup` | `/core/classify`, `/core/report` |
| Builds | upload, parse, validate, persist, dedup, internal transfers | taxonomy, engine L0–L2, review queue, P&L, drill-down |
| Blocked by | **the 7 real export files** | **the category list** |
| Front-end | Imports screen, validation report | Review queue, P&L, drill-down |

Coordination rules — few, and non-negotiable:

- **One migration owner per week.** Two people writing migrations in the same week will
  collide, and Postgres will not tell you politely.
- Small pull requests, no long-lived branches. A branch older than two days is a merge
  problem.
- Both review any change to the schema, the proto, or anything touching money.
- A 10-minute daily sync, on the seam only. Not a status meeting.

### 2.2 The risk that got worse

With one developer, a missing input blocked one person. With two, **each missing input
now blocks a whole track.**

- No real export files → Track A stalls after day 3.
- No category list → Track B stalls after day 6.

**Both inputs are due by 27 August.** If either slips, that track's developer works on the
other track's queue, and the tracks stop being parallel — which costs you the 1.6× and
puts you back at one developer's pace with two salaries.

---

## 3. Demo — 1 October

**Definition of done:** each pilot customer uploads their own files, sees a validation
result they can trust, and reads their own Management P&L in a browser, with every figure
openable to the transaction behind it.

**Definition of not-done:** a screenshot, a spreadsheet built by hand, or a demo on
invented data. If the pipeline did not compute it, it does not count.

| # | Change | Capability | Track | Days |
| --- | --- | --- | --- | --- |
| 0.1 | `bootstrap-monorepo` — Go core, **Python classifier**, buf/Connect, Vite+React+Tailwind, **k3d + Tilt**, CI | `platform-foundation` | both | ~~4~~ **9** |
| 0.2 | `add-postgres-and-migrations` — goose, sqlc, two DB roles, **River**, `Money` + no-float tests | `platform-foundation` | both | ~~2~~ **3.75** |
| 1.1 | `add-tenancy-and-rls` — organizations → entities → accounts, `SET LOCAL app.org_id`, forced RLS, CI policy check, cross-tenant tests | `tenancy` | B | 3 |
| 1.2 | `add-identity` — Google OIDC only | `identity-access` | B | 1.5 |
| 2.1 | `add-file-upload` — signed-URL upload, size and MIME limits, batch state machine, `source_kind` | `file-ingestion` | A | 2 |
| 2.2 | `add-statement-parsing` — charset, delimiter, header row, number locale, dates, XLSX, raw rows as JSONB | `file-ingestion` | A | 3 |
| 2.3 | **`add-ingest-validation`** — the checks in 1.1, three outcome states, the error report, atomic rejection | `ingest-validation` | A | 2.5 |
| 2.4 | `add-import-profiles` — the profile model and its application. No mapping UI yet | `file-ingestion` | A | 1 |
| 2.5 | `add-transaction-ledger` — canonical rows, one per payment or posting, `document_ref`, money, currency, `counterparty_key`. **Change 3.2 adds three columns to this change's scope**, because the classifier is sent normalised text and never normalises any itself: `description_norm`, `normalize_version` and `regulated_code`. `normalize_version` is not decoration — `core/internal/normalize` decides what counts as a match, so changing it is a backfill of every row carrying the old value. **Delivered 2026-09-14**, migration 007: `transactions` and `classifications`, plus `import_batches` in the minimal shape this change needs to point at — the upload state machine and its own columns are 2.1's delivery, not repeated here. `core/internal/ledger` is the typed seam: `Transaction`/`Classification`, `DedupHash`, `Insert`, `ToClassifyBatch`. **Corrected during implementation**: `classifications`' append-only enforcement is a deferred constraint trigger, not the plain unique index the migration first shipped with — a plain one cannot express "insert the new live row, then point the old one at it" without a race. Still explicitly out of scope here, unchanged from the proposal: FX rate fetching, dedup_hash computation (2.6), and the classification worker that reads `UnclassifiedTransactions` (proposed by 5.4 below) | `transaction-ledger` | A | 2 |
| 2.6 | `add-dedup` — D1 file hash, D2 in-batch, D3 cross-batch, internal-transfer pairs | `dedup-and-matching` | A | 1.5 |
| 3.1 | `add-classification-taxonomy` — category tree, `is_pnl`, `pnl_section`, non-P&L classes, versioned. **Delivered 2026-09-08**, migration 005: 41 shared nodes + 5 computed lines, split read/write policies, a constraint trigger where a composite key cannot reach. The 60 per-organisation leaves are an industry template, not seeded — a shared row belongs to nobody and these belong to whoever adopts them | `classification-taxonomy` | B | 1.5 |
| 3.2 | `add-classification-engine` — the `Classifier` interface plus L0, L0.5 and L1. **Delivered 2026-09-13**, migration 006: 71 template rules that belong to a country or a bank, vendor memory that belongs to one organisation, `ClassifyBatch` on the internal contract, normalisation ported to Go with a conformance fixture, and the engine in Python replaying `eval/engine.py` exactly. L2 is not here: it was listed with L0–L1 when the plan was written, and it is a separate change | `classification-engine` | B | 2.5 planned, ~3.5 actual |
| 3.4 | `add-classification-run` — the River job that calls the engine with the ledger's unclassified rows, chunked, retried and recorded. **Delivered 2026-09-16**, migration 016: `classification_runs`, one row per batch, deliberately not one more state on `import_batches` — widening that CHECK a fourth time would make every future Track A change reason about Track B's states too. **Three things were wrong in code this change only had to call**: `UnclassifiedTransactions` paged by relying on rows leaving the result as they were classified, which loops forever on the first below-threshold row; it was not scoped to a batch, so two concurrent imports would classify each other's rows; and it did not know about `retracted_at`, so a row whose only answer had been withdrawn was invisible to the worker while every report counted it as unanswered. Enqueued inside the transaction that lands a batch on `imported`, because a job inserted after the commit is one a crash in between loses | `classification-run` | B | 2 |
| 3.3 | `add-review-queue` — below-threshold items by amount, grouped by counterparty, keyboard-first, approval writes vendor memory. **Delivered 2026-09-15**, migration 013: `review_decisions`, one row per *counterparty* and not per transaction, replacing the `review_items` sketch — a per-transaction `state` is a second representation of a fact `classifications` already holds, and the drift is silent in the worst direction (a row marked resolved with no classification is missing from the queue and the report at once). The queue itself is therefore not a table: it is `NOT EXISTS (a live classification)`. **Two things were found by writing the tests rather than by inspection**: migration 007 needed a `retracted_at`/`retracted_by` pair, because an undo has no successor to point at and supersession cannot express "withdrawn, and nothing replaced it"; and a concurrent double-resolve reached the client as an internal error until the unique-index violation was given a code of its own. Role enforcement is read from a real membership, never from a string a caller passed | `review-queue` | B | 3 planned, ~3.5 actual |
| 4.1 | `add-management-pnl` — sections, periods, totals, percent of revenue, non-P&L exclusions, basis label from `source_kind`. **Delivered 2026-09-15**, migration 014: `categories.pnl_section` filled with the level-1 **code** (not the name — a name is unique by nothing and a rename would move every row out of its section silently), plus a `BEFORE` trigger that fills it where a caller omits it and refuses it where a caller states a different one. `core/internal/report` splits in two and the split is enforced: `pnl.go` is the calculation, a pure function over rows, and a test parses its imports so a database, a clock or an environment cannot reach it; `service.go` is the read. **Three things were settled rather than implemented as written**: the sign convention and the generated `direction` column (section 0, and not 4.1's work by rights — the report cannot be written while `sum(amount_minor)` over a cost section has no defined sign); D-3, the output table structure, which turned out to be a real change to `Compute` rather than a document edit; and the pinned versions, which are **sets** and not single strings — a report summing rows classified under two engine versions was produced under two, and naming one is a claim about reproducibility that is not true | `report-mgmt-pnl` | B | 2 planned, ~3 actual |
| 4.2 | `add-report-drilldown` — any figure opens its transactions with category, layer and confidence. **Delivered 2026-09-15**, migration 015: `transactions.line_no`, which this change's budget note said would cost half a day extra if it needed its own migration — it did, because 007 merged first. A cell is addressed by the four things that computed it (entity, basis, period, line) and never by a handle the server has to keep or sign. `core/internal/db/query/drilldown.sql` holds **one** line predicate, not one per bucket, written in the order `Compute` places a row; `report.SelectionFor` is what both the figure and its drill-down derive dates from, which is what stops a quarterly report that starts in February from opening January. A computed line answers with its operands rather than inventing a transaction set. **The reconciliation strip is a real comparison and not a summary**: the three movements come from one aggregation and the two balances from another, so a row dropped or double-counted breaks the identity — and it is labelled derived, because nothing stores what the statement itself declared until 2.3's balance check | `report-mgmt-pnl` | B | 1.5 planned, ~2 actual |
| 5.1a | `add-web-app-shell` — semantic token layer per `docs/DESIGN.md`, shell, nav, entity and period selectors, the state components. Light mode only | `web-app` | A | 1.5 |
| 5.1b | `add-i18n-and-error-catalog` — the i18n mechanism, en + ru. **The catalog is blocked by 2.3**: it translates error codes that do not exist yet | `web-app` | A | 1.5 |
| 5.2 | `add-demo-screens` — Imports (with the validation report), Review, P&L | `web-app` | A+B | 2 |

**Total as planned: 37 person-days.** Track A ≈ 17.5 · Track B ≈ 17 · shared ≈ 2.5.

> **Revised, 2026-09-06.** 5.1 is split into 5.1a and 5.1b and costs 3 days, not 2.
> `web/package.json` has no i18n library and no ECharts, and the invariant that the
> backend returns error codes puts every sentence in the client. 5.1a is blocked by
> nothing. 5.1b's catalog is blocked by 2.3. The design direction and the token set
> are in `docs/DESIGN.md`; the total above is not re-derived here.

> **Stale, 2026-08-23.** Changes 0.1 and 0.2 measured **≈13 person-days against the
> 6 budgeted here**, before either track wrote ingest or classification code. Three
> founder decisions taken after this table was written account for it: the Demo is
> hosted, the development environment is local Kubernetes, and the Python
> classifier ships from day one instead of at Commercial.
>
> The Demo also needs a change that provisions the cluster, DNS, TLS and backups.
> **No milestone budgets that work at all.**
>
> §2's capacity arithmetic predates all four and no longer holds. Re-derive it
> before treating 1 October as a commitment.

> **Retroactive, 2026-09-13.** 2.2 `add-statement-parsing` shipped inside PR #4 — the
> classification-taxonomy pull request — with no change proposed for it at the time.
> `openspec/changes/add-statement-parsing/proposal.md` is written from the code rather than
> the code from the proposal, and says so at its head. Nothing in this row needed correcting
> as a result; the code already carries `raw rows as JSONB` as an open task (4.1) rather than
> a delivered one.

### 3.1 Deliberately excluded from the Demo

| Excluded | Returns in | Why it is safe |
| --- | --- | --- |
| The wizard and RP scoring | Commercial | You already know what these two customers need |
| The column-mapping screen | Product | You write two import profiles by hand |
| Roles and permissions | Product | You create both users yourself |
| D4 ledger↔bank matching | Product | Give each customer one source kind only. Do not mix |
| Rules CRUD | Product | You seed their rules in SQL |
| L3 fuzzy matching | Product | L0–L2 plus the review queue covers it |
| XLSX and PDF export | Product / Commercial | They read it on screen |
| Dark mode, German | Product / Commercial | Cosmetic |
| Paddle, chat, consultant queue, dunning | Commercial | No money changes hands yet |

### 3.2 What must never be cut

| Never cut | Reason |
| --- | --- |
| Tenancy and forced RLS | Retro-fitting isolation is a migration of every table, and it is the one mistake that leaks one customer's finances to another |
| The `entity` level | Same reason. One column now, every table later |
| `int64` minor units for money | Rounding errors no customer forgives |
| Append-only classifications with versions | Without them a report cannot be re-printed, and an accountant will ask |
| **Balance reconciliation in stage ②** | It is the only proof you have that no row was lost |
| Internal-transfer detection | Without it, money between the customer's own accounts is counted as revenue |
| Drill-down | An accountant does not trust a figure they cannot open |

---

## 4. Product — 23 October

About 15 person-days ≈ 9 working days with two developers.

`add-column-mapping-ui` (3, A) · `add-roles-and-permissions` (1, B) ·
`add-classification-rules-crud` (1, B) · `add-l3-fuzzy-pg-trgm` (1, B) ·
`add-d4-ledger-bank-matching` (2.5, A) · `add-report-engine-templates` (2, B) ·
`add-xlsx-export` (1, B) · `add-dark-mode` (0.5, A) · `add-magic-link-auth` (1, B) ·
`add-validation-overrides` (1, A — let a customer accept a warning with a reason, logged) ·
`add-import-history` (1, A)

From here the product works without you. Invoice the first customers by hand. The first
five invoices teach you the price better than any pricing page.

---

## 5. Commercial — 27 November

About 30 person-days ≈ 19 working days with two developers.

`add-onboarding-wizard` (2) · `add-report-package-model` (1.5) · `add-assisted-chat` (5) ·
`add-consultant-requests` (1.5) · `add-billing-paddle` (4) · `add-subscription-addons` (1.5) ·
`add-dunning-lifecycle` (3) · `add-data-erasure` (1) · `add-report-sales` (2) ·
`add-report-opex` (2) · `add-report-cashflow` (2) · `add-deviation-highlighting` (2) ·
`add-audit-log` (1) · `add-pdf-export` (1.5) · `add-german-locale` (1) ·
`add-marketing-site` (3) · ~~`extract-classifier-service-python` (3)~~ — **done in
change 0.1**; the service has existed since the skeleton. See `ARCHITECTURE.md` §2

Carried from the Palm spec, unchanged: dunning (pause, keep data, 3 reminders, delete at
3 months, `scheduled_deletion_at` written once at pause and never recomputed); the
consultant at $30 per report with no hourly tracking; deviation highlighting with
deterministic thresholds and no AI text.

---

## 6. Intelligence — Q1 2027

`add-classification-model-layer` (L4: embeddings narrow to 3–5 candidates, then an LLM
picks one with structured output) · `add-xero-oauth` · `add-quickbooks-oauth` ·
remaining report templates · mobile client · AI comments on already-highlighted deviations.

---

## 7. Open decisions

| ID | Decision | Blocks | Due |
| --- | --- | --- | --- |
| D-1 | The classification category list | 3.1, 3.2, 4.1 | **Closed 2026-09-08.** 101 categories in `eval/out/taxonomy.csv`; the 46 shared ones are seeded by migration 005 and the other 60 are an industry template awaiting the change that creates an organisation. Built from the founder's P&L structure and the three categorisation files, measured against 4,508 real transactions; see `eval/README.md`. Track B is unblocked |
| D-2 | The 7 real export files in `/core/testdata` | 2.2, 2.3 | **Partly closed 2026-09-08.** Four redacted Priorbank fixtures are in `core/testdata`, both column layouts, with the balance check passing on all four. Kazakh and Polish exports are parsed by `eval/sources.py` but have no Go parser yet. Redacted rather than real: the originals name people and carry tax identifiers |
| D-3 | The predefined output table structure | 4.1 | **Closed 2026-09-15, change 4.1 task 0.5.** Rows are sections and computed lines *interleaved*, in the order 01 · 02 · GM · 03 · NM · 04 · 05 · CM · 06 · IBT · 07 · NI — each computed line immediately after the operands it consumes, because a table listing seven sections and then five results is a spreadsheet the reader has to reassemble mentally, and the reassembly is where a misreading happens. Columns are one per period plus Total, every cell an amount in the organisation's base currency with its share of revenue where there is revenue to take a share of. Below the table, in order: unclassified, excluded non-P&L, unallocated, other basis. Costs print positive. `report.Order` and `report.BucketOrder` are that decision in executable form, and three tests hold the table to it — one on the order itself, two asserting that nothing the report computes goes unprinted and nothing printed goes uncomputed |
| D-4 | Does the Demo split VAT out of gross? | 3.1, 4.1 | **Closed 2026-09-15, change 4.1, on the default.** No: the Demo reports gross and labels it. Nothing in the taxonomy separates a net amount from its tax — `05010305` VAT is an expense leaf like any other, which is the shape a gross report wants — and a bank statement carries no VAT breakdown to split by. Splitting would mean either a rate per category, which nobody has been asked for, or a guess printed as a figure. The change that has a customer asking for net reporting adds the tax columns to `transactions` and the rule for which report reads them |
| D-5 | FX rate source | 2.5 | Default: ECB daily reference rates, cached, rate on the booking date |
| D-6 | Hosting target — **the Demo is confirmed hosted; the host is still unnamed** | The provisioning change, and now `add-file-upload` (change 2.1) | Working assumption: Hetzner Cloud (EU) with k3s, matching the local k3d environment. Docker Compose and Caddy are superseded by Kubernetes and an Ingress. **2026-09-13: this now blocks a merged change, not only a future one.** 2.1's presigned-upload design runs against MinIO in the local overlay and nowhere else; `deploy/k8s/base` carries no object-store host because D-6 names none, so `CreateImportBatch` answers a configuration error in every environment but a laptop until this is answered |
| D-7 | Legal entity country | Commercial | Any Paddle-supported country. Ukraine and Kazakhstan qualify; Belarus does not |
| D-8 | May a customer override a completeness warning, and who signs it off? | 2.3, and `add-validation-overrides` | Default: `approver` may override, with a written reason, recorded on the report. **Closed 2026-09-13, change 2.3 task 0.2:** implemented as owner, admin or approver (not the literal string "approver" alone -- a role able to run the organisation is at least as trusted as one whose only job is approving) may override, gated on a written reason of at least ten characters. Pulled forward from a later change into 2.3 itself; the two hours it cost were paid for by dropping `add-import-profiles`' (2.4) profile-name-uniqueness UI affordance, per this document's own rule that pulling scope forward means naming what leaves in exchange |
| D-9 | Cross-client shared vendor memory: yes with consent, or never | The terms of service | Before the first invoice. Migration 006 takes the conservative side in the meantime: `vendors` is an ordinary tenant table with no shared rows, so sharing later is a schema change somebody has to make deliberately rather than a policy somebody could relax. **Unchanged by change 3.3 (2026-09-15)**, which is the change that first writes vendor memory in anger: every row it writes carries the deciding organisation's `org_id` and the `key_version` of the normalisation that produced the key, so the question stays exactly as open as it was — and a later yes has both the tenant boundary and the key provenance it would need |
| D-10 | Auto-accept confidence threshold | 3.2, 3.3 | **Closed 2026-09-13.** 0.80, and it is the engine's own default when a request leaves the field unset — proto3 cannot tell an unset double from a deliberate 0.0, so the ambiguity resolves to the stricter reading. The three layers sit at 1.00 (vendor memory), 0.99 (regulated code) and 0.95 (text rule), so today the threshold admits all three; the gaps are what will order a review queue and what a later layer will have to clear |
| D-11 | Product name: Vekst or Palm | Anything public | Before the marketing site |
| D-12 | Is L0.5 ledger-only? | 3.2 | **Closed 2026-09-13, against the original decision.** `ARCHITECTURE.md` §4.1 restricted the account-code layer to ledger rows on the reasoning that a bank row carries no account code. Kazakh bank rows carry a КНП, and 16 code/direction pairs classify 948 of 1,294 rows with no exception; PKO BP labels every row with its own operation type. The rule that replaces it is about who assigned the code, not where it came from: a code assigned by somebody other than the payer is stronger evidence than the payer's own free text. §4.1 is amended and migration 006's rules carry one `regulated_code` field for all of them, never one field per country |

---

## 8. How to run this with OpenSpec

1. `openspec/config.yaml` is filled and current.
2. Work the Demo table in track order. Each row is one `/opsx:propose <change-name>`,
   pasting that row and its capability as the description.
3. Review `proposal.md`, `design.md` and `tasks.md` before writing code.
4. `/opsx:apply`, then `/opsx:archive` when merged.

Repo rules:

- One change is one capability delta. More than two capabilities means split it.
- Every change touching a tenant table adds a cross-tenant isolation scenario.
- Every change touching money adds a non-base-currency scenario and a no-float test.
- Every change that imports data adds a validation scenario, including a rejected file.
- One migration owner per week.
- Write the classification engine as if it already ran in another process: inputs in,
  outputs out, no hidden state, no clock, no globals.

---

## 9. Sources

- Paddle — supported and unsupported seller countries —
  https://www.paddle.com/help/start/intro-to-paddle/which-countries-are-supported-by-paddle
- Stripe — global availability — https://stripe.com/global
