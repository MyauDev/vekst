# file-ingestion Specification

## Purpose

This delta extends `file-ingestion` with the parsing decisions a customer's file needs when
detection is wrong about it. Change 2.1 owns the batch and how its bytes arrive; this adds
the named, reusable description of how to read them.

## ADDED Requirements

### Requirement: A profile describes how to read one customer's export

The system SHALL store named, per-organisation import profiles carrying a column map and,
optionally, a charset, a delimiter, a decimal separator and a date format. A profile SHALL
declare the source kind it is for.

#### Scenario: A profile is named and reusable

- **WHEN** an organisation stores a profile named for one of its banks
- **THEN** that profile can be named on later batches
- **AND** its name is unique within the organisation

#### Scenario: A profile cannot be removed while a batch records using it

- **WHEN** a profile that a completed batch was parsed with is deleted
- **THEN** the deletion is refused
- **AND** the batch's record of how its numbers were produced is unchanged

### Requirement: A profile overrides detection field by field

The system SHALL apply each populated profile field in place of the detected value, and
SHALL detect any field the profile leaves unset. A batch naming no profile SHALL parse
exactly as it would if profiles did not exist.

#### Scenario: A profile that names one field leaves the rest to detection

- **WHEN** a profile sets only the date format
- **THEN** the charset, delimiter and decimal separator are detected
- **AND** the date format is the profile's

#### Scenario: No profile means no change

- **WHEN** a batch is parsed with no profile
- **THEN** the result is identical to parsing before profiles existed

#### Scenario: What a file was parsed with is recorded on the batch

- **WHEN** a profile is edited after a batch was parsed with it
- **THEN** the parameters recorded on that batch are unchanged
- **AND** they describe what was actually used

### Requirement: A column map names only canonical fields

The system SHALL restrict the targets of a column map to a fixed set of canonical fields,
and SHALL refuse a profile naming any other target at the moment the profile is written.

#### Scenario: A misspelled target is refused on write

- **WHEN** a profile maps a source column to `descrption`
- **THEN** the write is refused
- **AND** the refusal happens when the profile is stored, not when a file is parsed

#### Scenario: A valid map reaches validation as canonical fields

- **WHEN** a file is parsed with a profile mapping its own column names
- **THEN** the correctness checks read canonical fields
- **AND** they do not read the source's column names

### Requirement: A profile is chosen, never inferred

The system SHALL accept a profile identifier when a batch is created and SHALL NOT select a
profile automatically. The profile's source kind SHALL match the batch's.

#### Scenario: A mismatched source kind is refused

- **WHEN** a batch created as `bank` names a profile declared for `ledger`
- **THEN** the request is refused

#### Scenario: Another organisation's profile is unusable and undetectable

- **WHEN** a member of organisation A names a profile belonging to B
- **THEN** the request is refused
- **AND** the refusal is identical whether that profile exists or not

### Requirement: Profiles are isolated between organisations

The system SHALL apply row-level security to import profiles, with `FORCE ROW LEVEL
SECURITY`, and SHALL raise rather than return an empty result when no tenant context is set.

#### Scenario: One organisation cannot list another's profiles

- **WHEN** a member of organisation A lists profiles
- **THEN** only A's profiles are returned

#### Scenario: A read without tenant context raises

- **WHEN** a profile query runs outside a tenant transaction
- **THEN** the database raises `42704`
- **AND** it does not return zero rows

## MODIFIED Requirements

### Requirement: A batch exists before its bytes do

The system SHALL create the `import_batches` row when the upload is requested, not when it
completes. The row SHALL record the organisation, the entity, the uploading user, the
source kind, the object key **and, where one is given, the import profile**, and SHALL
start in `awaiting_upload`.

#### Scenario: Requesting an upload creates a visible batch

- **WHEN** a member of organisation A requests an upload for one of A's entities
- **THEN** a batch is created in `awaiting_upload`
- **AND** a presigned URL and its expiry are returned
- **AND** the batch is visible to A in the import list immediately

#### Scenario: A batch records the profile it was created with

- **WHEN** an upload is requested naming a profile
- **THEN** the batch stores that reference
- **AND** parsing consults it before detection

#### Scenario: An upload that never happens becomes abandoned

- **WHEN** the presigned URL expires and no object was written
- **THEN** the batch moves to `abandoned`
- **AND** it is never eligible for parsing

#### Scenario: A batch cannot be created for another organisation's entity

- **WHEN** a member of organisation A requests an upload naming an entity belonging to B
- **THEN** the request is refused
- **AND** the refusal is identical whether that entity exists or not
