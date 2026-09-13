## Why

`core/internal/ingest` detects charset, delimiter and number locale, and it is right about
Priorbank. It will be wrong about something, and when it is, there is no way to tell it so.

The Demo plan is explicit that there is no mapping screen and that "you write two import
profiles by hand". That sentence only works if a hand-written profile is a thing the system
can hold.

Milestone: **Demo**. Capability: **`file-ingestion`** (modified).

## What Changes

- Migration 010 creates `import_profiles` — a named, per-organisation set of parsing
  decisions: the column map, and optionally the charset, delimiter, decimal separator and
  date format.
- `import_batches` gains a nullable `import_profile_id`. A batch with no profile parses
  exactly as it does today.
- **A profile overrides detection field by field; it does not replace it.** A profile that
  names only `date_fmt` leaves charset and delimiter to the detector. The alternative makes
  the working path depend on a complete hand-written file, which is how a profile that is
  90% right becomes worse than no profile.
- `column_map` maps source column names to a **fixed** set of canonical fields. A constraint
  trigger refuses a key that is not one of them, so a typo is a write error and not a column
  that silently maps to nothing.
- `CreateImportBatchRequest` gains an optional `import_profile_id`. Adding a field is not a
  breaking change and `buf breaking` confirms it.

## Capabilities

### New Capabilities
- None.

### Modified Capabilities
- `file-ingestion`: a batch may name a profile, and parsing consults it before detection.

## Non-goals

- **No mapping screen.** Product owns `add-column-mapping-ui`. For the Demo a profile is
  written in SQL, which is what the plan already assumes.
- **No automatic profile selection.** A profile is named on the batch, never inferred from
  a header fingerprint. Inference is Product, and inferring the wrong one silently mis-maps
  an amount column.
- **No profile versioning and no history.** A profile is configuration, not a record of
  what happened; what happened is recorded by the batch and its validation.
- **No per-account profiles.** One profile, one organisation, named at upload.
- **No parser changes.** 2.2 owns detection; this change only supplies overrides to it.

## Impact

Touches `/core/migrations`, `/core/internal/db/query`, `/core/internal/ingest` and
`/proto/vekst/v1/import.proto` (**both reviewers**).

**Apply after 2.2.** There is nothing to override before detection exists.
