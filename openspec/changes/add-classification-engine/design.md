## Context

`eval/` holds a working engine, 71 template rules and a harness that measures both. The
numbers below are from `eval/run_eval.py` over the founder's real statements, 2023–2024:

| | Rules | Rows | Amount | Accuracy | Disagreements |
| --- | --- | --- | --- | --- | --- |
| Belarus, Priorbank | 41 | 87.5% | 85.2% | 94.8% | 15 |
| Kazakhstan | 21 | 86.1% | 99.5% | 92.4% | 4 |
| Poland, PKO BP | 9 | 38.6% | 42.5% | 99.6% | 1 |

None of those rules mentions the customer they were measured on. That is the property worth
preserving through this change, and §D1 is about the one column that makes it storable.

## The data model

```sql
CREATE TABLE classification_rules (
    id          uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    taxonomy_version text   NOT NULL,
    ruleset_version  text   NOT NULL,

    -- NULL = a template rule: it belongs to a country and a bank, not to a
    -- customer, and every organisation uses it. 87% of coverage comes from
    -- these. A schema where every rule has an owner cannot express them.
    org_id      uuid        NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    scope       text        NOT NULL,   -- 'country:BY' | 'bank:priorbank' | 'org' | ...

    priority    integer     NOT NULL,
    matcher     jsonb       NOT NULL,
    category_id uuid        NOT NULL REFERENCES categories (id) ON DELETE RESTRICT,
    active      boolean     NOT NULL DEFAULT true,
    created_at  timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT rules_scope_matches_owner CHECK (
        (org_id IS NULL AND scope <> 'org') OR (org_id IS NOT NULL AND scope = 'org'))
);

-- Priority is unique within what a single organisation sees: its own rules and
-- the shared ones. Ties would make the engine's answer depend on row order.
CREATE UNIQUE INDEX rules_priority_idx
    ON classification_rules (taxonomy_version, ruleset_version, org_id, priority)
    NULLS NOT DISTINCT;

CREATE TABLE vendors (
    id          uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      uuid        NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,

    -- Produced by core's counterparty_key(): 'tax:220340017991' or
    -- 'name:ДЖОНДОРИ'. The version is stored because changing the function
    -- changes what counts as the same counterparty, and that is a backfill.
    key         text        NOT NULL,
    key_version text        NOT NULL,
    display_name text       NOT NULL,
    category_id uuid        NOT NULL REFERENCES categories (id) ON DELETE RESTRICT,

    decided_by  uuid        NULL REFERENCES users (id) ON DELETE RESTRICT,
    decided_at  timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT vendors_key_unique UNIQUE (org_id, key_version, key)
);
```

### Row-level security

```sql
ALTER TABLE classification_rules ENABLE ROW LEVEL SECURITY;
ALTER TABLE classification_rules FORCE ROW LEVEL SECURITY;

CREATE POLICY rules_read ON classification_rules
    FOR SELECT USING (org_id IS NULL OR org_id = app_current_org());
CREATE POLICY rules_write ON classification_rules
    FOR ALL USING (org_id = app_current_org())
             WITH CHECK (org_id = app_current_org());

ALTER TABLE vendors ENABLE ROW LEVEL SECURITY;
ALTER TABLE vendors FORCE ROW LEVEL SECURITY;

-- Ordinary tenant table: memory is earned by one organisation's decisions and
-- is shared with nobody. Cross-client memory is BACKLOG B-6 and a consent
-- question, not a schema one.
CREATE POLICY vendors_tenant ON vendors
    FOR ALL USING (org_id = app_current_org())
             WITH CHECK (org_id = app_current_org());
```

`classification_rules` repeats the shared/tenant shape 3.1 introduced for `categories`,
including the seed-before-`FORCE` ordering and the split read/write policies. The reasons are
identical and are argued there; if 3.1's §D2 is not agreed, this change inherits the problem.

## D1 — Why `org_id` is nullable here

Measured, not assumed: template rules produce 87.5% of Belarusian coverage and 86.1% of
Kazakh, and not one of them names the customer. Two ways to store them were rejected.

**Rejected: copy the templates into each organisation at signup.** Then every rule has an
owner and the policy is ordinary. It also means a corrected rule reaches only customers
created afterwards, and 71 rows are duplicated per tenant for no gain. The Belarusian VAT
rule is not 40 different rules.

**Rejected: keep templates in a file and merge them in the application.** The engine's input
would then come from two places, only one of which is under RLS, and "which rules did this
March report use" stops being answerable from the database.

## D2 — Normalisation lives in `core`, and Track A must know

The engine matches `description_norm` and `counterparty_key`. It does not compute them.

That is not an aesthetic choice. `ARCHITECTURE.md` §5.5 already stores `counterparty_key` on
`transactions`, and §5's `dedup_hash` already depends on `normalize(description)`. Both are
ingest concerns and both land before any classification runs. Computing them again in the
classifier would be a second implementation of a function whose output defines what counts as
a duplicate.

**What change 2.5 (`add-transaction-ledger`, Track A) must add**, and should hear now rather
than after its migration is written:

- `transactions.description_norm text NOT NULL` — the normalised payment purpose.
- `transactions.normalize_version text NOT NULL` — because changing the function changes
  which rules fire and which rows are duplicates. A change to it is a backfill of the column,
  never a silent reinterpretation.
- `transactions.regulated_code text NULL` — КНП in Kazakhstan, `Typ operacji` in Poland, the
  1C account number in a ledger export. One column, because L0.5 is "the source carries a
  code somebody else assigned", not "Kazakhstan".

The Go implementation is a port of `eval/norm.py`. A conformance test runs both over the
committed fixtures and fails on any difference, so the port cannot drift from the reference
the harness measures.

## D3 — L0.5 applies to bank rows, contradicting `ARCHITECTURE.md` §4.1

§4.1 restricts the account-code layer to ledger rows: *"Never apply an account-code rule to a
bank row."* That was written for 1C, where the code is on a posting.

It does not hold for this market. Every Kazakh **bank** row carries КНП, a state-assigned
payment-purpose code, and sixteen КНП/direction pairs classify 948 rows with no exception at
all — 86.1% of rows and 99.5% of amount from 21 rules. Poland's `Typ operacji` is the weaker
version of the same thing: chosen by the bank rather than the regulator, but exact.

The rule that survives is the one §4.1 was reaching for: **a code assigned by someone other
than the payer is stronger evidence than the payer's own free text.** Whether it arrives on a
bank row or a ledger row is not what makes it trustworthy. `ARCHITECTURE.md` §4.1 is amended
by this change; the restriction is replaced by a requirement that the code's issuer be
recorded in `scope`.

## The contract

Browser-facing? No. This is `core` → `classifier` over native gRPC, on a ClusterIP address,
never routed through the Ingress. **`buf breaking` runs, and `/proto` needs both reviewers.**

```protobuf
service ClassifierService {
  rpc Version(VersionRequest) returns (VersionResponse);
  rpc ClassifyBatch(ClassifyBatchRequest) returns (ClassifyBatchResponse);
}

message ClassifyBatchRequest {
  string request_id        = 1;  // idempotency key; core stores it per chunk
  string taxonomy_version  = 2;
  string ruleset_version   = 3;
  // Asserted, not applied: core normalised these values and says with what.
  // The classifier rejects a request whose version it does not implement,
  // rather than matching normalised text against unnormalised rules.
  string normalize_version = 4;

  repeated Category     categories = 5;  // classifiable leaves only, never computed
  repeated Rule         rules      = 6;  // already ordered by priority
  repeated VendorMemory vendors    = 7;  // this organisation's L0 memory
  repeated TxnForClassify txns     = 8;
  double threshold                 = 9;  // D-10 default 0.80

  reserved 20 to 39;  // L4: candidates, embedding_model_version. See ARCHITECTURE 2.3
}

message TxnForClassify {
  string transaction_id    = 1;
  string source_kind       = 2;  // ledger | bank
  string description_norm  = 3;
  string counterparty_key  = 4;
  string direction         = 5;  // debit | credit
  vekst.type.v1.Money amount = 6;
  string regulated_code    = 7;  // КНП, Typ operacji, 1C account. Empty when none
  string account_id        = 8;
}

message Rule {
  int32  priority      = 1;
  string category_code = 2;
  string scope         = 3;
  repeated Condition all = 4;    // AND only. OR is a second rule, so each rule
}                                // stays independently measurable

message Condition {
  string field = 1;  // description | counterparty_key | regulated_code | direction | amount | account
  string op    = 2;  // contains_all | eq | gte | lte
  string value = 3;
  vekst.type.v1.Money amount_value = 4;  // never a double
}

message Proposal {
  string transaction_id = 1;
  string category_code  = 2;
  string engine_layer   = 3;  // L0 | L0.5 | L1
  double confidence     = 4;
  string evidence       = 5;  // the key tier, or the rule's scope
  int32  matched_rule_priority = 6;
}

message ClassifyBatchResponse {
  string engine_version   = 1;
  string ruleset_version  = 2;
  repeated Proposal proposals = 3;  // absent transaction_id = no layer answered
}
```

`contains_all` is an AND over `;`-separated parts. That semantics was not guessed: 40 of the
46 multi-part rules in the source files have transactions matching every part, and the six
exceptions are rules whose second half no longer appears in any statement.

## What this touches from the invariants list

- **Money** — `Condition.amount_value` and `TxnForClassify.amount` are `Money`, never a
  `double`. An amount-range rule that stored a float would round a threshold.
- **The classifier contract** — directly. `buf breaking`, both reviewers.
- **Tenant isolation** — two new tables, one of them with the shared/tenant shape from 3.1.
- **`source_kind`** — carried per transaction so a ledger-only rule can never fire on a bank
  row, which is what the old §4.1 restriction was protecting.
- **Append-only classifications** — not yet. Nothing is written; see Non-goals.

## Rejected alternatives

| Rejected | Why |
| --- | --- |
| Normalise inside the classifier | A second implementation of the function that defines what counts as a duplicate. §D2 |
| Nested boolean matcher (`any`/`not`) | OR becomes a second rule instead. Keeps each rule independently measurable by the harness, keeps priority meaningful, and keeps the future editing screen a form rather than a query builder |
| Copy template rules per organisation | A corrected rule reaches only customers created afterwards. §D1 |
| Keep `ClassifyBatch` out and call the engine in-process | Reverses design D2 of change 0.1, which the founder took deliberately. The service exists; this is what it is for |
| A separate КНП table instead of `regulated_code` | Three countries would become three tables for one idea: a code somebody else assigned |
| Let the classifier read `vendors` itself | It has no database credentials, and never will. ARCHITECTURE A-4 |

## Risks

| Risk | Mitigation |
| --- | --- |
| The Go port of `normalize` drifts from the Python reference | A conformance test over the committed fixtures fails on any difference |
| A retry re-runs a chunk and the caller double-writes later | `request_id` per chunk, stored, with the write guarded by it when 2.5 lands |
| A rule targets a section or a computed line | The request carries only classifiable leaves; the classifier rejects a `category_code` it was not given |
| Priority ties make the answer depend on row order | Unique index on `(taxonomy_version, ruleset_version, org_id, priority)` |
| 71 rules is small enough to feel finished | The harness reports seven rules that never fire and a residue of 54, 31 and 63 counterparties per country. It is not finished, and the report says so on every run |
