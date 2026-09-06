## ADDED Requirements

### Requirement: Tenant data is scoped to an organisation

The system SHALL model tenancy as `organizations` → `entities` → `accounts`. Every tenant
table SHALL carry `org_id`, and every entity-scoped table SHALL carry `entity_id`. `entity_id`
SHALL be populated from the first migration, even while each organisation has exactly one
entity, so that a holding customer needs no migration of existing data.

#### Scenario: An organisation owns entities and accounts

- **WHEN** an organisation is created and an entity and an account are created under it
- **THEN** the entity carries the organisation's identifier
- **AND** the account carries both the organisation's and the entity's identifiers

#### Scenario: An entity-scoped row cannot omit its entity

- **WHEN** an account is inserted with no `entity_id`
- **THEN** the insert is rejected by the schema
- **AND** no row is written

### Requirement: Row-level security enforces isolation, not application code

The system SHALL enable and force row-level security on every tenant table, with a policy
whose `USING` and `WITH CHECK` expressions both restrict rows to the current tenant. Isolation
SHALL NOT depend on a predicate written by the caller.

#### Scenario: A tenant reads only its own rows

- **WHEN** a transaction runs with the tenant context set to organisation A, and organisation
  B's rows exist in the same tables
- **THEN** every select returns A's rows only
- **AND** the query carries no organisation predicate of its own

#### Scenario: A tenant cannot write a row belonging to another tenant

- **WHEN** a transaction with the tenant context set to organisation A inserts a row stamped
  with organisation B's identifier
- **THEN** the insert is rejected
- **AND** the same rejection applies to an update that would move an existing row to B

#### Scenario: The table owner is not exempt

- **WHEN** the tables are inspected
- **THEN** each has row-level security both enabled and forced
- **AND** no role reaches tenant rows without a policy evaluating

### Requirement: Tenant context is transaction-local and fails closed

The system SHALL set the tenant context inside the transaction, scoped to that transaction, in
exactly the one place transactions are opened. A query issued with no tenant context SHALL
raise an error rather than return an empty result.

#### Scenario: Context does not outlive its transaction

- **WHEN** a transaction for organisation A commits, and a second transaction on the same
  pooled connection begins without a tenant context
- **THEN** the second transaction raises on any tenant table
- **AND** it does not observe organisation A's rows

#### Scenario: A missing context is loud, not empty

- **WHEN** a tenant table is queried with no tenant context set
- **THEN** the database raises an undefined-object error
- **AND** the caller does not receive zero rows, which would be indistinguishable from a
  customer with no data

#### Scenario: The tenant identifier is bound, not interpolated

- **WHEN** the transaction entry point sets the tenant context
- **THEN** it passes the identifier as a bound parameter
- **AND** no tenant identifier is concatenated into SQL text

### Requirement: References between tenant tables cannot cross an organisation

The system SHALL define foreign keys between tenant tables as composite keys including
`org_id`, because referential-integrity checks are not subject to row-level security.

#### Scenario: A cross-tenant reference is rejected

- **WHEN** a transaction with the tenant context set to organisation A inserts an account
  whose entity belongs to organisation B
- **THEN** the insert fails on the foreign key
- **AND** the row is not written

#### Scenario: A guessed identifier reveals nothing

- **WHEN** the insert above is compared with an insert naming an entity identifier that
  exists in no organisation
- **THEN** both fail in the same way
- **AND** the failure does not disclose whether another organisation owns the identifier

### Requirement: Every tenant table is proved to be covered

The system SHALL verify in CI that every table outside the checked-in exemption allowlist has
row-level security enabled, forced, and at least one policy. The allowlist SHALL remain the
only place an exemption is recorded.

#### Scenario: A new table without a policy fails the build

- **WHEN** a pull request adds a table that is neither allowlisted nor given a policy
- **THEN** the coverage test fails
- **AND** the pull request cannot be merged

#### Scenario: The coverage test can fail

- **WHEN** a table with no policy is created in a scratch database and the coverage test runs
- **THEN** the test reports that table
- **AND** the test is therefore known to detect the condition it asserts

#### Scenario: Enabled but not forced is not enough

- **WHEN** a table has row-level security enabled and a policy, but not forced
- **THEN** the coverage test fails on it

### Requirement: A background job carries its tenant identifier in its arguments

The system SHALL require a worker to take its tenant identifier from its job arguments and to
open its transaction through the same entry point a handler uses. A worker SHALL NOT obtain a
tenant identifier from ambient state.

#### Scenario: A job reads only its own tenant's rows

- **WHEN** a job whose arguments name organisation A runs, and organisation B's rows exist
- **THEN** it reads A's rows only

#### Scenario: A job with no tenant identifier does not run

- **WHEN** a job is enqueued whose arguments carry no organisation
- **THEN** the worker fails the job
- **AND** it does not execute against an unset or inherited tenant context

#### Scenario: Job arguments are documented as unisolated

- **WHEN** the worker package documentation is read
- **THEN** it states that River's tables carry no tenant column and no policy
- **AND** it states that job arguments therefore carry identifiers, never customer financial
  data

### Requirement: Resolving a user's organisations is a single reviewed exception

The system SHALL resolve which organisations a user belongs to through exactly one
`SECURITY DEFINER` function, which SHALL accept a user identifier, SHALL return only that
user's memberships, and SHALL be listed in the ownership file so that changing it requires
both reviewers.

#### Scenario: A user sees only their own memberships

- **WHEN** the function is called with a user identifier
- **THEN** it returns that user's organisations and roles
- **AND** it returns no membership belonging to any other user

#### Scenario: It is the only such exception

- **WHEN** the schema is inspected for `SECURITY DEFINER` functions reachable by the
  application role
- **THEN** this function is the only one
- **AND** it is named in the ownership file
