## ADDED Requirements

### Requirement: Migrations apply forward and backward

The system SHALL manage schema with `goose`, applied as the `vekst_migrator` role. Every
migration SHALL have a tested down step, and CI SHALL prove the round trip on every pull
request.

#### Scenario: A clean database reaches the current schema

- **WHEN** `goose up` runs against an empty Postgres 16 database
- **THEN** every migration applies in order
- **AND** `goose status` reports no pending migrations

#### Scenario: Migrations round-trip

- **WHEN** CI applies `goose up`, then `goose down` to zero, then `goose up` again against a
  scratch database
- **THEN** all three complete without error
- **AND** the resulting schema matches the schema after the first `goose up`

#### Scenario: A migration without a down step is rejected

- **WHEN** a pull request adds a migration with no `-- +goose Down` section
- **THEN** the round-trip job fails
- **AND** the pull request cannot be merged

### Requirement: Two database roles with distinct powers

The system SHALL define exactly two database roles. `vekst_migrator` SHALL own the schema and
apply migrations. `vekst_app` SHALL own nothing, SHALL NOT hold `BYPASSRLS`, SHALL NOT be a
superuser, and SHALL hold only DML rights. The `core` service SHALL connect as `vekst_app`.

#### Scenario: The application role cannot bypass row-level security

- **WHEN** the roles are inspected after migrations are applied
- **THEN** `vekst_app` has `rolbypassrls` false and `rolsuper` false
- **AND** a test asserts this, so a later `ALTER ROLE` granting it fails the build

#### Scenario: The application role cannot create objects

- **WHEN** a session connected as `vekst_app` attempts to create a table in `public`
- **THEN** the statement is denied for lack of privilege

#### Scenario: A new table is usable without a follow-up grant

- **WHEN** `vekst_migrator` creates a table in a later migration
- **THEN** `vekst_app` can select, insert, update and delete on it immediately
- **AND** no additional `GRANT` statement is required in that migration

#### Scenario: Migrator credentials are absent from the serving container

- **WHEN** the `core` Deployment's environment and mounted Secrets are inspected
- **THEN** they contain `vekst_app` credentials only
- **AND** no `vekst_migrator` credential is present

### Requirement: One transaction entry point

The system SHALL open database transactions in exactly one place, which every caller —
Connect RPC handler and background job alike — SHALL use. No other code SHALL begin a
transaction or issue a query directly against the pool.

#### Scenario: Handlers and workers share one door

- **WHEN** the codebase is inspected for transaction or query calls against the connection
  pool
- **THEN** they occur only inside the database package's entry point

#### Scenario: A second door fails the build

- **WHEN** a pull request adds a direct pool query or `Begin` call outside the database
  package
- **THEN** the CI check for it fails
- **AND** the pull request cannot be merged

### Requirement: Background jobs are enqueued transactionally

The system SHALL run background jobs through River, storing them in Postgres, and SHALL
enqueue a job inside the caller's transaction so that the job exists if and only if the rows
it concerns were committed.

#### Scenario: A committed transaction produces a runnable job

- **WHEN** a job is enqueued inside a transaction that commits
- **THEN** the job is present in River's table
- **AND** a worker executes it

#### Scenario: A rolled-back transaction produces no job

- **WHEN** a job is enqueued inside a transaction that then rolls back
- **THEN** no job row exists
- **AND** no worker ever executes it

#### Scenario: A failing job is retried rather than lost

- **WHEN** a worker returns an error
- **THEN** River schedules a retry with backoff
- **AND** the job is not discarded on its first failure

### Requirement: Job handlers establish their own tenant context

The system SHALL require every River worker to obtain its tenant context from its job
arguments through the single transaction entry point, never from ambient state. River's own
tables carry no tenant column and no row-level security, so a worker that does not set
context explicitly runs unfiltered.

#### Scenario: A worker runs inside the transaction entry point

- **WHEN** any registered worker is inspected
- **THEN** its database access occurs through the single transaction entry point
- **AND** its tenant identifier is read from the job's arguments

#### Scenario: A worker that skips the entry point is rejected

- **WHEN** a pull request adds a worker that queries the pool directly
- **THEN** the transaction entry-point check fails
- **AND** the pull request cannot be merged

### Requirement: Infrastructure tables are allowlisted explicitly

The system SHALL carry a checked-in allowlist naming every table that is infrastructure
rather than tenant data, stating why the exemption exists. The row-level-security coverage
test added by a later change SHALL read this file rather than embedding its own exceptions.

#### Scenario: River and migration tables are listed

- **WHEN** the allowlist is inspected
- **THEN** it names River's tables and the migration version table
- **AND** it states that adding an entry is a security decision

#### Scenario: Adding an exemption requires both reviewers

- **WHEN** a pull request adds a line to the allowlist with approval from one developer
- **THEN** the required-reviewers check is unsatisfied
- **AND** the pull request cannot be merged

### Requirement: Money is integer minor units with an explicit currency

The system SHALL represent monetary amounts as a signed `int64` count of the currency's
minor units together with an uppercase ISO-4217 currency code, in the proto contract, in Go
and in Python. It SHALL NOT represent a monetary amount as a floating-point number in any
language, and SHALL NOT expose one to TypeScript as a JavaScript number.

#### Scenario: A monetary amount survives the wire

- **WHEN** an amount of 12.34 EUR crosses the browser API
- **THEN** it is carried as minor units 1234 with currency code `EUR`
- **AND** the TypeScript type of the minor-units field is `string`, not `number`

#### Scenario: Currencies with non-standard exponents are handled

- **WHEN** an amount is formatted for JPY, which has exponent 0, and for KWD, which has
  exponent 3
- **THEN** each is scaled by its own exponent from the currency table
- **AND** neither is scaled by a hardcoded factor of 100

#### Scenario: Mixed-currency arithmetic is refused

- **WHEN** two amounts with different currency codes are added
- **THEN** the operation returns an error
- **AND** it does not silently adopt either currency

#### Scenario: A floating-point money field fails the build

- **WHEN** a pull request declares a monetary field as a floating-point type in Go or Python
- **THEN** the no-float guard test fails
- **AND** the pull request cannot be merged

### Requirement: Readiness reflects dependencies; liveness does not

The system SHALL expose `/readyz`, reporting ready only when the process responds and the
database pool answers. `/healthz` SHALL remain dependency-free. Kubernetes SHALL use
`/healthz` for liveness and `/readyz` for readiness.

#### Scenario: A healthy pod with a reachable database serves traffic

- **WHEN** `core` is running and the database answers
- **THEN** `/healthz` and `/readyz` both succeed
- **AND** the pod is Ready

#### Scenario: A database outage removes traffic without restarting pods

- **WHEN** the database becomes unreachable while `core` is running
- **THEN** `/readyz` fails and the pod leaves the Ready state
- **AND** `/healthz` continues to succeed, so Kubernetes does not restart the pod

#### Scenario: Recovery needs no intervention

- **WHEN** the database becomes reachable again
- **THEN** `/readyz` succeeds and the pod returns to Ready
- **AND** no pod was restarted during the outage

### Requirement: Generated query code is committed and never drifts

The system SHALL generate typed query code with `sqlc` into the committed generated-code
tree, and CI SHALL fail when the committed output does not match what `sqlc` produces from
the checked-in SQL.

#### Scenario: Regenerating produces no diff

- **WHEN** CI runs the generators on a pull request
- **THEN** `git diff --exit-code` reports no change across both the `buf` and `sqlc` outputs

#### Scenario: A query change without regeneration is rejected

- **WHEN** a pull request modifies a `.sql` query without committing regenerated code
- **THEN** the drift job fails
- **AND** the pull request cannot be merged

## MODIFIED Requirements

### Requirement: The health service is unauthenticated and stateless

The system SHALL expose `vekst.v1.HealthService/Check` without authentication and without a
tenant context. The handler SHALL NOT read any tenant data and SHALL NOT require a database
connection. It SHALL remain the only unauthenticated RPC until a later change adds one
deliberately.

#### Scenario: Health responds without a database

- **WHEN** `core` starts with no reachable Postgres instance
- **THEN** the process starts successfully
- **AND** `HealthService/Check` returns `STATUS_SERVING`

#### Scenario: Health carries no tenant data

- **WHEN** `HealthService/Check` is called
- **THEN** the response contains only status, version and build time
- **AND** no query is issued against any tenant-scoped table

#### Scenario: Kubernetes liveness uses the health endpoint

- **WHEN** the `core` Deployment is applied with `/healthz` configured as its liveness probe
- **THEN** the pod stays alive even though no database is reachable
- **AND** the probe does not depend on any tenant-scoped state

#### Scenario: Kubernetes readiness uses the readiness endpoint

- **WHEN** the `core` Deployment is applied
- **THEN** its readiness probe targets `/readyz`, not `/healthz`
- **AND** readiness depends on the database while liveness does not
