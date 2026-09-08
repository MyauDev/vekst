## MODIFIED Requirements

### Requirement: The health service is unauthenticated and stateless

The health service SHALL answer without a database, SHALL carry no tenant data, and SHALL
remain reachable without authentication once authenticated routes exist. Kubernetes liveness
and readiness probes SHALL use it, and SHALL never be required to present a credential.

#### Scenario: Health responds without a database

- **WHEN** the database is unreachable and the health service is called
- **THEN** it reports serving
- **AND** liveness does not fail

#### Scenario: Health carries no tenant data

- **WHEN** the health response is inspected
- **THEN** it contains version and status information only
- **AND** it identifies no person and no organisation

#### Scenario: Kubernetes probes use the health endpoint

- **WHEN** the manifests are inspected
- **THEN** liveness probes the dependency-free endpoint
- **AND** readiness probes the endpoint that checks the database and the schema version

#### Scenario: Probes are exempt from authentication

- **WHEN** authentication middleware is in place and a probe arrives with no credential
- **THEN** the probe succeeds
- **AND** a probe is never answered with an unauthenticated error
