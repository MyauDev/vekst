## Context

Three tables, one of which is the product's centre of gravity. `transactions` is where a
wrong decision prints a wrong number six months later rather than failing a test today, so
most of what follows is about constraints that make a wrong row impossible to store rather
than about what a right row looks like.

What already exists and constrains this change:

| | |
| --- | --- |
| `accounts (org_id, id, entity_id, currency, …)` | migration 004, composite primary key |
| `categories`, `classification_rules`, `vendors` | migrations 005 and 006 |
| `money.Money` = `int64` minor units + ISO-4217 | `core/internal/money`, no table uses it yet |
| `normalize.Version` | `core/internal/normalize`, and this is the first table to store it |
| `ClassifyBatch` | `vekst.internal.v1`, answered by the classifier, persisted nowhere |

## The data model

```sql
CREATE TABLE import_batches (
    org_id      uuid        NOT NULL,
    id          uuid        NOT NULL DEFAULT gen_random_uuid(),
    entity_id   uuid        NOT NULL,

    -- ledger | bank. The single most load-bearing column in the schema: a
    -- report line is computed from one of these and never from both.
    source_kind text        NOT NULL CHECK (source_kind IN ('ledger', 'bank')),

    -- Change 2.1 owns the state machine. Stored now because transactions must
    -- be able to say which import produced them, and a batch with no state at
    -- all cannot express "persisted" -- which is the only state this change
    -- creates rows in.
    status      text        NOT NULL DEFAULT 'persisted'
                            CHECK (status IN ('persisted')),
    created_at  timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (org_id, id),
    FOREIGN KEY (org_id, entity_id) REFERENCES entities (org_id, id) ON DELETE RESTRICT
);

CREATE TABLE transactions (
    org_id       uuid        NOT NULL,
    id           uuid        NOT NULL DEFAULT gen_random_uuid(),
    entity_id    uuid        NOT NULL,
    account_id   uuid        NOT NULL,
    batch_id     uuid        NOT NULL,

    -- Denormalised from the batch on purpose: every report query filters on
    -- it, and a join to discover which half of the business a row belongs to
    -- is a join somebody eventually forgets. A trigger keeps it honest.
    source_kind  text        NOT NULL CHECK (source_kind IN ('ledger', 'bank')),

    -- The grain, ARCHITECTURE.md §5.0. A bank payment: document_ref NULL,
    -- posting_no 0. A ledger document with five postings: five rows sharing
    -- one document_ref, posting_no 1..5.
    document_ref text,
    posting_no   integer     NOT NULL DEFAULT 0 CHECK (posting_no >= 0),

    booked_on    date        NOT NULL,
    value_on     date,
    direction    text        NOT NULL CHECK (direction IN ('income', 'expense')),

    -- Money, five columns of it. See D2.
    amount_minor      bigint NOT NULL,
    currency          text   NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    fx_rate           numeric(20, 10),
    fx_rate_on        date,
    base_amount_minor bigint,
    base_currency     text   CHECK (base_currency ~ '^[A-Z]{3}$'),

    counterparty_raw  text   NOT NULL DEFAULT '',
    counterparty_key  text   NOT NULL DEFAULT '',
    description_raw   text   NOT NULL DEFAULT '',
    description_norm  text   NOT NULL DEFAULT '',

    -- What produced description_norm and counterparty_key. See D3.
    normalize_version text   NOT NULL,

    -- КНП, Typ operacji, a 1C account code. Empty when the source carried
    -- none. This is what makes L0.5 reachable.
    regulated_code    text   NOT NULL DEFAULT '',

    bank_ref     text        NOT NULL DEFAULT '',
    dedup_hash   text        NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (org_id, id),
    FOREIGN KEY (org_id, entity_id)  REFERENCES entities (org_id, id)       ON DELETE RESTRICT,
    FOREIGN KEY (org_id, account_id) REFERENCES accounts (org_id, id)       ON DELETE RESTRICT,
    FOREIGN KEY (org_id, batch_id)   REFERENCES import_batches (org_id, id) ON DELETE RESTRICT,

    CONSTRAINT txn_grain CHECK (
        (document_ref IS NULL AND posting_no = 0)
        OR (document_ref IS NOT NULL AND posting_no > 0)),

    CONSTRAINT txn_fx_is_all_or_nothing CHECK (
        num_nonnulls(fx_rate, fx_rate_on, base_amount_minor, base_currency) IN (0, 4)),

    CONSTRAINT txn_fx_is_a_conversion CHECK (
        base_currency IS NULL OR base_currency <> currency),

    CONSTRAINT txn_fx_rate_is_positive CHECK (fx_rate IS NULL OR fx_rate > 0)
);

CREATE TABLE classifications (
    org_id          uuid        NOT NULL,
    id              uuid        NOT NULL DEFAULT gen_random_uuid(),
    transaction_id  uuid        NOT NULL,
    category_id     uuid        NOT NULL,

    engine_layer    text        NOT NULL CHECK (engine_layer IN ('L0', 'L0.5', 'L1', 'L2', 'human')),
    confidence      numeric(4, 3) NOT NULL CHECK (confidence >= 0 AND confidence <= 1),
    evidence        text        NOT NULL DEFAULT '',

    -- The three strings a report pins, plus the one that says how the text it
    -- matched on was produced. All four, on every row, because a report is
    -- reproducible only if every input to it is recorded beside the output.
    taxonomy_version  text      NOT NULL,
    ruleset_version   text      NOT NULL,
    engine_version    text      NOT NULL,
    normalize_version text      NOT NULL,

    decided_by      uuid        NULL,
    decided_at      timestamptz NOT NULL DEFAULT now(),
    superseded_by   uuid        NULL,

    PRIMARY KEY (org_id, id),
    FOREIGN KEY (org_id, transaction_id) REFERENCES transactions (org_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (org_id, superseded_by)  REFERENCES classifications (org_id, id) ON DELETE RESTRICT,

    CONSTRAINT cls_human_has_a_decider CHECK (
        engine_layer <> 'human' OR decided_by IS NOT NULL)
);

-- At most one live classification per transaction. The partial index is what
-- makes "the current answer" a fact rather than a query convention.
CREATE UNIQUE INDEX classifications_one_live_idx
    ON classifications (org_id, transaction_id)
    WHERE superseded_by IS NULL;
```

`category_id` is deliberately **not** a foreign key: `categories` is shared-and-tenant, so a
classification may point at a row with a NULL `org_id`, which no composite key can express.
The same constraint trigger migration 006 uses for rules and vendors applies here, and for
the same reason — it also refuses a section or a computed line, which is what keeps an
amount from being counted twice.

## Decisions

### D1 — `source_kind` is denormalised onto `transactions`, and a trigger enforces it

Every report query filters on it, and §5.1 says a line is computed from one kind. Reaching
through `batch_id` to find out which half of the business a row belongs to is a join that
somebody eventually writes without, and the failure is silent: a P&L that quietly mixes
invoices and payments. So it is stored on the row.

The cost of denormalising is that the two copies can disagree, so a constraint trigger
asserts the row's `source_kind` equals its batch's. Not a composite foreign key including
`source_kind` — that would work, but it makes every insert carry a column it already has and
turns a typo into a foreign-key error that names the wrong problem.

**Alternative rejected:** derive it in the query with a join. Correct until the first query
that forgets, and no test can catch a join that was never written.

### D2 — Multi-currency is six columns, and all-or-nothing

`amount_minor` + `currency` is what the bank said. `fx_rate` + `fx_rate_on` +
`base_amount_minor` + `base_currency` is the conversion, recorded rather than recomputed. A
report that re-derives a conversion at read time can disagree with the one shown yesterday,
because the rate table moved.

The all-or-nothing CHECK is the part that matters. Four nullable columns admit fifteen
partial states, of which one is meaningful — a row with a rate and no converted amount is a
row a report will either skip or convert itself, and both are wrong quietly. Either the
conversion happened and all four are present, or it did not and all four are null, meaning
the row is already in the organisation's base currency.

`numeric(20,10)` for the rate, not a float: a rate is not money but it multiplies money, and
`0.1 + 0.2` is as untrue here as anywhere. Go reads it as a string and the conversion is
done in integer arithmetic in `core/internal/money`.

**Alternative rejected:** store only the original amount and convert on read. Cheaper to
write and it makes a report a function of when it was run.

**What this cost, and the rule it had to buy.** `base_currency` is the second column in the
schema naming the currency a report converts into, and migration 004 made "exactly one home"
a *tested* invariant — `TestTheReportingCurrencyHasOneHome` failed on the first run of this
migration, which is the test working. The escape hatch its message offers is a rule saying
which copy applies, so this change adds one: a constraint trigger asserting a row's
`base_currency` is the organisation's at the time of writing.

The rule is deliberately not "these are always equal". A row already stored carries the
currency its `base_amount_minor` is actually denominated in, which is the right value for
that row rather than a disagreement. So an organisation changing its reporting currency
becomes a visible backfill plus a re-conversion, instead of a silent reinterpretation of
every amount ever converted. The test is now stricter than it was: a second carrier with no
such trigger still fails.

Dropping the column instead would leave `base_amount_minor` as an integer with no code
beside it, which is the one thing money is never allowed to be.

### D3 — `normalize_version` is on the row, not in a config table

The classifier matches a rule's stored text against `description_norm` exactly, and refuses
a request whose `normalize_version` it does not implement. That refusal is only worth
anything if the version travelling in the request is the version that produced the value —
which means reading it from the row, not from a setting that may have changed since the row
was written.

The consequence is deliberate and is the reason it is a column: changing
`core/internal/normalize` is a backfill. Rows carrying `v1` keep matching `v1` rules until
something rewrites them, and a mixed table is a legible state rather than a corrupt one.

### D4 — `classifications` is append-only, enforced rather than documented

`vekst_app` holds `INSERT` and `SELECT` on the table, plus `UPDATE` on `superseded_by` alone
via a column grant. A correction inserts the new row and points the old one at it. There is
no `DELETE` grant.

The partial unique index on `(org_id, transaction_id) WHERE superseded_by IS NULL` is what
makes "the current classification" a single row by construction. Without it, "current" is a
convention every query has to re-implement, and the first query that gets it wrong produces
a report with a transaction counted twice.

**Alternative rejected:** a `current` boolean. Two writers can both set it, and nothing
notices until a report double-counts.

### D5 — `dedup_hash` is NOT NULL with a unique index from the start, and it counts occurrences

Change 2.6 owns D1/D2/D3 deduplication. The column and its index land here anyway, because
adding a unique index to a table that already holds duplicates is not a migration — it is an
incident with a data-cleaning exercise attached.

The input set is the row's own content: `account_id`, `booked_on`, `amount_minor`,
`currency`, `description_norm`, `bank_ref`, `document_ref`, `posting_no` — **plus the
occurrence index of that content within its batch.**

That last term is not a refinement, it is the difference between working and losing money.
A hash over content alone makes two genuinely distinct payments collide: a company that buys
coffee twice on the same day, same amount, same wording, no bank reference, has two real
rows that a unique index would silently reduce to one. The bank statement shows two lines
because there were two payments, and a report that shows one is wrong by the amount of a
coffee — and by a great deal more the first time it happens to a payroll run.

With the occurrence term both cases come out right:

| | |
| --- | --- |
| Two identical payments in one statement | occurrences 1 and 2, different hashes, both stored |
| The same file imported twice | the same occurrences, the same hashes, the second rejected |

So the index still catches the duplicate that matters — a file re-imported, which is D1 and
D3 — and stops catching the one that was never a duplicate. 2.6 may widen the input set; what
it must not have to do is introduce uniqueness retroactively.

**Alternative rejected:** no unique index, detect duplicates in application code. It works
until two imports race, and the failure is a doubled revenue line rather than an error.

### D6 — `entity_id` is on the row, even though v1 creates one entity per organisation

Same reasoning migration 004 used, and the same reason it is not optional: a holding customer
needs no migration, only rows. The alternative — add it when somebody asks — means asking
every existing customer which entity each of their transactions belonged to, retroactively,
which is a question they cannot answer.

### D7 — The transaction id crossing to the classifier is the database's own

`ClassifyBatch` takes `transaction_id` as a string and returns it on the proposal. It is the
`transactions.id` UUID rendered as text. This is safe specifically because the classifier
holds no database handle: an id it cannot use to read anything is not a capability, it is a
correlation token. Passing a surrogate index instead would mean the worker maintaining a
mapping for the lifetime of a batch, which is a bug with no upside.

## Risks

| Risk | Mitigation |
| --- | --- |
| The FX columns land with nothing filling them, and stay empty until somebody notices a report is wrong | The all-or-nothing CHECK makes "not converted" a stated fact, and a non-base-currency row with no conversion is visible as such rather than silently summed |
| `import_batches` grows the columns 2.1 wants and the two changes disagree | This change creates only what `transactions` needs to point at, and 2.1's own columns are additive to it. The `status` CHECK admits one value, so 2.1 widening it is a visible edit |
| `dedup_hash` computed here differs from what 2.6 decides | The hash is versioned by nothing and that is intentional: 2.6 recomputes it for every row if it changes the input set, which is a backfill of one column and not a schema change |
| A classification written against a superseded taxonomy version | All four versions are on the classification row; a report pins them and can tell |
