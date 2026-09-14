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

- [x] 0.1 Confirm with the founder that D-6 stays unanswered and this change merges undeployable. The alternative is to answer D-6 first, which is cheaper than it looks — confirmed 2026-09-13: merge undeployable, D-6 answered separately
- [x] 0.2 Agree the 25 MiB limit against the four real Priorbank exports and the two Kazakh files in `../docCl`. A limit set below a real customer file is discovered at the Demo — confirmed 2026-09-13: the four Priorbank fixtures in `core/testdata/priorbank-by/` are 35–203 KB, ~100x under the limit; `../docCl` does not exist in this environment so the Kazakh files could not be checked directly

## 1. Migration — Track A

- [x] 1.1 Migration 008 up: the emptiness assertion, then `ALTER TABLE import_batches` adding the twelve columns in the design
- [x] 1.2 Widen the `status` `CHECK` from 007's single `'persisted'` to the eleven states, and drop the column default. Confirm `'persisted'` is **gone** rather than kept as a twelfth name for `imported`
- [x] 1.3 `import_batches_measured_past_upload`, **including `'failed'` among the exempt statuses** — without it a batch that fails as `upload_missing` cannot be recorded at all. Verified on PostgreSQL 16 that the first draft of this constraint refused exactly that row
- [x] 1.4 `UNIQUE (org_id, file_key)` and the two indexes. Add **no** policy and **no** `(org_id, id, source_kind)` constraint — 007 owns the first, and its constraint trigger made the second redundant
- [x] 1.5 `REVOKE UPDATE (source_kind) ON import_batches FROM vekst_app` (design D4) — done as the stronger table-level revoke + selective grant-back the migration's own §5 comment explains, since a column-only revoke leaves the table-level grant in force
- [x] 1.6 Migration 008 down — drop the added columns and restore 007's one-value `status` `CHECK` and its default; confirm `up → down → up` against a scratch database
- [x] 1.7 Confirm the coverage test still sees the table, and that it needs no line in `rls-exempt-tables.txt` or `rls-shared-tenant-tables.txt`

## 2. Generated queries — Track A

- [x] 2.1 `core/internal/db/query/ingest.sql`: `InsertImportBatch`, `GetImportBatch`, `ListImportBatches`
- [x] 2.2 `RecordUploadMeasurement` and `SetImportBatchStatus`, both `:execrows` through `db.ExactlyOneRow`. Neither statement lists `source_kind` among the columns it sets
- [x] 2.3 `AbandonExpiredBatch` — abandons only if the row is still `awaiting_upload`, so a late job cannot undo a real upload — `:one` + `RETURNING id`, since zero rows is the expected outcome when the upload already arrived, not an error
- [x] 2.4 Run `make gen`; confirm the codegen drift job stays green — `go tool sqlc generate` run directly; `core/gen/db/ingest.sql.go` generated and compiles

## 3. Proto — both reviewers

- [x] 3.1 `proto/vekst/v1/import.proto`: `ImportService`, the two enums and the five messages from the design — every request message also carries `org_id`, a correction over the design doc recorded there and in design.md
- [x] 3.2 `buf lint`. Confirm `SERVICE_SUFFIX`, `ENUM_ZERO_VALUE_SUFFIX` and `ENUM_VALUE_PREFIX` pass without exclusions — clean, no output
- [x] 3.3 `make gen`; confirm the Go and TypeScript stubs land and the drift check is green — `core/gen/vekst/v1/{import.pb.go,vektv1connect/import.connect.go}` and `web/src/gen/vekst/v1/import_pb.ts` generated; `go build ./...` clean

## 4. Object store — Track A

- [x] 4.1 `core/internal/blob`: an `ObjectStore` interface — `PresignPut`, `Head`, `Get`, `Delete` — and nothing else above it depends on S3
- [x] 4.2 The S3-compatible implementation, with path-style addressing and a configurable endpoint — `aws-sdk-go-v2`'s `s3` client, `UsePathStyle` + `BaseEndpoint`
- [x] 4.3 `PresignPut` signs `Content-Length` and `Content-Type`, and expires in `UploadURLLifetime` — verified against a live MinIO container that a mismatched length, mismatched type, or expired URL is rejected (403), and a matching upload succeeds end to end
- [x] 4.4 The object key: `org/<org_id>/batch/<batch_id>`, built from identifiers only, never from `file_name`
- [x] 4.5 Config: the seven fields in the design, with an empty endpoint disabling upload the way an empty Google client ID disables sign-in

## 5. Service and jobs — Track A

- [x] 5.1 `core/internal/ingest/state.go`: the transition function and its table. Every illegal transition returns a named error
- [x] 5.2 `CreateImportBatch`: insert `awaiting_upload`, presign, enqueue the expiry job at `upload_expires_at` — via new `jobs.Client.InsertTxAt`, added because `InsertTx` hardcoded `nil` opts and could not schedule
- [x] 5.3 `ConfirmImportUpload`: enqueue the measurement job and return. It records nothing from the request (design D2)
- [x] 5.4 The measurement job: `HEAD`, stream through SHA-256 and a byte counter with a hard stop at `UploadMaxBytes + 1`, sniff, write in one `db.InTx`
- [x] 5.5 The job takes its tenant context from `db.OrgIDFromJobArgs`; its arguments are `org_id` and `batch_id` and carry no file content
- [x] 5.6 The expiry job: abandon if still `awaiting_upload`, delete the object if one exists
- [x] 5.7 `GetImportBatch` and `ListImportBatches`; register `ImportService` in `core/internal/server` — org resolution (`db.OrgIDForSession`) lives in `ingest.Service`, not the handler: `core/internal/server`'s own `TestHealthPathImportsNoDatabase` forbids importing `core/internal/db` anywhere in that package, so every RPC request carries `org_id` and the handler passes the caller's user id and the raw requested org id through

## 6. Tests — Track A

- [x] 6.1 **Cross-tenant isolation:** organisation A cannot read, list or update B's batch, and the failure is a policy denial rather than a not-found — `TestCrossTenantIsolation`
- [x] 6.2 **Fail-closed:** every query in `ingest.sql` raises `42704` outside a tenant transaction, rather than returning zero rows — `TestQueriesFailClosedOutsideATenantTransaction`, table-driven over all six statements
- [x] 6.3 **The client lies about size:** a `declared_bytes` of 1 KiB with a 40 MiB object fails the batch as `file_too_large`, and the object is deleted — `TestMeasurementFailsAnOversizedUpload`
- [x] 6.4 **The client lies about type:** `declared_type: text/csv` on an XLSX object records `content_type` as XLSX, from the magic bytes — `TestMeasurementSniffsContentTypeIgnoringDeclaredType`
- [x] 6.5 **The upload never arrives:** the expiry job moves the batch to `abandoned`; a confirm afterwards does not resurrect it — `TestExpiryAbandonsAnUploadThatNeverArrives`
- [x] 6.6 **A late expiry job cannot undo a real upload:** a batch already `uploaded` is untouched — `TestExpiryDoesNotUndoARealUpload`
- [x] 6.7 **`source_kind` is immutable:** an update attempt is denied by the column grant, and no query in `core/internal/db/query` sets it on an existing row — `TestSourceKindIsImmutable`; `ingest.sql`'s own header comment records the second half
- [x] 6.8 **The measured-past-upload constraint holds:** an attempt to set `status = 'uploaded'` with a NULL sha256 is rejected by the database, not merely by Go — `TestMeasuredPastUploadConstraintHolds`
- [x] 6.8b **…and still admits a failure that happened before measurement:** a batch whose object was missing records `status = 'failed'`, `failure_code = 'upload_missing'` and three NULL measurements. This is the row the first draft of the constraint refused — `TestFailureBeforeMeasurementAdmitsNullMeasurements`
- [x] 6.9 **Transitions:** every legal transition in design D3 is accepted and every other pair is refused, table-driven — `TestCheckTransitionTableDriven`, against an independently-written legal-pairs list
- [x] 6.10 **Idempotent confirm:** running the measurement job twice writes the same row and enqueues no duplicate work — `TestConfirmImportUploadTwiceIsIdempotent`
- [x] 6.11 **No money:** the no-float test in `core/internal/money` covers the new generated structs, and `import.proto` declares no amount field — `TestNoFloatMoneyFields` walks all of `/core` automatically; confirmed `import.proto` has no `Money` field

## 7. Deployment — both reviewers

- [x] 7.1 MinIO in `deploy/k8s/overlays/local`, with the bucket created at start. Extend the comment in `deploy/k8s/base/kustomization.yaml` that already explains why Postgres lives there — do not copy it — bucket creation is a `create-minio-bucket` Job (not an initContainer, which cannot reach its own Pod's not-yet-started main container), mirroring `migrate-job.yaml`'s shape
- [x] 7.2 The object-store environment on `core` in `deploy/k8s/base`, reading a Secret named `vekst-object-store`
- [x] 7.3 A committed placeholder secret for the local overlay, matching the `google-oidc.yaml` pattern, so `kubectl kustomize` renders in CI — implemented as the **`db-secrets.yaml` pattern instead**: MinIO's local credentials are throwaway values for a store that exists only on a laptop, exactly like Postgres's, so there is no real secret for a placeholder to stand in for (unlike Google's, D-6 names no production credential to protect). Both overlays verified with `kubectl kustomize` + `kubeconform -strict`: 18 resources (local), 8 (base), all valid

## 8. Front end — Track A

- [x] 8.1 A typed client for the two RPCs plus the raw `PUT`, in `web/src/data`. No component here: the Imports screen belongs to `add-web-experience` §6 — `web/src/data/importUpload.ts`; `createImportBatch`/`confirmImportUpload`/`putUpload`, tested against a router transport and a mocked `fetch`
- [x] 8.2 i18n keys for the six failure codes this change can produce, in `en` and `ru` — `upload_missing`, `file_too_large` (batch `failure_code` values), `object_store_not_configured`, `invalid_argument`, `batch_not_found`, `not_a_member` (RPC-level codes `importError` in `core/internal/server/import.go` produces)

## 9. Close

- [x] 9.1 Add `/core/internal/blob/` and `/core/internal/db/query/ingest.sql` to `.github/CODEOWNERS` under Track A
- [x] 9.2 Record in `docs/IMPLEMENTATION_PLAN.md` §7 that D-6 now blocks a merged change, not only a future one
- [x] 9.3 Update the capability spec and run the full suite — the delta spec already matched the implementation; added the one untested scenario it named (`TestCreateImportBatchRefusesAnotherOrganisationsEntity`). Full suite green: `go build`/`go vet`/`gofmt` clean, `buf lint` clean, `go test ./...` clean (one pre-existing, unrelated failure in `core/internal/db`'s forgery test — a Go-toolchain error-message wording difference in a fixture predating this change), live-dependency suites (`db`, `jobs`, `blob`, `ingest`, `migrate`) green individually, `web`: `tsc` + 94 vitest tests green, both k8s overlays render and pass `kubeconform -strict`. `scripts/check-db-entry-point.sh`'s committed `OrgIDFromJobArgs` count updated 1→3 for the two new workers
