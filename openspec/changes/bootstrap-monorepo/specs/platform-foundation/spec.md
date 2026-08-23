## ADDED Requirements

### Requirement: End-to-end walking skeleton across both service boundaries

The system SHALL serve a browser page that renders values obtained from the Go `core`
service over a Connect RPC, one of which `core` obtains from the Python `classifier`
service over gRPC. Every wire type on every hop SHALL be generated from `/proto`; no side
SHALL contain a hand-written definition of one.

#### Scenario: A value travels the full chain

- **WHEN** a developer runs `make dev` and opens the web application in a browser
- **THEN** the page issues a `vekst.v1.HealthService/Check` call to `core` through the
  cluster Ingress
- **AND** `core` issues a `vekst.internal.v1.Classifier/Version` call over gRPC
- **AND** the page renders `version`, `built_at` and a non-empty `classifier_version`
- **AND** every message on both hops is typed by code generated from `/proto`

#### Scenario: The wire types are not hand-written

- **WHEN** the repository is searched for declarations of `CheckRequest`, `CheckResponse`,
  `VersionRequest` or `VersionResponse` outside the generated output directories
- **THEN** no such declaration exists in Go, TypeScript or Python

### Requirement: Generated code is committed and never drifts

The system SHALL commit `buf` output to `/core/gen`, `/web/src/gen` and `/classifier/gen`,
and CI SHALL fail when the committed output does not match what `buf generate` produces
from `/proto`. `vekst/v1` SHALL generate Go and TypeScript; `vekst/internal/v1` SHALL
generate Go and Python.

#### Scenario: Regenerating produces no diff

- **WHEN** CI runs `buf generate` on a pull request
- **THEN** `git diff --exit-code` reports no change

#### Scenario: A proto change without regeneration is rejected

- **WHEN** a pull request modifies a file under `/proto` without committing the regenerated
  stubs
- **THEN** the codegen job fails
- **AND** the pull request cannot be merged

### Requirement: The proto contract is linted and protected from breaking changes

The system SHALL run `buf lint` and `buf breaking` on every pull request. `buf breaking`
SHALL compare against the `main` branch, and SHALL pass when `main` contains no proto files
rather than erroring.

#### Scenario: A conforming proto change passes

- **WHEN** a pull request adds a new field with a new field number to an existing message
- **THEN** `buf lint` and `buf breaking` both pass

#### Scenario: A breaking proto change is blocked

- **WHEN** a pull request removes a field, reuses a field number, or renames a service
- **THEN** `buf breaking` fails
- **AND** the pull request cannot be merged

#### Scenario: The first proto commit has no baseline

- **WHEN** `buf breaking` runs against a `main` branch that contains no `/proto` directory
- **THEN** the job passes rather than erroring on a missing baseline

### Requirement: CI gates every pull request

The system SHALL run one GitHub Actions workflow on every pull request with parallel jobs
for `buf lint`, `buf breaking`, codegen drift, `go vet`, `go test`, `govulncheck`, `ruff`,
`mypy`, `pytest`, `tsc`, `vitest`, `gitleaks`, manifest validation, and container image
builds. A failing job SHALL block merge.

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

#### Scenario: Kubernetes probes use the health endpoint

- **WHEN** the `core` Deployment is applied with `/healthz` configured as its liveness and
  readiness probe
- **THEN** the pod reaches the Ready state even though no database is reachable
- **AND** the probe does not depend on any tenant-scoped state

### Requirement: The classifier is a separate service that answers over gRPC

The system SHALL run the classification service as a Python process separate from `core`,
implementing `vekst.internal.v1.Classifier/Version` and the standard gRPC health-checking
protocol. `core` SHALL reach it only by its in-cluster Service address.

#### Scenario: The classifier reports its version

- **WHEN** `core` calls `Classifier/Version` on the running service
- **THEN** the response carries a non-empty `engine_version` and an RFC 3339 `built_at`

#### Scenario: Kubernetes probes the classifier over gRPC

- **WHEN** the `classifier` Deployment is applied with a gRPC liveness and readiness probe
- **THEN** the pod reaches the Ready state
- **AND** no HTTP server is required for the probe to succeed

#### Scenario: The engine itself is out of scope

- **WHEN** the classifier's proto contract is inspected
- **THEN** it declares `Version` and no `ClassifyBatch` RPC
- **AND** no taxonomy, vendor memory, rule or threshold type is declared

### Requirement: The classifier holds no database credentials

The system SHALL NOT supply the `classifier` service with a database connection string,
credential, or any other means of reaching Postgres, in this change or any later one. Tenant
isolation SHALL remain single-mechanism, enforced by Postgres RLS in `core` alone.

#### Scenario: No database configuration reaches the classifier

- **WHEN** the `classifier` Deployment and its ConfigMaps and Secrets are inspected in every
  overlay
- **THEN** no database host, port, user, password or connection string is present
- **AND** the container image contains no Postgres client library

#### Scenario: The classifier declares no database dependency

- **WHEN** `/classifier/pyproject.toml` is inspected
- **THEN** it declares no database driver or ORM dependency

### Requirement: The classifier is unreachable from outside the cluster

The system SHALL expose the `classifier` Service as ClusterIP only, with no Ingress rule and
no route from the public host. Only `core` SHALL address it.

#### Scenario: No ingress route reaches the classifier

- **WHEN** the Ingress in the Kustomize base is inspected
- **THEN** it routes `/rpc/*` to `core` and all other paths to `web`
- **AND** no rule, in the base or in any overlay, routes to the `classifier` Service

#### Scenario: A browser cannot call the classifier

- **WHEN** a request for a `vekst.internal.v1.Classifier` method is sent to the public
  ingress host on any path
- **THEN** it does not reach the classifier

### Requirement: Core degrades rather than fails when the classifier is unreachable

The system SHALL treat `classifier_version` as best-effort. `core` SHALL report itself as
serving whether or not the classifier answers, encoding the failure posture that a
classifier outage is a retryable condition and not an outage of `core`.

#### Scenario: The classifier is down

- **WHEN** the `classifier` Deployment is scaled to zero and `HealthService/Check` is called
- **THEN** `core` returns `STATUS_SERVING`
- **AND** `classifier_version` is empty
- **AND** the call does not hang beyond its configured timeout or return an error

### Requirement: A build identifies itself

The system SHALL embed the git commit SHA and the build timestamp into the `core` binary at
build time, and SHALL return them from `HealthService/Check`.

#### Scenario: Version reflects the built commit

- **WHEN** `core` is built from a given commit and `HealthService/Check` is called
- **THEN** the `version` field equals that commit's SHA
- **AND** `built_at` is a valid RFC 3339 timestamp

#### Scenario: An unstamped build is identifiable

- **WHEN** `core` is built without the version stamping flags, as in a bare `go run`
- **THEN** `version` returns `dev` rather than an empty string or a misleading value

### Requirement: One command starts the development environment in local Kubernetes

The system SHALL provide a documented single command that creates a local Kubernetes
cluster if none exists, deploys `core`, `classifier`, `web` and an empty development
Postgres 16 into it via Tilt, and exposes `core` and `web` through an Ingress. The README SHALL state the module
path, the required tool versions and the available make targets.

#### Scenario: A new developer starts the stack

- **WHEN** a developer with Go, Node, buf, Docker, k3d, Tilt and kubectl installed clones
  the repository and runs `make dev`
- **THEN** a local cluster is created if one is not already running
- **AND** Tilt builds all three images and applies the `local` overlay
- **AND** the `core`, `classifier`, `web` and Postgres workloads all reach the Ready state
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
- **AND** a subsequent `make dev` reproduces the environment from the manifests alone

### Requirement: Editing source updates the running cluster without a manual rollout

The system SHALL configure Tilt `live_update` so that source changes reach the running
workloads without a developer issuing a build, push or rollout command.

#### Scenario: A Go change reaches the running pod

- **WHEN** a developer edits a Go source file under `/core`
- **THEN** Tilt syncs the change, recompiles inside the container and restarts the process
- **AND** the change is observable in the browser without a manual image build or
  `kubectl rollout`

#### Scenario: A Python change reaches the running pod

- **WHEN** a developer edits a Python source file under `/classifier/src`
- **THEN** Tilt syncs the change into the running container and restarts the gRPC server
- **AND** the change is observable through `core` without a manual image build

#### Scenario: A web change reaches the browser without a restart

- **WHEN** a developer edits a file under `/web/src`
- **THEN** Tilt syncs it into the running container and Vite applies hot module replacement
- **AND** the pod is not restarted and the browser is not reloaded from scratch

### Requirement: The same manifests describe local and deployed environments

The system SHALL define the Kubernetes manifests as a Kustomize base with per-environment
overlays. An overlay SHALL differ from the base only in configuration — image tags, replica
counts, resource limits, ingress host, and endpoints of external dependencies — and SHALL
NOT introduce a workload that changes what the application is.

#### Scenario: The local overlay builds

- **WHEN** `kustomize build deploy/k8s/overlays/local` is run
- **THEN** it succeeds
- **AND** the output contains a Deployment and Service for each of `core`, `classifier` and
  `web`, and one Ingress

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

### Requirement: The browser reaches the API same-origin through the Ingress

The system SHALL route browser traffic through a single Ingress host, sending `/rpc/*` to
the `core` Service and all other paths to the `web` Service. No cross-origin request SHALL
be required, and the routing SHALL be defined in the Kustomize base so that every
environment inherits it.

#### Scenario: RPC and application share one origin

- **WHEN** the browser loads the application and issues a `HealthService/Check` call
- **THEN** both the document and the RPC are served from the same Ingress host
- **AND** the RPC succeeds with no cross-origin response header

#### Scenario: Routing is not redefined per environment

- **WHEN** the `local` overlay is compared to the base
- **THEN** the overlay changes the ingress host only
- **AND** it does not restate the path routing rules

### Requirement: Ownership boundaries are enforced by review

The system SHALL define a `CODEOWNERS` file that assigns `/core/internal/ingest` and
`/core/internal/dedup` to Track A, `/core/classify`, `/core/internal/report` and
`/classifier` to Track B,
and SHALL require review from both owners for any change under `/proto` or
`/deploy/k8s/base`.

#### Scenario: A contract change needs both reviewers

- **WHEN** a pull request modifies a file under `/proto` or `/deploy/k8s/base` and has
  approval from one developer
- **THEN** the required-reviewers check is unsatisfied
- **AND** the pull request cannot be merged

### Requirement: Project documentation describes the code as built

The system SHALL carry a `README.md` and a `CLAUDE.md` at the repository root, both written
after the implementation exists and both describing the repository as it actually is rather
than as it was planned. `CLAUDE.md` SHALL state the build and test commands, the directory
layout with its track ownership, and the invariants that later changes must not violate.

#### Scenario: The README gets a developer running

- **WHEN** a developer who has not seen the repository follows the README from a clean
  machine
- **THEN** they install the named tool versions, run the named command, and reach the
  walking skeleton in a browser
- **AND** no step required to get there is missing from the document

#### Scenario: CLAUDE.md records the invariants

- **WHEN** `CLAUDE.md` is inspected
- **THEN** it names the money representation, tenancy and RLS rules, the append-only
  classification rule, the one-row-per-payment-or-posting grain, the rule that the
  classifier never holds database credentials, and the rule that generated code is never
  hand-edited
- **AND** each rule is stated as a constraint, not as background

#### Scenario: The documentation does not describe code that was never written

- **WHEN** `README.md` and `CLAUDE.md` are compared against the merged implementation
- **THEN** every command they name runs successfully
- **AND** every directory they describe exists

### Requirement: Environment-only build artefacts are not tracked

The system SHALL ignore editor, operating-system and build artefacts, and SHALL remove the
`.DS_Store` files currently tracked in the repository.

#### Scenario: OS metadata cannot be committed

- **WHEN** a developer stages all changes in a working tree containing `.DS_Store` files
- **THEN** no `.DS_Store` file is added to the index
- **AND** `git ls-files` reports none tracked
