## Context

The repository is documents only: four design documents, an OpenSpec config, one commit.
`docs/IMPLEMENTATION_PLAN.md` §3 lists change 0.1 as the sole item with no predecessor,
4 person-days, both developers. Everything else in the Demo milestone waits on it.

Two constraints shape this design more than any technical preference:

1. **Two developers on two tracks, sharing one seam.** §2.1 of the plan says: after the
   foundation, do not both edit the same file. The directory layout and CODEOWNERS are the
   cheapest enforcement of that, and they cost nothing to put in on day one.
2. **The toolchain is the risk, not the code.** buf + connect-go + connect-es + Vite +
   Tailwind v4 + React 19 is six moving parts that must agree. If they are going to fight,
   it is better to find out in day 1 of 29 than in day 20.

Two founder decisions taken after the first draft shape the rest of this document: the
Demo **will be hosted**, and the classification engine is a **Python service from day one**
rather than a Go package extracted at Commercial. The first makes deployment readiness worth
paying for; the second is recorded as a reversal of `ARCHITECTURE.md` A-1/A-2 in D2.

The development environment runs on **local Kubernetes driven by Tilt**, so that the
artifact developed against and the artifact deployed are the same artifact. That decision
is D5, and it the one that most changes the shape of this change.

Three services, not two: `core` (Go), `classifier` (Python), `web` (TypeScript).

Local toolchain confirmed present: Go 1.26.1, buf, Docker (daemon not currently running).
To be added or upgraded: **Node 24** (the machine has 22.17.1), **Python 3.14**, `uv`,
`k3d`, `tilt`, `kubectl`, `kustomize`.

## Goals / Non-Goals

**Goals:**

- A **walking skeleton that crosses both boundaries**: `make dev` brings the stack up in a
  local Kubernetes cluster, and a browser page renders a value that travelled browser →
  Connect → `core` → gRPC → `classifier` and back, through types nobody hand-wrote. Half a
  skeleton would leave the boundary that D2 exists to prove untested.
- CI that gates every later change: `buf lint`, `buf breaking`, `go vet`, `go test`,
  `govulncheck`, `tsc`, `vitest`, `gitleaks`, image build.
- A directory layout that lets Track A and Track B work without touching each other's files.
- One command a new developer runs to get productive, documented in the README.
- Kubernetes manifests that are the **deployment artifact**, not a development toy: the
  local overlay and any future deployment overlay differ in configuration, never in the
  set of workloads.

**Non-Goals:**

- Any Postgres schema, `goose`, `sqlc`, the `Money` type, the no-float tests — change 0.2.
- `ClassifyBatch`, the taxonomy, and engine layers L0–L2 — change 3.2. The service exists;
  the engine does not. See D2.
- River — needs the database. **It is currently named in no Demo change**; it belongs in 0.2.
- A provisioned cluster, a DNS name, TLS certificates, backups, or a deployment overlay.
  The Demo is now confirmed hosted, so this is **required work that no change currently
  owns**; D-6 still does not name the target host. The base is structured so the overlay is
  purely additive when it arrives. See Open Questions.
- Auth, tenancy, RLS, ingest, the classifier contract, `/classifier`, `/site`, i18n, theming.

## Decisions

### D1 — Data model: none

**No tables are created by this change.** No migration runs, no `org_id` column exists, no
RLS policy is written. The `local` overlay runs an empty Postgres 16 in-cluster so that 0.2
has a target to migrate against; nothing connects to it yet, and `core` starts and passes
its health check with the database absent.

That Postgres lives in the **local overlay only, never in the base** — see D5. A reporting
product for other people's finances needs a database with a backup and restore procedure
somebody has rehearsed, which a development `StatefulSet` is not.

The `classifier` gets no database at all, in this change or any later one — `ARCHITECTURE.md`
A-4. Now that the service exists, that invariant is asserted by a spec scenario rather than
by a paragraph.

This is stated explicitly because it is the last change for which it will be true. From 0.2
onward every table carries `org_id`, an RLS policy and `FORCE ROW LEVEL SECURITY`.

**Invariant call-out, per the repo rules:** this change touches **no money, no currency, no
tenant isolation, no `source_kind`, and no classifier contract.** It does establish the proto
workspace those contracts will later live in — the shape of `/proto` is the part of this
change that is expensive to redo.

### D2 — The classifier is a separate Python service from day one

`ARCHITECTURE.md` A-1 and A-2 place the Python split at the Commercial milestone, with the
engine living in `/core/classify` behind a Go interface until then. **That decision is
reversed here at the founder's direction**, and this section records the reversal rather
than quietly implementing it.

**What it costs**, by the architecture document's own accounting (§2.1, §2.2): roughly three
days inside the tightest milestone, two runtimes, two dependency managers, two lint and
test pipelines, and a network boundary where a function call would otherwise be. Track B
now writes engine layers L0–L2 in Python rather than Go, which §9 explicitly weighed against
on the grounds that the team is faster in Go.

**What it buys**, and this is not nothing:

- §2.3's five rules — one entry point, everything in the request, no database handle, no
  clock, no globals, fixture-tested — stop being a discipline that can be violated by
  accident. A separate process cannot reach a database handle it was never given. The rule
  becomes structural instead of aspirational.
- A-4 (the classifier never holds database credentials) becomes testable on day one, and a
  spec scenario asserts it.
- The marginal cost of a second service is far lower now than it was before D5. The
  manifests, the image build, the live-reload loop and the CI matrix all exist for `core`
  and `web` already; the classifier is one more Deployment, one more Dockerfile and one more
  Tilt resource.
- There is no port to schedule later, and therefore no chance of the port slipping past the
  milestone that assumed it.

**Scope discipline.** The service exists; the engine does not. `/classifier` implements
`Version` and the standard gRPC health protocol and nothing else. `ClassifyBatch` — its
message shapes, the taxonomy, vendor memory, rules, threshold — is change 3.2's design work
and must not be guessed at here. Standing up the wire is this change's job; deciding what
travels over it is not.

**`/core/classify` still exists**, and still holds the `Classifier` Go interface required by
A-2. Its only implementation is now a gRPC client adapter rather than the engine itself.

**grpcio, and no FastAPI yet.** `ARCHITECTURE.md` §8 lists "grpcio + FastAPI". The skeleton
needs grpcio for the contract and the standard gRPC health-checking protocol for Kubernetes
probes, which Kubernetes supports natively. FastAPI earns its place when something needs
HTTP; adding it now would be a second server listening on a second port for no caller.

### D3 — Proto: two packages, one per boundary

Two packages, because there are now two boundaries, and they have different rules.

`/proto/vekst/v1/health.proto` is a **browser-facing Connect RPC**, served by `connect-go`
and called from the browser with `@connectrpc/connect-web`.

```protobuf
syntax = "proto3";

package vekst.v1;

option go_package = "github.com/MyauDev/vekst/core/gen/vekst/v1;vektv1";

// HealthService is browser-facing. It is the only service that may be called
// without a tenant context, and it must never read tenant data.
service HealthService {
  rpc Check(CheckRequest) returns (CheckResponse);
}

message CheckRequest {}

message CheckResponse {
  enum Status {
    STATUS_UNSPECIFIED = 0;
    STATUS_SERVING     = 1;
    STATUS_NOT_SERVING = 2;
  }

  Status status             = 1;
  string version            = 2;  // git SHA of the running core build
  string built_at           = 3;  // RFC 3339; a string, not Timestamp — see below
  string classifier_version = 4;  // best-effort; empty when the classifier is unreachable
}
```

`built_at` is a string rather than `google.protobuf.Timestamp` deliberately: the skeleton
should not depend on well-known-type resolution before anything needs it.

`classifier_version` is what makes the skeleton prove the whole chain rather than half of
it. It is **best-effort**: `core` reports `STATUS_SERVING` for itself whether or not the
classifier answers, and leaves the field empty when it does not. That encodes
`ARCHITECTURE.md` §3.5's posture — a classifier outage is a retry, not an outage of `core`
— from the first commit instead of after the first incident.

`/proto/vekst/internal/v1/classifier.proto` is an **internal `core` → `classifier` gRPC
call**, native gRPC on a private ClusterIP Service, never reachable from a browser:

```protobuf
syntax = "proto3";

package vekst.internal.v1;

option go_package = "github.com/MyauDev/vekst/core/gen/vekst/internal/v1;vektinternalv1";

// Classifier is internal. It is never exposed through the Ingress, and it never
// receives database credentials — see ARCHITECTURE.md A-4.
service Classifier {
  rpc Version(VersionRequest) returns (VersionResponse);
  // rpc ClassifyBatch(...) — change 3.2 owns the request and response shapes.
}

message VersionRequest {}

message VersionResponse {
  string engine_version = 1;  // recorded on every classification row from 3.2 onward
  string built_at       = 2;  // RFC 3339
}
```

`ClassifyBatch` is deliberately absent. `ARCHITECTURE.md` §3.2 specifies its shape, but that
shape depends on the taxonomy, the vendor-memory model and the rule matcher — none of which
exist until changes 3.1 and 3.2. Declaring it now would freeze guesses into a contract that
`buf breaking` then defends.

### D4 — Generated code is committed, and CI checks it has not drifted

`buf generate` writes Go stubs to `/core/gen`, TypeScript stubs to `/web/src/gen`, and
Python stubs to `/classifier/gen`, all three committed. `vekst/v1` generates Go and
TypeScript; `vekst/internal/v1` generates Go and Python. No package generates a language it
does not speak.

- Rationale: `go build`, `tsc`, `mypy` and every editor work with no codegen step; the
  wire-format diff is visible in review, which is what makes the `buf breaking` rule
  meaningful to a human reader. This matters more with two boundaries than it did with one.
- Python stubs are generated with `grpcio-tools` via a buf plugin, and `mypy-protobuf` so
  the Python service is type-checked against the same contract.
- Cost: merge conflicts in generated files. Mitigated by a CI job that runs `buf generate`
  and fails if `git diff --exit-code` is non-empty — resolution is always "regenerate".

*Rejected:* generating at build time and gitignoring the output. It makes the TypeScript
job depend on a Go/buf toolchain and hides contract changes from review.

### D5 — The development environment is a local Kubernetes cluster driven by Tilt

`make dev` creates a **k3d** cluster if one is absent, then runs `tilt up`. Tilt builds both
images, applies the `local` Kustomize overlay, and live-updates running containers on file
change.

```
/deploy
  Tiltfile
  docker/                     Dockerfile.core · Dockerfile.classifier · Dockerfile.web
  k8s/
    base/                     Deployment+Service core       (ClusterIP, on the Ingress)
                              Deployment+Service classifier (ClusterIP, NOT on the Ingress)
                              Deployment+Service web        (ClusterIP, on the Ingress)
                              Ingress (path routing) · kustomization.yaml
    overlays/
      local/                  dev Postgres 16 · localhost host · no resource limits
                              image tags managed by Tilt
      # a deployment overlay is added when D-6 names a host — purely additive
```

**Why Kubernetes locally at all.** The stated reason is deployment: whatever runs this in
production will run Kubernetes, and the gap between a Compose file and a Deployment is
discovered at the worst possible moment — the week before a customer demo. Developing
against the deployment artifact removes that gap. Health probes, resource requests,
config-through-`ConfigMap`, and the ingress routing in D6 are all exercised from day one
instead of being written under time pressure later.

**What it costs.** Honestly: more than Compose. Two developers now need working knowledge
of Kubernetes, and the change is roughly 8 hours larger — see the budget note in
`tasks.md`. It also pays for itself faster than it would have: the Demo is hosted, and D2
adds a third service that this machinery absorbs almost for free. The mitigations are to keep the manifests deliberately boring (plain Kustomize;
no Helm, no operators, no service mesh, no CRDs) and to get `live_update` right, because
without it Kubernetes development is materially slower than Compose and the team will route
around it.

**k3d, decided** (Q5), over kind, minikube and Docker Desktop's built-in cluster: k3d runs
k3s in Docker, and k3s is also the cheapest credible self-hosted target — which is exactly
the Hetzner default that D-6 proposes. Local and deployed then run the same distribution.
It is also the fastest of the four to create and delete on macOS/arm64. The k3s image tag is
pinned (D9), so the local Kubernetes minor version is a decision rather than an accident.
Nothing in `k8s/base` depends on the flavour, so this stays cheap to revisit.

**Kustomize, not Helm.** Two developers, two environments, no chart to distribute.
Overlays express "same workloads, different configuration" directly, and `kustomize build`
output is readable YAML that a human can diff. Helm's templating is a cost paid for
distribution flexibility this project does not need.

### D6 — Same-origin through an Ingress; the classifier is not on it

The Ingress in `k8s/base` routes by path on a single host: `/rpc/*` to the `core` Service,
everything else to the `web` Service. The browser therefore never makes a cross-origin
request, and no CORS configuration is written in this change or any later one.

**The `classifier` Service has no Ingress rule and must never acquire one.** It is a
ClusterIP reachable only from inside the cluster, and only `core` calls it. This is the
network half of `ARCHITECTURE.md` §3.3: the classifier is a pure function that `core` feeds,
not a service anyone else may address. A spec scenario asserts the absence.

This is a change from the earlier Compose-based sketch, which used the Vite dev proxy. The
Ingress is better precisely because it is the production mechanism: routing is described
once, in the base, and both the local overlay and a future deployment overlay inherit it.
Nothing about the browser's view of the API changes between environments, so CORS never
becomes a decision anyone has to make under pressure.

The web container in the `local` overlay still runs the Vite dev server, for HMR — but it
is reached through the Ingress, not directly.

### D7 — Make, not a JavaScript monorepo tool

A root `Makefile`: `make dev`, `make down`, `make gen`, `make test`, `make lint`,
`make ci`. `make dev` is a thin wrapper — ensure a k3d cluster exists, then `tilt up` — so
that a developer needs one command on day one and can drop to `tilt`/`kubectl` directly
once they want to. No Turborepo, no Nx, no workspace protocol spanning Go and TypeScript.

*Rejected:* Turborepo/Nx — they solve caching across many JS packages. There is one JS
package and one Go module, and half the repo is not JavaScript at all.
*Rejected:* Bazel — correct at a scale this project will not reach before the split.

### D8 — Directory layout enforces the track split

```
/proto/vekst/v1/            browser-facing contracts       shared, both review
/proto/vekst/internal/v1/   classifier contract (3.2)      Track B
/core/cmd/vekst-core/       main
/core/internal/server/      chi, connect handlers, config, logging, shutdown
/core/internal/ingest/      Track A          ─┐  created empty, one doc.go each,
/core/internal/dedup/       Track A           │  so both tracks have files of
/core/classify/             Track B           │  the Classifier interface + gRPC
/core/internal/report/      Track B          ─┘  client adapter (see D2)
/core/gen/                  generated, committed
/classifier/src/vekst_classifier/  Track B — grpcio server, health, version
/classifier/tests/          Track B — pytest, no network, no database
/classifier/gen/            generated, committed
/web/src/gen/               generated, committed
/deploy/Tiltfile            dev orchestration                shared, both review
/deploy/docker/             Dockerfile.core, Dockerfile.web
/deploy/k8s/base/           workloads + Ingress              shared, both review
/deploy/k8s/overlays/local/ dev Postgres, localhost host
```

`/core/classify` sits outside `internal/` on purpose: `ARCHITECTURE.md` §8 places it there,
and A-2 requires the `Classifier` interface to survive the split. Under D2 the split has
already happened, so what lives there is the interface and its gRPC client adapter — not the
engine. Everything else is under `internal/` so it cannot be imported from outside and
cannot quietly become an API.

`/classifier` belongs to Track B in full, which keeps the plan's "do not both edit the same
file" rule intact across the new language boundary.

A `CODEOWNERS` file maps those directories to the two developers, and marks `/proto`,
`/deploy/k8s/base`, `/core/internal/db` and anything touching money as requiring both
reviewers — the plan's
coordination rules, expressed as something GitHub enforces rather than something people
remember.

### D9 — Versions are pinned, not floated

| What | Pinned to | Where |
| --- | --- | --- |
| Go | 1.26 | `go.mod` |
| Node | **24**, the Active LTS line | `.nvmrc`, `packageManager`, `package-lock.json` |
| Python | **3.14** (Q7, decided) | `/classifier/.python-version`, `pyproject.toml`, `uv.lock` |
| Kubernetes | the k3s image tag k3d creates (Q5, decided) | the `make dev` target |
| buf plugins | exact versions, by tag | `buf.gen.yaml` |
| React, Tailwind, Vite | exact versions, never ranges | `package-lock.json` |
| Container base images | digests, never `latest` | the three Dockerfiles |

The local machine currently has Node 22.17.1, so Node 24 is an install, not a no-op. Node 26
enters LTS in October 2026, mid-milestone; do not chase it.

Rationale: this toolchain must resolve identically on two laptops, in CI, and on the
deployed cluster, and "works on mine" is expensive to debug in a 29-day schedule. Pinning
Kubernetes matters as much as pinning Go — an unpinned local cluster silently drifts away
from the thing you deploy to, which is the entire benefit D5 was bought for.

## Risks / Trade-offs

| Risk | Mitigation |
| --- | --- |
| **`buf breaking` has no baseline on the first merge** — comparing against `main` when `main` has no proto fails or errors | Configure the job to compare against `main` and treat "no such file" as a pass. Verify it goes red on a deliberate breaking change before trusting it |
| Tailwind v4 + React 19 + Vite version churn; v4 moved configuration into CSS | No `tailwind.config.js`; CSS-first `@import "tailwindcss"` + `@theme`. 0.1 proves the pipeline compiles only — the token layer is change 5.1's job |
| buf remote plugins make CI depend on network availability of `buf.build` | Pin plugin versions. If it becomes flaky, switch to local `protoc-gen-*` binaries installed via `go tool` — a one-file change to `buf.gen.yaml` |
| Committed generated code produces merge conflicts | The drift check makes resolution mechanical: regenerate, commit. Never hand-edit `/gen` |
| **Python 3.14 wheels for `grpcio` and `grpcio-tools` may lag on linux/arm64** — building either from source is hours, not minutes | Verify wheel availability as the first Python task (7.1), before anything depends on the version. Falling back to Python 3.13 costs one line in `.python-version` on day one and a migration later |
| Two runtimes, two dependency managers and two test pipelines from day one — the cost `ARCHITECTURE.md` §2.2 defers to Commercial | Accepted deliberately; see D2. Contain it by keeping `/classifier` owned entirely by Track B, and by keeping the contract minimal (`Version` only) until 3.2 |
| A network boundary inside the tightest milestone: `core` → `classifier` can now fail in ways a function call could not | `classifier_version` is best-effort from the first commit (D3), so a classifier outage degrades one field rather than failing `core`. The retry-with-backoff posture of §3.5 lands with 3.2, where there is something to retry |
| **The list no longer fits 4 person-days.** Kubernetes, Tilt and a third service put the change near 8 person-days | Stated, not absorbed — see the budget note in `tasks.md`, which names what to cut and what must not be cut. This is now double its allocation and the Demo's 4 days of slack cannot cover it; the plan needs re-baselining rather than the change needing trimming. The walking skeleton is the acceptance test; ship it first |
| Docker daemon is not currently running locally | Trivial, but it is a day-1 blocker for `make dev`. Named so it is discovered before Monday, not during it |
| **Kubernetes development is slower than Compose without `live_update`** — a full image build and rollout per keystroke-batch is the single fastest way to make the team abandon this setup | Treat `live_update` as load-bearing, not polish: sync Go sources and compile in-container; sync `/web/src` and let Vite HMR handle it with no pod restart. Budget the 3 hours it takes to get both right (tasks 6.6, 6.7) and measure the edit-to-visible loop |
| Kubernetes is a new skill dependency for a two-person team under a 29-day schedule | Keep the manifests deliberately boring: plain Kustomize, no Helm, no operators, no CRDs, no service mesh, no autoscaling. If a manifest needs explaining, it is too clever for this project |
| An in-cluster development Postgres invites an in-cluster production Postgres | It lives in the `local` overlay only, never in the base, so no deployment overlay can inherit it — the rule is enforced by structure rather than by memory. A spec scenario asserts it |
| The k3d/Tilt choice hardens as soon as manifests exist | Nothing in `k8s/base` depends on the cluster flavour; switching to kind is a `Tiltfile` line and a `make` target. Settle it alongside D-6 rather than in isolation |
| The layout guess for `/core/internal/*` may not survive contact with 2.2 and 3.2 | Empty packages with a `doc.go` are free to rename. Nothing depends on them in this change |

## Migration Plan

Greenfield; there is nothing to migrate. Two notes:

- `.DS_Store` is currently tracked in three places. `git rm --cached` them and add
  `.gitignore` in this change, before more files accumulate.
- Rollback is `git revert`. There is no deployed state and no data.
- `docs/ARCHITECTURE.md` A-1 and A-2, and `IMPLEMENTATION_PLAN.md` §5's
  `extract-classifier-service-python` (3 days, Commercial), are made obsolete by D2. Those
  documents should be amended so a reader six weeks from now is not following a plan that
  no longer describes the system. That edit is not in this change's scope, but it should
  not be left undone.
- No Compose file is written. If one appears from an earlier spike, delete it — two
  development environments is one more than this team can keep working.

## Open Questions

| # | Question | Blocks | Position taken here |
| --- | --- | --- | --- |
| Q1 | **D-6, hosting target.** The Demo being hosted settles *whether*, not *where* — no host, region, ingress controller or TLS issuer is named | The deployment overlay and the change that provisions it | Working assumption: the plan's own default, Hetzner Cloud (EU) with k3s, which also matches the k3d parity argument in D5. Needs confirmation before anything is provisioned |
| Q2 | ~~Is the Demo hosted?~~ **Resolved: yes, hosted.** | — | Settled. This retires the doubt over D5 and makes the Kubernetes investment worth its price. It also creates work: a change to provision the cluster, DNS, TLS and backups exists nowhere in the 37 person-days of Demo scope, and must be written and budgeted |
| Q3 | ~~Which change sets up River?~~ **Resolved: change 0.2**, where Postgres first exists | 2.2, 3.2 | Settled. Two consequences must travel with it into 0.2 and 1.1 — see "River lands in 0.2" below. 0.2's scope in `IMPLEMENTATION_PLAN.md` §3 does not mention River today and needs amending |
| Q4 | ~~Go module path~~ **Resolved: `github.com/MyauDev/vekst`.** | — | The repository moved to the MyauDev organisation. The local git remote still points at `github.com/tuxqeq/vekst` and must be updated before the first push, or the module path and the remote will disagree |
| Q5 | ~~Local cluster flavour: k3d, or kind?~~ **Resolved: k3d.** | — | Settled. k3s parity with the likely deployment target, and the fastest of the candidates to create and delete on macOS/arm64. Pin the k3s image tag so both laptops and the deployment run one Kubernetes minor version |
| Q7 | ~~Python version: 3.14, or 3.13?~~ **Resolved: Python 3.14.** | — | Settled. Task 4.1 still verifies `grpcio` and `grpcio-tools` wheels for linux/amd64 and darwin/arm64 on day one — but the outcome is now a blocker to raise, not a silent fallback to 3.13 |
| Q8 | Does Track B have the Python depth D2 assumes? Engine layers L0–L2 are now Python, which `ARCHITECTURE.md` §9 weighed against | 3.2, and the Demo schedule | Not a question this design can answer. Worth asking out loud before 3.2 starts, while switching back is still cheap |
| Q6 | Where does production Postgres live — managed, or a dedicated host? | The deployment overlay, and the backup procedure the Demo's customer data needs | Not in the cluster. Otherwise unresolved, and it is a Q1 sub-decision |

### River lands in 0.2 — what must go with it

Not this change's work, recorded here because the decision was taken here and the detail is
easy to lose between changes.

River stores its jobs in Postgres, which is why it waits for 0.2 and why it needs no broker:
a job can be inserted in the **same transaction** as the rows it is about. That is what makes
`PERSIST → CLASSIFY` safe under `ARCHITECTURE.md` §4a's atomic-import rule — the classify job
exists if and only if the transactions did.

Two consequences that are not obvious from River's own documentation:

1. **River's tables are infrastructure, not tenant tables.** `river_job`, `river_leader` and
   the rest carry no `org_id` and get no RLS policy. The CI test in `ARCHITECTURE.md` §7 that
   "lists tenant tables without an RLS policy and fails the build" will find them and go red.
   It needs an explicit allowlist with a comment saying why — and that test belongs to change
   1.1, so the allowlist has to be agreed across 0.2 and 1.1 rather than inside either.
2. **A job handler must set its own tenant context.** Because River's tables have no RLS, a
   handler must `SET LOCAL app.org_id` from its job arguments at the top of its transaction,
   exactly as an HTTP request does. The job payload is untrusted input for tenancy purposes,
   not an ambient context. Without this rule a background job runs with no tenant filter at
   all, which is the one failure `ARCHITECTURE.md` §7 exists to prevent.

## Rejected alternatives

| Rejected | Why |
| --- | --- |
| Gitignore generated code and generate at build time | Makes `tsc` depend on the Go/buf toolchain and hides wire-format changes from code review. See D3 |
| Turborepo or Nx to manage the monorepo | Built for caching across many JavaScript packages. This repo has one JS package, one Go module, and later a Python service they do not manage |
| **Keeping the engine in Go until Commercial**, per `ARCHITECTURE.md` A-1 and A-2 | The documented plan, and cheaper by its own estimate: 1.5 days to port later against 3 days to split now. Overridden at the founder's direction — see D2, which records both sides |
| Defining `ClassifyBatch` in this change, now that the Python service exists | Its shape depends on the taxonomy, vendor memory and rule matcher, none of which exist before 3.1 and 3.2. A guessed contract is one `buf breaking` then defends against correction |
| Exposing the classifier through the Ingress for easier manual testing | It would make a service that must never be publicly addressable publicly addressable, to save a `kubectl port-forward`. Tilt already port-forwards it locally |
| FastAPI alongside grpcio in the skeleton, per `ARCHITECTURE.md` §8 | A second server on a second port with no caller. Kubernetes probes the standard gRPC health protocol natively. Add FastAPI when something needs HTTP |
| Split 0.1 and 0.2 into a single `platform-foundation` change | They are one capability, but 6 person-days is a long-lived branch, and the plan's own rule is small PRs. 0.2 also fits the design template (it has DDL) while 0.1 does not |
| Scaffold with a starter template (`create-vite`, a Go layout generator) | Fine for `/web`, and it should be used there. Rejected as the *shape* of the repo — the layout in D6 exists to enforce the track split, which no generator knows about |
| **Docker Compose for local development** | Replaced by D5. The artifact you develop against is not the artifact you deploy, and that gap is discovered at the worst moment. Compose remains the cheaper and faster option, and would be the right call if the Demo turns out never to be deployed — see Q2 |
| A reverse proxy (Caddy) in front of both services for local development | The Ingress already gives same-origin, and it is the mechanism a deployment will use. Caddy would be a second routing layer that exists only on a laptop |
| Helm charts instead of Kustomize overlays | Templating is a cost paid for distributing a chart to strangers. Two developers and two environments need "same workloads, different configuration", which overlays express directly and diff readably |
| kind, minikube, or Docker Desktop's built-in cluster | All workable. k3d wins on k3s parity with the likely deployment target and on create/delete speed on macOS/arm64. Reversible — see Q5 |
| Skaffold or DevSpace instead of Tilt | Comparable tools. Tilt chosen for `live_update` ergonomics and for keeping the whole dev orchestration in one readable file |
| Running Postgres in-cluster in production, as the local overlay does | A finance product's database needs a rehearsed backup and restore procedure. Keeping it out of the base means no deployment overlay can inherit it by accident |
