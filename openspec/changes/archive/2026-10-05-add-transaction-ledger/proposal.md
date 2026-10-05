## Why

Everything downstream is waiting on one table. Change 3.2 shipped an engine that answers
`ClassifyBatch` correctly and stores nothing, because its own non-goals say so: there is
nowhere to put a classification when there is no transaction to classify. The review queue
(3.3) has no queue, the P&L (4.1) has nothing to sum, and `ClassifyBatch` is reachable only
from a test.

This is also the table the product's worst failure mode lives in. A report line computed
from a mix of ledger and bank rows counts an invoice and its payment twice, so
`source_kind` is not a label on a row — it is what a report is allowed to group by. And the
grain decides whether ledger detail survives: one row per payment or posting, never one row
per document with line items in JSONB.

Milestone: **Demo**. Capability: **`transaction-ledger`** (new).

## What Changes

- **Migration 007 creates `transactions`** at the grain ARCHITECTURE.md §5.0 fixes: a bank
  payment is one row with a null `document_ref` and `posting_no` 0; a ledger document with
  five postings is five rows sharing one `document_ref`, numbered 1…5.
- **Migration 007 creates `classifications`**, append-only, each row carrying
  `engine_layer`, `ruleset_version` and `engine_version` — the three strings that make a
  March report reproduce in June. A correction inserts a new row and supersedes the old one;
  nothing is ever updated in place except the `superseded_by` pointer.
- **Multi-currency is stored, not derived.** The original amount, the FX rate, the rate date
  and the base-currency amount, all as `int64` minor units plus an ISO-4217 code. A report
  that has to re-derive a conversion is a report that can disagree with itself.
- **The three columns change 3.2 asked for**: `description_norm`, `normalize_version` and
  `regulated_code`. `core/internal/normalize` decides what counts as a match, so the version
  travels with every value it produced — changing the function is a backfill of every row
  carrying the old string, never a silent reinterpretation.
- **`import_batches` arrives here, minimally.** See the assumption below.
- **`core/internal/ledger`** — the typed queries: insert a batch of rows, read a page of
  them for classification, write classifications back, read the current classification of a
  transaction.

## An assumption this change makes, and why

`transactions.batch_id` points at `import_batches`, which the plan puts in change 2.1
(`add-file-upload`, Track A, not started). Three ways to handle that, and this change takes
the third:

1. *Wait for 2.1.* Leaves Track B blocked on a change nobody has started, which is the
   situation this change exists to end.
2. *Leave `batch_id` out.* A ledger row that cannot say which import produced it is not a
   ledger row. Validation is keyed by line number in the original file, and the user-facing
   count — "412 rows imported · 88 duplicates skipped" — is per batch. Provenance is not a
   nice-to-have here.
3. **Create `import_batches` with the columns `transactions` needs, and no more.** `id`,
   `org_id`, `entity_id`, `source_kind`, `status`, `created_at`. Change 2.1 adds `file_key`,
   `file_sha256`, `row_count` and `uploaded_by` when there is an upload to record them from,
   and owns the state machine that gives `status` its meaning.

The invariant forces most of this: a foreign key between tenant tables is composite, so the
parent has to exist before the child can point at it. What this change deliberately does not
take from 2.1 is everything that makes uploading a file work — signed URLs, MIME and size
limits, the endpoint, the state machine's transitions. Those are still Track A's, and this
change adds no handler and no route.

## Non-goals

- **No ingest.** Nothing writes a transaction from a file yet. The parser in
  `core/internal/ingest` produces rows; wiring it to this table is change 2.2/2.3, after
  validation exists to say a batch may be persisted at all.
- **No classification run.** The worker that reads transactions, calls `ClassifyBatch` and
  writes the answers is its own change — see below.
- **No dedup.** `dedup_hash` is a stored column with a unique index and nothing computing
  it. D1/D2/D3 are change 2.6.
- **No `transaction_links`.** D4 — matching a bank payment to the postings it settles — is
  its own change, and the one most likely to print a wrong number if rushed.
- **No FX rate fetching.** The columns exist and the job that fills them from ECB is 2.5's
  successor. A row whose rate is null is a row in the organisation's base currency.
- **No review items.** 3.3.

## A gap this change surfaces

The plan has no change that owns the River worker which reads transactions, calls
`ClassifyBatch`, and writes `classifications` rows. It is implied between 2.5 and 3.3 and
written down nowhere: `core/internal/jobs` holds `NoopWorker` and `TenantProbeWorker` and
nothing else. This change creates the table the worker would write to and the queries it
would use, and stops there. Proposing that change is the natural next step after this one.

## Impact

Touches `/core/migrations`, `/core/internal/db` (**both reviewers**), `/core/internal/ledger`
(new), and `/deploy/db/rls-exempt-tables.txt` only if something turns out to need it — it
should not. Apply after 3.2. No proto changes, so `buf breaking` has nothing to say.
