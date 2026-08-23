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

#### Scenario: Migrations complete before core serves traffic

- **WHEN** the environment is started from nothing
- **THEN** the migration Job runs to completion before any `core` pod becomes Ready
- **AND** no request is served against an unmigrated database

#### Scenario: A binary newer than the schema refuses to serve

- **WHEN** `core` starts against a database whose applied migration version is below the
  version the binary requires
- **THEN** `/readyz` fails and the pod does not become Ready
- **AND** the reason names the applied version and the required version

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

### Requirement: Background workers reach the database only through the entry point

The system SHALL require every River worker to access the database through the single
transaction entry point, never against the pool directly. River's own tables carry no tenant
column and no row-level security, so a worker is the one place where a missing tenant filter
has no request to make its absence visible. The rule that a worker takes its tenant
identifier from its job arguments SHALL be recorded where workers are written; enforcing it
requires the tenant context that change 1.1 introduces.

#### Scenario: A worker runs inside the transaction entry point

- **WHEN** any registered worker is inspected
- **THEN** its database access occurs through the single transaction entry point

#### Scenario: A worker that skips the entry point is rejected

- **WHEN** a pull request adds a worker that queries the pool directly
- **THEN** the transaction entry-point check fails
- **AND** the pull request cannot be merged

#### Scenario: The tenant rule is recorded for the change that enforces it

- **WHEN** the worker package is inspected
- **THEN** it documents that a handler takes its tenant identifier from job arguments and
  never from ambient state
- **AND** it names change 1.1 as the change that adds the enforcing context

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

#### Scenario: The generated types carry money safely in all three languages

- **WHEN** the types generated from the money contract are inspected
- **THEN** the minor-units field is a 64-bit integer in Go and in Python
- **AND** its TypeScript type is `string`, not `number`
- **AND** an amount of 12.34 EUR is representable as minor units 1234 with currency code
  `EUR`

Note: no RPC carries a monetary amount in this change. The end-to-end assertion that an
amount survives a real browser round trip belongs to the first change that returns one.

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

## MODIFIED Requirements

### Requirement: Generated code is committed and never drifts

The system SHALL commit generated code to `/core/gen`, `/web/src/gen` and
`/classifier/src/vekst`, and `sqlc` output to `/core/gen/db`. CI SHALL fail when any committed
output does not match what the generators produce from the checked-in sources. `vekst/v1`
SHALL generate Go and TypeScript; `vekst/internal/v1` SHALL generate Go and Python; `sqlc`
SHALL generate Go from the checked-in SQL. Every generator SHALL be local and pinned by its
language's lockfile.

#### Scenario: Regenerating produces no diff

- **WHEN** CI runs `make gen` on a pull request
- **THEN** `git diff --exit-code` reports no change across any generated directory

#### Scenario: A proto change without regeneration is rejected

- **WHEN** a pull request modifies a file under `/proto` without committing the regenerated
  stubs
- **THEN** the codegen job fails
- **AND** the pull request cannot be merged

#### Scenario: A query change without regeneration is rejected

- **WHEN** a pull request modifies a checked-in `.sql` query without committing the
  regenerated Go
- **THEN** the codegen job fails
- **AND** the pull request cannot be merged

### Requirement: CI gates every pull request

The system SHALL run one GitHub Actions workflow on every pull request, with parallel jobs
including at minimum `buf lint`, `buf breaking`, codegen drift, `go vet`, `go test`,
`govulncheck`, `ruff`, `mypy`, `pytest`, `tsc`, `vitest`, `gitleaks`, manifest validation,
container image builds, the migration round trip, and the database role assertions. A later
change adding a job SHALL NOT require this requirement to be restated. A failing job SHALL
block merge.

#### Scenario: A green pull request is mergeable

- **WHEN** every job in the workflow succeeds
- **THEN** the pull request is mergeable

#### Scenario: Failing Go tests block merge

- **WHEN** a pull request introduces a failing Go test or a `go vet` diagnostic
- **THEN** the workflow fails
- **AND** the pull request cannot be merged

#### Scenario: Failing Python checks block merge

- **WHEN** a pull request introduces a `ruff` violation, a `mypy` error or a failing
  `pytest` case under `/classifier`
- **THEN** the workflow fails
- **AND** the pull request cannot be merged

#### Scenario: A committed secret is rejected

- **WHEN** a pull request adds a file containing a credential that `gitleaks` recognises
- **THEN** the `gitleaks` job fails
- **AND** the pull request cannot be merged

#### Scenario: A migration that cannot be rolled back blocks merge

- **WHEN** a pull request adds a migration whose down step fails or is missing
- **THEN** the migration round-trip job fails
- **AND** the pull request cannot be merged

#### Scenario: A role that can bypass row-level security blocks merge

- **WHEN** a pull request changes migrations such that `vekst_app` gains `BYPASSRLS` or
  superuser
- **THEN** the role assertion job fails
- **AND** the pull request cannot be merged

### Requirement: One command starts the development environment in local Kubernetes

The system SHALL provide a documented single command that creates a local Kubernetes
cluster if none exists, deploys `core`, `classifier`, `web` and a development Postgres 16
into it via Tilt, applies migrations before `core` serves, and exposes `core` and `web`
through an Ingress. The README SHALL state the module path, the required tool versions and
the available make targets.

#### Scenario: A new developer starts the stack

- **WHEN** a developer with Go, Node, buf, Docker, k3d, Tilt and kubectl installed clones
  the repository and runs `make dev`
- **THEN** a local cluster is created if one is not already running
- **AND** Tilt builds all three images and applies the `local` overlay
- **AND** the migration Job completes, then the `core`, `classifier`, `web` and Postgres
  workloads all reach the Ready state
- **AND** the walking-skeleton page loads through the Ingress with no further configuration

#### Scenario: An unreachable cluster reports what to do

- **WHEN** `make dev` runs and the Docker daemon is not running or the cluster cannot be
  reached
- **THEN** the command fails with a message naming the required tool and the make target
  that fixes it
- **AND** it does not surface a raw `kubectl` connection-refused error as its only output

#### Scenario: Tearing down leaves nothing behind

- **WHEN** a developer runs `make down`
- **THEN** the Tilt session ends and the local cluster and its volumes are removed
- **AND** a subsequent `make dev` reproduces the environment, including its schema, from the
  manifests and migrations alone

### Requirement: The same manifests describe local and deployed environments

The system SHALL define the Kubernetes manifests as a Kustomize base with per-environment
overlays. An overlay SHALL differ from the base only in configuration — image tags, replica
counts, resource limits, ingress host, credentials, and endpoints of external dependencies —
and SHALL NOT introduce a workload that changes what the application is. Because every
environment must migrate its schema, the migration Job SHALL live in the base.

#### Scenario: The local overlay builds

- **WHEN** `kustomize build deploy/k8s/overlays/local` is run
- **THEN** it succeeds
- **AND** the output contains a Deployment and Service for each of `core`, `classifier` and
  `web`, one Ingress, and one migration Job

#### Scenario: An invalid manifest is rejected

- **WHEN** a pull request introduces a manifest that fails schema validation or a
  `kustomize build` error
- **THEN** the manifest validation job fails
- **AND** the pull request cannot be merged

#### Scenario: The development database cannot be inherited by a deployment

- **WHEN** the Kustomize base is inspected
- **THEN** it contains no Postgres workload
- **AND** the in-cluster Postgres exists only in the `local` overlay, so no deployment
  overlay can inherit a database that has no backup and restore procedure

#### Scenario: Migrating is not reinvented per environment

- **WHEN** the Kustomize base is inspected
- **THEN** it contains the migration Job
- **AND** an overlay supplies only its credentials and image tag, never its own copy of the
  Job


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
