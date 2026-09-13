**Budget.** `docs/IMPLEMENTATION_PLAN.md` §3 allocates **2 person-days** to change 2.1.
These tasks total **≈ 15.5 hours ≈ 1.9 person-days**. It fits, and it fits only because the
object store is one interface with one implementation. If MinIO has to be replaced with a
second backend inside this change, re-price it before starting.

**Ordering.** Apply after change 2.5's migration 007, which creates the table this one
alters, already forces row-level security on it, and leaves the state machine to §1.2.

**Ownership.** Track A throughout, except §3 and §7, which need **both reviewers**:
`/proto` and `/deploy/k8s/base` per CODEOWNERS.

**Rule for this change.** No task here reads a number the browser supplied. If a task
appears to need one, it is the wrong task — see design D2.

## 0. Decide before writing code

- [ ] 0.1 Confirm with the founder that D-6 stays unanswered and this change merges undeployable. The alternative is to answer D-6 first, which is cheaper than it looks
- [ ] 0.2 Agree the 25 MiB limit against the four real Priorbank exports and the two Kazakh files in `../docCl`. A limit set below a real customer file is discovered at the Demo

## 1. Migration — Track A

- [ ] 1.1 Migration 008 up: the emptiness assertion, then `ALTER TABLE import_batches` adding the twelve columns in the design
- [ ] 1.2 Widen the `status` `CHECK` from 007's single `'persisted'` to the eleven states, and drop the column default. Confirm `'persisted'` is **gone** rather than kept as a twelfth name for `imported`
- [ ] 1.3 `import_batches_measured_past_upload`, **including `'failed'` among the exempt statuses** — without it a batch that fails as `upload_missing` cannot be recorded at all. Verified on PostgreSQL 16 that the first draft of this constraint refused exactly that row
- [ ] 1.4 `UNIQUE (org_id, file_key)` and the two indexes. Add **no** policy and **no** `(org_id, id, source_kind)` constraint — 007 owns the first, and its constraint trigger made the second redundant
- [ ] 1.5 `REVOKE UPDATE (source_kind) ON import_batches FROM vekst_app` (design D4)
- [ ] 1.6 Migration 008 down — drop the added columns and restore 007's one-value `status` `CHECK` and its default; confirm `up → down → up` against a scratch database
- [ ] 1.7 Confirm the coverage test still sees the table, and that it needs no line in `rls-exempt-tables.txt` or `rls-shared-tenant-tables.txt`

## 2. Generated queries — Track A

- [ ] 2.1 `core/internal/db/query/ingest.sql`: `InsertImportBatch`, `GetImportBatch`, `ListImportBatches`
- [ ] 2.2 `RecordUploadMeasurement` and `SetImportBatchStatus`, both `:execrows` through `db.ExactlyOneRow`. Neither statement lists `source_kind` among the columns it sets
- [ ] 2.3 `AbandonExpiredBatch` — abandons only if the row is still `awaiting_upload`, so a late job cannot undo a real upload
- [ ] 2.4 Run `make gen`; confirm the codegen drift job stays green

## 3. Proto — both reviewers

- [ ] 3.1 `proto/vekst/v1/import.proto`: `ImportService`, the two enums and the five messages from the design
- [ ] 3.2 `buf lint`. Confirm `SERVICE_SUFFIX`, `ENUM_ZERO_VALUE_SUFFIX` and `ENUM_VALUE_PREFIX` pass without exclusions
- [ ] 3.3 `make gen`; confirm the Go and TypeScript stubs land and the drift check is green

## 4. Object store — Track A

- [ ] 4.1 `core/internal/blob`: an `ObjectStore` interface — `PresignPut`, `Head`, `Get`, `Delete` — and nothing else above it depends on S3
- [ ] 4.2 The S3-compatible implementation, with path-style addressing and a configurable endpoint
- [ ] 4.3 `PresignPut` signs `Content-Length` and `Content-Type`, and expires in `UploadURLLifetime`
- [ ] 4.4 The object key: `org/<org_id>/batch/<batch_id>`, built from identifiers only, never from `file_name`
- [ ] 4.5 Config: the seven fields in the design, with an empty endpoint disabling upload the way an empty Google client ID disables sign-in

## 5. Service and jobs — Track A

- [ ] 5.1 `core/internal/ingest/state.go`: the transition function and its table. Every illegal transition returns a named error
- [ ] 5.2 `CreateImportBatch`: insert `awaiting_upload`, presign, enqueue the expiry job at `upload_expires_at`
- [ ] 5.3 `ConfirmImportUpload`: enqueue the measurement job and return. It records nothing from the request (design D2)
- [ ] 5.4 The measurement job: `HEAD`, stream through SHA-256 and a byte counter with a hard stop at `UploadMaxBytes + 1`, sniff, write in one `db.InTx`
- [ ] 5.5 The job takes its tenant context from `db.OrgIDFromJobArgs`; its arguments are `org_id` and `batch_id` and carry no file content
- [ ] 5.6 The expiry job: abandon if still `awaiting_upload`, delete the object if one exists
- [ ] 5.7 `GetImportBatch` and `ListImportBatches`; register `ImportService` in `core/internal/server`

## 6. Tests — Track A

- [ ] 6.1 **Cross-tenant isolation:** organisation A cannot read, list or update B's batch, and the failure is a policy denial rather than a not-found
- [ ] 6.2 **Fail-closed:** every query in `ingest.sql` raises `42704` outside a tenant transaction, rather than returning zero rows
- [ ] 6.3 **The client lies about size:** a `declared_bytes` of 1 KiB with a 40 MiB object fails the batch as `file_too_large`, and the object is deleted
- [ ] 6.4 **The client lies about type:** `declared_type: text/csv` on an XLSX object records `content_type` as XLSX, from the magic bytes
- [ ] 6.5 **The upload never arrives:** the expiry job moves the batch to `abandoned`; a confirm afterwards does not resurrect it
- [ ] 6.6 **A late expiry job cannot undo a real upload:** a batch already `uploaded` is untouched
- [ ] 6.7 **`source_kind` is immutable:** an update attempt is denied by the column grant, and no query in `core/internal/db/query` sets it on an existing row
- [ ] 6.8 **The measured-past-upload constraint holds:** an attempt to set `status = 'uploaded'` with a NULL sha256 is rejected by the database, not merely by Go
- [ ] 6.8b **…and still admits a failure that happened before measurement:** a batch whose object was missing records `status = 'failed'`, `failure_code = 'upload_missing'` and three NULL measurements. This is the row the first draft of the constraint refused
- [ ] 6.9 **Transitions:** every legal transition in design D3 is accepted and every other pair is refused, table-driven
- [ ] 6.10 **Idempotent confirm:** running the measurement job twice writes the same row and enqueues no duplicate work
- [ ] 6.11 **No money:** the no-float test in `core/internal/money` covers the new generated structs, and `import.proto` declares no amount field

## 7. Deployment — both reviewers

- [ ] 7.1 MinIO in `deploy/k8s/overlays/local`, with the bucket created at start. Extend the comment in `deploy/k8s/base/kustomization.yaml` that already explains why Postgres lives there — do not copy it
- [ ] 7.2 The object-store environment on `core` in `deploy/k8s/base`, reading a Secret named `vekst-object-store`
- [ ] 7.3 A committed placeholder secret for the local overlay, matching the `google-oidc.yaml` pattern, so `kubectl kustomize` renders in CI

## 8. Front end — Track A

- [ ] 8.1 A typed client for the two RPCs plus the raw `PUT`, in `web/src/data`. No component here: the Imports screen belongs to `add-web-experience` §6
- [ ] 8.2 i18n keys for the six failure codes this change can produce, in `en` and `ru`

## 9. Close

- [ ] 9.1 Add `/core/internal/blob/` and `/core/internal/db/query/ingest.sql` to `.github/CODEOWNERS` under Track A
- [ ] 9.2 Record in `docs/IMPLEMENTATION_PLAN.md` §7 that D-6 now blocks a merged change, not only a future one
- [ ] 9.3 Update the capability spec and run the full suite
