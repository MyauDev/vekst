# Vekst — Architecture (v3)

Date: 2026-08-20 · Team: 2 developers
Supersedes the single-service architecture in `IMPLEMENTATION_PLAN.md` v1.
Decision owner: you (technical). Product shape: the Palm spec (see `SPEC-RECONCILIATION.md`).

---

## 1. Decisions taken

| # | Decision | Owner |
| --- | --- | --- |
| A-1 | Two backend services: `core` in Go, `classifier` in Python. ~~The split ships in the Commercial milestone~~ — **superseded 2026-08-23: the split shipped in change 0.1.** See 2.0 | You |
| A-2 | `core/classify` holds the `Classifier` **interface** with exactly the shapes the proto carries. Its implementation is a gRPC client, not the engine | You, 08-15; revised 08-23 |
| A-3 | Browser talks to `core` over ConnectRPC. `core` will talk to `classifier` over native gRPC | Me, from A-1 |
| A-4 | **The classifier has no database access**, in either form. It is a pure function over its inputs | Me — see 3.3 |
| A-5 | Deduplication is its own module and its own capability spec | You |
| A-6 | Product shape follows the Palm spec: wizard, RP packages, payment wall, chat, consultant, Paddle, dunning | You |
| A-7 | AI classification is layer L4, in the Intelligence milestone | Me — see 4.4 |
| A-8 | An `entity` level exists in the schema from the first migration | Me, from Palm Q4/Q5 |

---

## 2. Why the split waits, and what that changes

> **Superseded, 2026-08-23.** Section 2 argued for deferring the Python split to
> the Commercial milestone. The founder reversed that: the classifier shipped as
> a separate Python service in change 0.1. The reasoning below is kept because it
> still records what the deferral would have bought and what the split costs —
> §2.2's list of costs is now a description of the present, not a forecast.
>
> What actually changed: Track B writes engine layers L0–L2 in Python rather than
> Go, two runtimes and two lint/test pipelines exist from day one, and §2.3's five
> rules stopped being a discipline that can be violated by accident — a separate
> process cannot reach a database handle it was never given.
>
> `IMPLEMENTATION_PLAN.md` §5's `extract-classifier-service-python` (3 days,
> Commercial) is therefore obsolete. See
> `openspec/changes/archive/*-bootstrap-monorepo/design.md` D2 for the decision
> record, including the cost accepted.

### 2.1 The reasoning

Three facts decide the timing:

1. Demo and Product run a **rules-only** engine — L0 vendor memory, L0.5 account codes,
   L1 org rules, L2 industry seed. None of that needs Python.
2. **L3 fuzzy matching does not need Python either.** Postgres `pg_trgm` does trigram
   similarity inside a query, and the database is already there.
3. Only **L4** — embeddings plus an LLM — genuinely belongs in Python.

Cost to build the split now: 3 days in the tightest milestone, plus a boundary tax on
every change that crosses it. Cost to port later: about 1.5 days, because the interface
already has the right shape.

### 2.2 What that costs when it arrives

A two-language backend for one developer is not free. From the Commercial milestone you
carry two runtimes, two dependency managers (`go mod` and `uv`), two lint and test
pipelines, a network boundary that used to be a function call, a versioned contract that
can drift, and two containers to ship.

It is still the right destination, for one reason: the classification engine is the only
part of this system that will grow an embedding index, a model client and an evaluation
harness. Those live in Python or they live nowhere.

What it does **not** buy you: scale, performance, or independent deployment. You do not
need any of those at five customers. Do not let the split justify further splitting.

### 2.3 The rule that makes the port cheap

**Write the engine as if it already ran in another process.** Concretely, from day one:

- One Go package, one entry point: `Classify(ctx, ClassifyBatchRequest) (ClassifyBatchResponse, error)`.
- The request carries everything: taxonomy, vendor memory, org rules, account-code maps,
  transactions, threshold. The engine reads **no** global state.
- The engine holds **no** database handle, no HTTP client, no clock. Pass the time in.
- Same request in, same response out. Table-driven fixture tests, no database in them.
- The request and response types are generated from the proto that the future Python
  service will implement — so the wire format exists before the wire does.

If any of those five is violated, the port stops being mechanical. That is the only thing
that makes this decision reversible.

---

## 3. Service boundary

```
                    browser (React + Vite + TS)
                              │  ConnectRPC over HTTP/2
                              ▼
   ┌──────────────────────────────────────────────────┐
   │  core  (Go)                                       │
   │  auth · tenancy · RLS · wizard & RP scoring       │
   │  ingest orchestration · parsing · dedup           │
   │  transactions · rules CRUD · review queue         │
   │  chat orchestration · report computation          │
   │  export · Paddle billing · dunning jobs (River)   │
   └───────┬───────────────────────────────┬───────────┘
           │ native gRPC (private network)  │
           ▼                                ▼
   ┌──────────────────────┐        ┌────────────────────┐
   │ classifier (Python)  │        │ Postgres 16        │
   │ FastAPI + grpcio     │        │ RLS forced         │
   │ stateless            │        │ core connects only │
   │ NO database access   │        └────────────────────┘
   └──────────────────────┘
                                   ┌────────────────────┐
                                   │ S3-compatible store│
                                   └────────────────────┘
```

### 3.1 What `core` owns

Everything with state, everything with a tenant, everything a user can see. If it writes
to Postgres or to object storage, it is `core`.

### 3.2 What `classifier` owns

One thing: given transactions and the tenant's classification context, return proposals.

```protobuf
// Named ClassifierService, not Classifier: buf's STANDARD lint set requires the
// suffix, and HealthService already carries it. Renamed in change 0.1, before
// any client existed, because renaming afterwards is a breaking change.
service ClassifierService {
  rpc ClassifyBatch(ClassifyBatchRequest) returns (ClassifyBatchResponse);
  rpc Version(VersionRequest) returns (VersionResponse);
}

message ClassifyBatchRequest {
  string  request_id            = 1;   // idempotency key
  Taxonomy taxonomy             = 2;   // categories + taxonomy_version
  repeated VendorMemory vendors = 3;   // this org's L0 memory
  repeated Rule rules           = 4;   // this org's L1 rules
  repeated AccountCodeMap codes = 5;   // L0.5, regulated chart of accounts
  repeated TxnForClassify txns  = 6;
  double  threshold             = 7;
  bool    allow_model_layer     = 8;   // false until L4 ships
}

message ClassifyBatchResponse {
  string ruleset_version              = 1;  // recorded on every classification row
  string engine_version               = 2;  // the classifier build
  repeated Proposal proposals         = 3;  // category_id, confidence, engine_layer, evidence
}
```

### 3.3 The rule that matters: no database in the classifier

The classifier never receives database credentials. `core` reads the tenant's context
under RLS and passes it in the request.

Three reasons, in order of importance:

1. **Isolation stays single-mechanism.** RLS in Postgres is the only thing standing
   between two customers' data. One process holds the connection. A bug in the Python
   service cannot cross a tenant boundary, because it never had the ability to.
2. **Reproducibility.** The same request produces the same response. That is what makes a
   report re-printable, and it is what makes the engine testable against fixtures.
3. **It stays deletable.** If the split turns out to be wrong, a stateless service is a
   library you re-host. A service with its own tables is a migration.

If you ever find yourself wanting to give the classifier a database, that is the signal
that the boundary was drawn in the wrong place. Move the logic, not the credentials.

### 3.4 Contract management

- One `buf` workspace in `/proto` generates Go, TypeScript and Python stubs.
- `buf lint` and `buf breaking` run in CI against the main branch. A breaking change to
  the classifier contract fails the build.
- The classifier reports `engine_version`. `core` stores it on every classification row.
  A report pins `taxonomy_version` + `ruleset_version` + `engine_version`. Those three
  strings are what make a March report reproduce in June.

### 3.5 Failure behaviour

| Failure | `core` does |
| --- | --- |
| Classifier unreachable | The River job retries with backoff. The batch stays in `classifying`. The user sees "still working" |
| Classifier returns an error | The batch fails atomically. No partial classifications are written |
| Classifier is slow | Batches are chunked at 5,000 transactions. Timeout 60 s per chunk |
| Classifier returns an unknown `category_id` | `core` rejects the whole response. Never trust the other side's ids |

---

## 4. The classification engine

### 4.1 Layers

| Layer | Method | Source | Confidence |
| --- | --- | --- | --- |
| **L0** | Exact match on the org's vendor key memory | any | 1.00 |
| **L0.5** | Regulated chart-of-accounts code lookup | ledger only (1C) | 0.99 |
| **L1** | Org rule match (contains, regex, amount range, direction, account) | any | 0.95 |
| **L2** | Seed rules for the industry profile from the wizard | any | 0.80 |
| **L3** | Trigram / fuzzy match against the org's own approved history, via Postgres `pg_trgm` | any | 0.50–0.79 |
| **L4** | Embeddings narrow to 3–5 candidates, then an LLM picks one with structured output | any | model-reported |
| — | No match | — | 0.00 |

Milestones: L0, L0.5, L1 and L2 ship in **Demo** (Go). L3 ships in **Product** (Go, via
`pg_trgm`). L4 ships in **Intelligence** (Python), after the service is extracted in
**Commercial**.

L0.5 comes from the Palm spec and is the strongest layer for 1C-sourced data. It applies
**only** to ledger rows. Never apply an account-code rule to a bank row.

L4 is the Palm AI design, unchanged in method. Only its position in the schedule moved.

### 4.2 Learning loop

An approval in the review queue or in the chat writes a vendor-memory row for that
`counterparty_key`. Next month the same vendor is caught by L0 at zero cost. This is what
makes month 2 take three minutes instead of fifteen.

### 4.3 Escalation (from Palm)

Adopted as specified: count the clarifying questions asked in the chat and the fill rate
of the report template. After 3 questions, if the template is under 80 percent complete,
the system offers a consultant. It does not wait for the user to give up.

### 4.4 Why L4 is not in the first milestones

Not a disagreement with the method — a disagreement with the order.

- Without a rules-only baseline there is no number to compare a model against. You cannot
  answer "did the model help" if nothing preceded it.
- An LLM call per unknown line has a cost per transaction that never falls. A vendor
  memory row gets cheaper every month. Ship the layer whose cost curve goes down first,
  measure the residue, then buy the model only for what is left.
- The Python service exists from day one anyway, so adding L4 later is a new code path in
  an existing service, not a new service.

The product does not suffer: the customer experience Palm describes — mapping feels
automatic, gaps are asked about in chat, a consultant appears when it stalls — is
delivered by L0 to L3 plus the Wizard-of-Oz consultant. The customer cannot tell which
layer answered.

---

## 4a. The ingest pipeline

```
documents → ① PARSE → ② VALIDATE → ③ PERSIST → ④ CLASSIFY → ⑤ REPORT
```

### 4a.1 Stage ② — validation

Validation sits between parsing and persistence, and it is **blocking**. A file that fails
persists nothing: the import is atomic. This is the difference between numbers that look
right and numbers you can prove are complete.

Three outcomes: `valid` · `valid_with_warnings` · `rejected`.

**Correctness — per row, blocking**

| Check | Fails when |
| --- | --- |
| Date parses and is plausible | Outside [today − 10 years, today + 1 day] |
| Amount parses to `int64` minor units | Non-numeric, or the number locale was guessed wrong |
| Currency is a valid ISO-4217 code | Unknown or empty |
| Debit and credit are mutually exclusive | Both populated on one row |
| No U+FFFD replacement characters | Charset detection was wrong — a silent data corruption otherwise |
| Description present after trimming | Empty |
| Account identifier resolves | Unknown and not creatable |

**Completeness — per file**

| Check | Why |
| --- | --- |
| **opening balance + Σ movements = closing balance** | The strongest check available. Most bank statements declare both. Zero tolerance. If it passes, no row was lost, truncated or mis-signed |
| Declared row count matches the parsed count | 1C exports often declare one |
| The declared statement period is covered continuously | Catches a file that starts mid-period |
| One currency per account within the file | Catches a mis-mapped currency column |
| No duplicate bank references inside the file | Catches a double-appended export |

A completeness failure may be overridden by an `approver`, with a written reason, recorded
and shown on any report computed from that batch. A **correctness** failure may not be
overridden.

The error report is keyed by **line number in the original file**, not by parsed row
index. A user who has to find row 2,847 of a CSV in Excel will not use your product twice.

### 4a.2 Failure surface

| Failure | Behaviour |
| --- | --- |
| Any correctness error | Reject the batch. Persist nothing. Return the error list |
| A completeness warning | Persist, mark the batch, surface the warning on every report from it |
| Parse succeeded but validation crashed | Treat as rejected. Never persist a partially validated batch |

---

## 5. Deduplication and matching (A-5)

Its own capability: `dedup-and-matching`. Four levels, each with a different action.

| Level | Detects | Key | Action |
| --- | --- | --- | --- |
| **D1** | The same file uploaded twice | SHA-256 of the file bytes | Reject the upload, report "already imported" |
| **D2** | Repeated rows inside one file | `dedup_hash` | Skip, count, show the count |
| **D3** | Rows already imported from another file | `dedup_hash` lookup across batches | Skip, count, show which batch holds the original |
| **D4** | The same economic event in two sources — a ledger invoice and its bank payment | amount + counterparty + date window | **Propose a link. Never delete.** Human confirms |

```
dedup_hash = sha256(
    org_id, entity_id, account_id, booked_on,
    amount_minor, currency,
    normalize(description), bank_ref, document_ref, posting_no
)
```

### 5.0 The canonical grain

`transactions` holds **one row per payment or posting**.

- A bank payment → one row. `document_ref` is null, `posting_no` is 0.
- A ledger document with five postings → five rows sharing one `document_ref`, with
  `posting_no` 1…5.

This satisfies "one line per payment" without throwing away ledger detail, and it is what
makes D4 possible: a bank payment can link to the postings of the invoice it settles.

Rejected alternative: one row per *document*, with line items in JSONB. It reads well and
it makes every report query a JSONB unnest. Do not.

`normalize()` lowercases, collapses whitespace, strips punctuation, and strips the
bank's own reference noise. Its exact behaviour is versioned, because changing it changes
what counts as a duplicate.

### 5.1 D4 is the one that decides whether the numbers are right

`core` must know the **source kind** of every transaction: `ledger` or `bank`.

- A report line is computed from **one** source kind. Never from both.
- Where both exist, the org picks which is authoritative for that report. The other side
  becomes reconciliation evidence, shown in the drill-down.
- D4 proposes links between the two. A confirmed link marks the pair as one event.

Without this, the Palm "single financial model" merges an invoice and its payment and
counts the revenue twice. This is the single most likely way for this product to print a
wrong number.

### 5.2 Internal transfers

Separate from D4 and equally necessary. Find pairs within the same org: opposite signs,
equal absolute amount, within 3 days, different accounts. Propose them as internal
transfers. They are excluded from every P&L line.

### 5.3 What the user sees

> 412 rows imported · 88 duplicates skipped · 6 internal transfers found ·
> 12 possible matches to your accounting data — review

---

## 5.5 Core tables (first cut)

```
organizations(id, name, country, base_currency, created_at, deleted_at)
entities(id, org_id, name, legal_name, tax_id, base_currency)        -- one per org in Demo
users(id, email, name, locale, created_at)
memberships(org_id, user_id, role)                                   -- owner|admin|approver|viewer
accounts(id, org_id, entity_id, name, currency, external_ref)

import_batches(id, org_id, entity_id, uploaded_by, source_kind,      -- ledger | bank
               status, file_key, file_sha256, row_count)
import_profiles(id, org_id, name, source_kind, column_map,
               charset, delimiter, decimal_sep, date_fmt)
raw_rows(id, org_id, batch_id, line_no, payload_jsonb)
import_validations(id, org_id, batch_id, outcome,                    -- valid|warnings|rejected
               error_count, warning_count, balance_check_passed,
               report_jsonb, overridden_by, override_reason, at)

transactions(id, org_id, entity_id, account_id, source_kind,
             document_ref, posting_no,                               -- the grain, see 5.0
             booked_on, value_on, direction,
             amount_minor, currency, fx_rate, fx_rate_on, base_amount_minor,
             counterparty_raw, counterparty_key, description_raw,
             bank_ref, dedup_hash, batch_id)
transaction_links(id, org_id, ledger_txn_id, bank_txn_id,            -- D4
             confidence, confirmed_by, confirmed_at)

vendors(id, org_id, key, display_name, default_category_id)          -- L0 memory
categories(id, taxonomy_version, code, parent_id, name_i18n, pnl_section, is_pnl)
account_code_maps(id, taxonomy_version, chart, account_code, category_id)  -- L0.5
classification_rules(id, org_id, priority, matcher_jsonb, category_id, active)
classifications(id, org_id, transaction_id, category_id, confidence,
             engine_layer, ruleset_version, engine_version,
             decided_by, decided_at, superseded_by)                  -- APPEND ONLY
review_items(id, org_id, transaction_id, state, resolved_by, resolved_at)
report_runs(id, org_id, entity_id, kind, params_jsonb, taxonomy_version,
             ruleset_version, engine_version, status, result_key, created_at)
audit_events(id, org_id, actor_id, action, target, payload_jsonb, at)  -- APPEND ONLY
```

Every table above carries `org_id`, an RLS policy, and `FORCE ROW LEVEL SECURITY`.
`users` is the only exception — it is global, and access to it is mediated by
`memberships`.

---

## 6. Money, currency and reproducibility

Unchanged from v1, and now enforced across a language boundary:

- Go: `int64` minor units + ISO-4217 code. Never `float64`.
- Python: `int` minor units, or `decimal.Decimal`. **`float` and raw pandas numeric
  columns are forbidden for money.** Add a test that fails when a money field is a float.
- Protobuf: a `Money` message, never a `double`.
- TypeScript: money crosses the wire as a string. Never a JS `number`.
- Multi-currency: store the original amount, the FX rate, the rate date and the
  base-currency amount. Rate source: ECB daily reference rates, cached.
- `classifications` is append-only. A correction inserts a new row and supersedes the old
  one. Each row carries `engine_layer`, `ruleset_version` and `engine_version`.

---

## 7. Tenancy

- One Postgres database, shared schema.
- **Three levels**: `organizations` → `entities` → `accounts`. Every tenant table carries
  `org_id`; entity-scoped tables also carry `entity_id`.
- v1 creates exactly one entity per organisation. The column is not optional — it is
  populated from day one, so a holding customer needs no migration.
- RLS on every tenant table, plus `ALTER TABLE ... FORCE ROW LEVEL SECURITY`.
- Two database roles: `vekst_migrator` owns the schema; `vekst_app` owns nothing, has no
  `BYPASSRLS`, and holds only DML rights.
- Tenant context is set with `SET LOCAL app.org_id` inside the request transaction. A
  repository call outside that transaction must fail.
- A CI test lists tenant tables without an RLS policy and fails the build.

---

## 8. Repository layout

```
/proto            buf workspace — the single source of truth for every language
  /vekst/v1       public API (browser ↔ core)
  /vekst/internal/v1  classifier contract — defined now, implemented in Go now,
                      re-implemented in Python at the Commercial milestone
/core             Go: chi, connect-go, pgx, sqlc, goose, River
  /core/classify  the engine. No DB handle, no clock, no globals. See 2.3
/classifier       Python (from Commercial): grpcio + FastAPI, uv, ruff, pytest
/web              React 19 + Vite + TypeScript + Tailwind v4
/site             Astro static marketing site (from Commercial)
/deploy           Docker Compose, Caddy, backup scripts
/docs             these documents
```

CI (GitHub Actions), one workflow with parallel jobs:

`buf lint` · `buf breaking` · codegen drift · `go vet` + `go test` + `govulncheck` ·
`ruff` + `mypy` + `pytest` · `vitest` + `tsc` · `gitleaks` · manifest validation ·
SonarQube · build images.

The Python jobs joined in change 0.1 rather than at Commercial, because the
classifier service exists from day one (A-1). `gitleaks` runs as the MIT CLI, not
the Action wrapper, which is licensed for organisation-owned repositories.

---

## 9. Rejected alternatives

| Rejected | Why |
| --- | --- |
| Everything in Python | Fine technically. Rejected because you are faster in Go, and you are the only developer. Speed of the one developer outranks language purity |
| Everything in Go | The engine will need embeddings and a model client. Both are painful in Go and native in Python |
| The classifier reads the database directly | Breaks single-mechanism tenant isolation and breaks reproducibility. See 3.3 |
| HTTP/JSON between the services | No generated types, no breaking-change detection, hand-written serialisation for money |
| gRPC from the browser to `core` | A browser cannot speak native gRPC. Connect solves it without a proxy |
| A message queue between the services | Adds a broker for a request that is already inside a River job with retries |
| Separate databases per service | Two sources of truth for one financial model |
