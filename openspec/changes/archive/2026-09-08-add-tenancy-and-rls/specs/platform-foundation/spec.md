## MODIFIED Requirements

### Requirement: Two database roles with distinct powers

The system SHALL define exactly two database roles that anything authenticates as.
`vekst_migrator` SHALL own the schema and apply migrations. `vekst_app` SHALL own nothing,
SHALL NOT hold `BYPASSRLS`, SHALL NOT be a superuser, and SHALL hold only DML rights. The
`core` service SHALL connect as `vekst_app`.

`vekst_migrator` SHALL ALSO NOT be a superuser and SHALL NOT hold `BYPASSRLS`, in every
environment including continuous integration and local development. Either attribute bypasses
row-level security unconditionally, which makes `FORCE ROW LEVEL SECURITY` decorative and
grants every `SECURITY DEFINER` function it owns an unbounded implicit bypass. A build
environment that provisions it as a superuser would pass a schema whose isolation does not
work where it is deployed.

`vekst_migrator` SHALL be provisioned by the environment rather than created by a migration,
because it is the role migrations are applied as and therefore exists before any of them
runs. `vekst_app` SHALL be created by the first migration.

One further role MAY exist that nothing authenticates as: it SHALL have no `LOGIN`, no
`SUPERUSER` and no `BYPASSRLS`, SHALL exist solely to own the single `SECURITY DEFINER`
function that resolves a user's organisations, and SHALL NOT be assumable by `vekst_app`.

#### Scenario: The migrator role is supplied by the environment

- **WHEN** the first migration runs
- **THEN** it does not attempt to create the role it is running as
- **AND** it creates `vekst_app`, which does not exist beforehand

#### Scenario: A missing or under-privileged migrator fails with a named reason

- **WHEN** migrations are applied by a role that is not `vekst_migrator`, or by one that
  cannot create `vekst_app`
- **THEN** the migration fails immediately
- **AND** the error names the role that must be provisioned, rather than surfacing a bare
  permission error from a later statement

#### Scenario: Neither authenticating role can bypass row-level security

- **WHEN** the roles are inspected after migrations are applied
- **THEN** `vekst_app` has `rolbypassrls` false and `rolsuper` false
- **AND** `vekst_migrator` has `rolbypassrls` false and `rolsuper` false
- **AND** a test asserts both, so a later `ALTER ROLE` granting either fails the build

#### Scenario: The build environment matches the deployed one

- **WHEN** the test suite runs against the migrated database
- **THEN** the schema is owned by a role that is subject to its own policies
- **AND** a mechanism that would work only for a superuser fails in the build rather than
  after deployment

#### Scenario: The third role cannot be reached

- **WHEN** the roles are inspected
- **THEN** the role owning the organisation-resolving function cannot log in
- **AND** `vekst_app` cannot assume it

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

#### Scenario: The migration Job carries the credentials core does not

- **WHEN** the migration Job's environment and mounted Secrets are inspected
- **THEN** they contain `vekst_migrator` credentials
- **AND** the two workloads read separate Secrets, so neither can inherit the other's

### Requirement: One transaction entry point

The system SHALL open database transactions in exactly one place, which every caller —
Connect RPC handler and background job alike — SHALL use. No other code SHALL begin a
transaction or issue a query directly against the pool.

That entry point SHALL take the organisation as a required argument of a type that cannot be
constructed outside the database package, and SHALL set the tenant context as the
transaction's first statement, transaction-locally, in that place and nowhere else. A
separate, explicitly named entry point SHALL serve the reads that have no tenant — readiness,
migration status, and the global users table — and the number of its call sites SHALL be
committed, so that adding one appears in review.

#### Scenario: Handlers and workers share one door

- **WHEN** the codebase is inspected for transaction or query calls against the connection
  pool
- **THEN** they occur only inside the database package's entry point

#### Scenario: A second door fails the build

- **WHEN** a pull request adds a direct pool query or `Begin` call outside the database
  package
- **THEN** the CI check for it fails
- **AND** the pull request cannot be merged

#### Scenario: Tenant context cannot be forgotten

- **WHEN** a caller opens a tenant transaction
- **THEN** the organisation is a required argument, so omitting it does not compile
- **AND** the context is set before any statement of the caller's own runs

#### Scenario: Tenant context cannot be invented

- **WHEN** a caller opens a tenant transaction
- **THEN** the organisation argument came from one of the database package's named
  constructors, because no other package can build the type
- **AND** the constructors that do not start from an authenticated session have their call
  sites counted, so adding one appears in review

#### Scenario: An untenanted transaction is visible in review

- **WHEN** a pull request adds a call site of the untenanted entry point
- **THEN** the committed call-site count no longer matches
- **AND** the check fails until the count is updated deliberately
