## ADDED Requirements

### Requirement: Tenant data is scoped to an organisation

The system SHALL model tenancy as `organizations` → `entities` → `accounts`. Every tenant
table SHALL carry `org_id`, except `organizations` itself, whose own `id` is the tenant key.
Every entity-scoped table SHALL carry `entity_id`. `entity_id` SHALL be populated from the
first migration, even while each organisation has exactly one entity, so that a holding
customer needs no migration of existing data.

The reporting currency SHALL have exactly one home in the schema, because two unconstrained
copies of the same fact can disagree with no error and a report would then convert against
whichever the query happened to read.

#### Scenario: An organisation owns entities and accounts

- **WHEN** an organisation is created and an entity and an account are created under it
- **THEN** the entity carries the organisation's identifier
- **AND** the account carries both the organisation's and the entity's identifiers
- **AND** the organisation row is identified by its own primary key, which is the tenant key

#### Scenario: The reporting currency is not duplicated

- **WHEN** the schema is inspected for the currency a report converts into
- **THEN** exactly one table carries it
- **AND** no second table carries a copy that could disagree with it

#### Scenario: An entity-scoped row cannot omit its entity

- **WHEN** an account is inserted with no `entity_id`
- **THEN** the insert is rejected by the schema
- **AND** no row is written

### Requirement: Row-level security enforces isolation, not application code

The system SHALL enable and force row-level security on every tenant table, with a policy
applying to all roles whose `USING` and `WITH CHECK` expressions both restrict rows to the
current tenant. Isolation SHALL NOT depend on a predicate written by the caller.

A policy scoped to a named role MAY additionally exist where a specific, enumerated mechanism
requires it. Such a policy SHALL NOT apply to the application role, and the set of them SHALL
be asserted in CI, so that an unrestricted policy can never be introduced as though it were
one of these exceptions.

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
exactly the one place tenant transactions are opened — the untenanted entry point sets no
context and reaches no tenant table. A query issued with no tenant context SHALL
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

### Requirement: Referential-integrity checks cannot cross an organisation

The system SHALL scope every referential-integrity check on a tenant table by `org_id`,
because such checks are not subject to row-level security. Foreign keys between tenant tables
SHALL be composite and include `org_id`, and every uniqueness constraint on a tenant table
SHALL carry `org_id` as its leading column.

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

#### Scenario: A uniqueness collision cannot be observed across organisations

- **WHEN** a transaction with the tenant context set to organisation A inserts a row whose
  natural key duplicates a row belonging to organisation B
- **THEN** the insert succeeds, because uniqueness is scoped by `org_id`
- **AND** no duplicate-key error discloses the existence of B's row

### Requirement: Every tenant table is proved to isolate, not merely to have a policy

The system SHALL verify in CI that every relation outside the checked-in exemption allowlist
has row-level security enabled and forced, and has a policy that applies to all roles,
restricts a column of that table to the current tenant, and covers writes as well as reads.

The check SHALL enumerate relations by kind rather than assuming every one is an ordinary
table, because two kinds cannot be protected by a policy and neither appears in the obvious
listings. A materialized view SHALL be reported: row-level security cannot be enabled on one,
and the schema's default privileges grant the application role access to it on creation. A
partition SHALL be required to carry its own policy: a partition inherits neither the
row-level-security flags nor the policies of its parent, so reading it directly bypasses
them. That column
SHALL be `org_id`, except on the organisations table itself, where it is the primary key. A
policy that exists without restricting rows to the current tenant SHALL NOT satisfy the check.
The allowlist SHALL remain the only place a *table* exemption is recorded; the enumerated
policy and function exceptions are asserted by count rather than by allowlist, because there
should never be a second of either.

#### Scenario: A new table without a policy fails the build

- **WHEN** a pull request adds a table that is neither allowlisted nor given a policy
- **THEN** the coverage test fails
- **AND** the pull request cannot be merged

#### Scenario: A policy that does not isolate fails the build

- **WHEN** a table's policy that applies to all roles has an unconditionally true condition,
  or restricts on a column other than that table's tenant key
- **THEN** the coverage test fails on it
- **AND** the failure names the table and the policy

#### Scenario: The organisations table is covered by its own key

- **WHEN** the coverage test inspects the organisations table, which has no `org_id` column
- **THEN** it accepts the policy restricting the table's primary key to the current tenant
- **AND** it does not report the table as uncovered

#### Scenario: A read-only policy fails the build

- **WHEN** a table's only policy covers reads and no policy covers writes
- **THEN** the coverage test fails on it, rather than leaving writes silently rejected

#### Scenario: Enabled but not forced is not enough

- **WHEN** a table has row-level security enabled and a policy, but not forced
- **THEN** the coverage test fails on it

#### Scenario: A materialized view over tenant data fails the build

- **WHEN** a materialized view exists in the schema and is not allowlisted
- **THEN** the coverage test fails on it, naming it as a materialized view
- **AND** it does so even though the view appears in none of the schema's table listings

#### Scenario: A partition without its own policy fails the build

- **WHEN** a partition of a tenant table exists whose own row-level security is not enabled
  and forced, while its parent's is
- **THEN** the coverage test fails on it
- **AND** the failure states that the parent's policy does not govern a direct read of the
  partition

#### Scenario: The coverage test can fail

- **WHEN** each condition the test asserts is reproduced in turn in a scratch database — a
  table with no policy, an all-roles policy that is unconditionally true, a policy on the
  wrong column, a read-only policy, a table that is enabled but not forced, a second
  role-scoped policy, a second `SECURITY DEFINER` function, a materialized view, a partition
  without its own policy, and an application role holding `BYPASSRLS` — and the coverage test
  runs
- **THEN** the test reports the offending object in every case
- **AND** the test is therefore known to detect each condition it asserts

### Requirement: A tenant identifier is established, never asserted

The system SHALL make the tenant identifier passed to the transaction entry point
constructible only within the database package, through named functions that each correspond
to one way a caller may legitimately come to act for an organisation: an authenticated
session whose membership has been resolved, a background job's own arguments, and the creation
of a new organisation. Application code SHALL NOT be able to build a tenant identifier from an
untrusted value such as a request field. A further constructor MAY exist for tests, and SHALL
require a value that only a test can supply, so that it cannot become a fourth door for
production code.

#### Scenario: A request field cannot become a tenant identifier

- **WHEN** code outside the database package attempts to construct a tenant identifier
  directly from a raw identifier taken from a request
- **THEN** it does not compile

#### Scenario: A session may only act for an organisation it belongs to

- **WHEN** an authenticated caller asks to act for an organisation they are not a member of
- **THEN** no tenant identifier is produced and the call fails
- **AND** the failure is the same one produced when the organisation does not exist, so
  membership cannot be probed

#### Scenario: An unset tenant identifier does not silently mean a valid tenant

- **WHEN** a zero-valued tenant identifier reaches the transaction entry point
- **THEN** the entry point rejects it rather than opening a transaction

#### Scenario: The test constructor cannot be used outside tests

- **WHEN** non-test code attempts to call the test-only constructor
- **THEN** it must take on a dependency that only tests carry
- **AND** that dependency is visible in review and detectable in CI

### Requirement: A write that matches no row is reported as a failure

The system SHALL treat a write intended to affect a single row that affects none as an error
rather than as success, because row-level security filters rows silently on update and delete
and a caller cannot otherwise distinguish "not permitted" from "already in that state".

#### Scenario: An update filtered by policy does not report success

- **WHEN** a transaction updates a row that the tenant policy does not admit
- **THEN** the caller receives an error naming the zero affected-row count
- **AND** the caller does not observe a successful write

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

### Requirement: Resolving a user's organisations is a single enumerated exception

The system SHALL resolve which organisations a user belongs to through exactly one
`SECURITY DEFINER` function, which SHALL accept a user identifier, SHALL return only that
user's memberships, and SHALL be listed in the ownership file so that changing it requires
both reviewers.

That function SHALL obtain its access through a policy scoped to a dedicated role, and NOT
through a role privilege that bypasses row-level security. The dedicated role SHALL have no
`LOGIN`, no `SUPERUSER` and no `BYPASSRLS`, and the application role SHALL NOT be able to
assume it. Row-level security SHALL remain enabled and forced on the memberships table.

#### Scenario: A user sees only their own memberships

- **WHEN** the function is called with a user identifier
- **THEN** it returns that user's organisations and roles
- **AND** it returns no membership belonging to any other user

#### Scenario: The exception does not depend on who owns the schema

- **WHEN** the function is called with no tenant context set, against a database whose schema
  is owned by a role with no `SUPERUSER` and no `BYPASSRLS`
- **THEN** it returns the user's memberships
- **AND** the memberships table still has row-level security enabled and forced

#### Scenario: The exception does not widen into a second door

- **WHEN** the application role queries the memberships table directly with no tenant context
- **THEN** the query raises, exactly as it does for every other tenant table
- **AND** the application role cannot assume the dedicated role to read more

#### Scenario: It is the only such exception

- **WHEN** the schema is inspected for `SECURITY DEFINER` functions reachable by the
  application role, and for policies scoped to a particular role
- **THEN** exactly one function and exactly one such policy exist
- **AND** a test asserts both counts, so that adding a second fails the build
- **AND** they are named in the ownership file
