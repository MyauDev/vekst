## MODIFIED Requirements

### Requirement: One transaction entry point

The system SHALL open database transactions in exactly one place, which every caller —
Connect RPC handler and background job alike — SHALL use. No other code SHALL begin a
transaction or issue a query directly against the pool.

That entry point SHALL take the organisation as a required argument and SHALL set the tenant
context as the transaction's first statement, transaction-locally, in that place and nowhere
else. A separate, explicitly named entry point SHALL serve the reads that have no tenant —
readiness, migration status, and the global users table — and the number of its call sites
SHALL be committed, so that adding one appears in review.

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

#### Scenario: An untenanted transaction is visible in review

- **WHEN** a pull request adds a call site of the untenanted entry point
- **THEN** the committed call-site count no longer matches
- **AND** the check fails until the count is updated deliberately
