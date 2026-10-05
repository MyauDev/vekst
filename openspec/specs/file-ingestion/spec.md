# file-ingestion Specification

## Purpose

How a customer's file gets into the system, and what the system knows about it once it is
there. One batch is one uploaded file. Everything the pipeline later says about that file —
that it parsed, that it balanced, that it was already imported — hangs off this row.

## Requirements

### Requirement: A batch exists before its bytes do

The system SHALL create the `import_batches` row when the upload is requested, not when it
completes. The row SHALL record the organisation, the entity, the uploading user, the
source kind and the object key, and SHALL start in `awaiting_upload`.

#### Scenario: Requesting an upload creates a visible batch

- **WHEN** a member of organisation A requests an upload for one of A's entities
- **THEN** a batch is created in `awaiting_upload`
- **AND** a presigned URL and its expiry are returned
- **AND** the batch is visible to A in the import list immediately

#### Scenario: An upload that never happens becomes abandoned

- **WHEN** the presigned URL expires and no object was written
- **THEN** the batch moves to `abandoned`
- **AND** it is never eligible for parsing

#### Scenario: A batch cannot be created for another organisation's entity

- **WHEN** a member of organisation A requests an upload naming an entity belonging to B
- **THEN** the request is refused
- **AND** the refusal is identical whether that entity exists or not

### Requirement: The system measures the file itself

The system SHALL determine the SHA-256, the byte length and the content type by reading the
stored object. It SHALL NOT record any of the three from a value supplied by the client.
A batch SHALL NOT leave `awaiting_upload` for any state other than `abandoned` until all
three are recorded.

#### Scenario: A declared size smaller than the object is rejected

- **WHEN** a client declares 1 KiB and writes an object larger than the configured maximum
- **THEN** the batch fails with `file_too_large`
- **AND** the stored object is deleted
- **AND** no parsing is attempted

#### Scenario: A declared content type that does not match the bytes is ignored

- **WHEN** a client declares `text/csv` and writes a file whose leading bytes are `PK\x03\x04`
- **THEN** the recorded content type is the spreadsheet type
- **AND** the parser is selected from the recorded type, never from the declared one

#### Scenario: A batch cannot claim to be uploaded without measurement

- **WHEN** a write sets a batch to `uploaded` while its SHA-256 is unrecorded
- **THEN** the write is rejected by a check constraint

#### Scenario: Confirming twice changes nothing

- **WHEN** the same upload is confirmed a second time
- **THEN** the recorded SHA-256, byte length and content type are unchanged
- **AND** no duplicate downstream work is enqueued

### Requirement: A batch's source kind is fixed at creation

The system SHALL require `source_kind` to be `ledger` or `bank` when the batch is created,
and SHALL NOT permit it to change afterwards. The application role SHALL hold no update
privilege on the column.

#### Scenario: The source kind cannot be edited

- **WHEN** an update attempts to change a batch's `source_kind`
- **THEN** the statement is refused by the database
- **AND** the stored value is unchanged

#### Scenario: Correcting the source kind means a new import

- **WHEN** a customer uploads a bank export and marks it as ledger
- **THEN** the remedy is to abandon that batch and upload the file again
- **AND** no report already computed from the first batch changes basis

### Requirement: A batch moves through the pipeline in one direction

The system SHALL permit only the transitions declared for the ingest pipeline, and SHALL
distinguish a customer-facing rejection from an internal failure. A validation outcome
SHALL be `rejected`; a defect in the system SHALL be `failed` and SHALL carry a code.

#### Scenario: An illegal transition is refused

- **WHEN** a caller attempts to move a batch from `awaiting_upload` directly to `imported`
- **THEN** the transition is refused with a named error
- **AND** the stored status is unchanged

#### Scenario: A failure carries a code and not a sentence

- **WHEN** a batch fails because its object is missing from the store
- **THEN** `failure_code` is `upload_missing`
- **AND** no human-readable message is stored or returned

### Requirement: Batches are isolated between organisations

The system SHALL apply row-level security to `import_batches`, with `FORCE ROW LEVEL
SECURITY`, and SHALL raise rather than return an empty result when no tenant context is
set.

#### Scenario: One organisation cannot see another's imports

- **WHEN** a member of organisation A lists imports
- **THEN** only A's batches are returned
- **AND** no batch belonging to B appears, whatever its status

#### Scenario: A read without tenant context raises

- **WHEN** a batch query runs outside a tenant transaction
- **THEN** the database raises `42704`
- **AND** it does not return zero rows
