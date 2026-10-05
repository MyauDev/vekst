## Why

Track A has written a parser and has nothing to parse. `core/internal/ingest` holds a parser
registry, a charset decoder and a Priorbank reader, and the only way to reach any of them is
a Go test. There is no route by which a customer's file enters the system.

This is the first change of the ingest track and every change after it waits on the row it
creates: 2.2 parses a batch, 2.3 validates one, 2.4 applies a profile to one, 2.5 persists
one, 2.6 deduplicates across them.

Milestone: **Demo**. Capability: **`file-ingestion`** — introduced by change 2.2
`add-statement-parsing`, extended here.

## What Changes

- Migration 008 **extends** `import_batches`. Change 2.5's migration 007 already creates it
  in the minimal shape `transactions` points at, with row-level security already forced and
  a `status` `CHECK` admitting one placeholder value — 007's own comment defers the rest of
  the state machine to this change. 008 adds the uploader, the object key, the measured
  file facts and the other ten states.
- **The browser sends the bytes to the object store, not to `core`.** `CreateImportBatch`
  returns a presigned `PUT`. `core` signs it, never holds the file, and adds no second
  non-Connect browser surface — `CLAUDE.md` names exactly one, and it is `/auth`.
- **The client's declared size and type are signing conditions, not facts.** A River job
  reads the object back, computes the SHA-256, measures the byte length and sniffs the
  content type. Nothing downstream reads a number the browser supplied.
- `core/internal/blob` — an `ObjectStore` interface over an S3-compatible endpoint, with
  MinIO in the local overlay for the same reason and by the same precedent as Postgres.
- **`source_kind` is chosen at creation and is immutable.** A report line is computed from
  one source kind; letting it change after rows exist re-bases every report silently.
- A per-batch expiry job abandons a batch whose upload never arrives, so a signed URL that
  is never used does not leave a row that looks like work in progress.
- **No `UNIQUE (org_id, id, source_kind)`.** An earlier draft added one so 2.5 could hang a
  composite foreign key off it; 007 solved that with a constraint trigger instead, so the
  constraint would now be a redundant index on a tenant table.

## Capabilities

### New Capabilities
- None. `file-ingestion` is introduced by change 2.2.

### Extended Capabilities
- `file-ingestion`: the batch record, its lifecycle, and how a file's bytes get in. 2.2
  reads a file; this change is how a file arrives.

### Modified Capabilities
- `platform-foundation`: the local overlay gains a second stateful workload, and the base
  gains the object-store configuration `core` reads.

## Non-goals

- **No parsing.** Nothing opens the file. That is 2.2, and it reads the object this change
  stores.
- **No validation and no transactions.** 2.3 and 2.5.
- **No D1 rejection.** The SHA-256 is computed and stored; nothing compares it yet. 2.6
  owns every dedup level, and it will need no backfill because the column arrives full.
- **No mapping screen, no retention, no deletion, no virus scan, no resumable upload.**
- **No production bucket.** D-6 names no host and therefore no store. This change is
  developable and testable against the local overlay; it is not deployable until D-6 is
  answered.

## Impact

Touches `/core/migrations`, `/core/internal/db/query`, `/core/internal/ingest`,
`/core/internal/blob` (new), `/core/internal/jobs`, `/core/internal/config`,
`/proto/vekst/v1` and `/deploy/k8s` — **the last two need both reviewers**. `/web` gains
the upload screen's data layer only; the screen itself belongs to `add-web-experience`.

**Apply after 2.5 (migration 007).** This change alters a table 007 creates. That inverts
the order `docs/IMPLEMENTATION_PLAN.md` §3 implies — 2.1 before 2.5 — and the plan's order
is the one that is now wrong: 007 is written, tested and ahead of this.
