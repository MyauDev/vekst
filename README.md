# Vekst

Management reporting for SME owners. Upload your financial data, get the reports
your business actually needs.

**Status: change 0.2, `add-postgres-and-migrations`, in progress.** `core` now
connects to Postgres 16: migrations, the two database roles, River for
background jobs, and the `Money` type. There is still no ingest, no
classification and no report.

---

## What runs today

The walking skeleton: a browser page renders a version string that travelled

```
browser ──Connect/HTTP──► core (Go) ──gRPC──► classifier (Python)
   ▲                                                │
   └────────────────── every type generated from /proto ──┘
```

Stop the classifier and the page still loads: `classifierVersion` goes empty and
`core` keeps serving. A classifier outage is a retryable condition, not an outage
of core.

## Requirements

| Tool | Version | Why that version |
| --- | --- | --- |
| Go | 1.26 | `go.mod` |
| Node | 24 | `.nvmrc`; Active LTS. Node 26 goes LTS in October — do not chase it |
| Python | 3.14 | `classifier/.python-version` |
| uv | 0.11+ | resolves `classifier/uv.lock` |
| buf | 1.72+ | lint and breaking checks on `/proto` |
| Docker | 28+ | must be running before `make dev` |
| k3d | 5.9+ | creates the local cluster |
| Tilt | 0.37+ | builds and live-reloads into it |
| kubectl, kustomize | recent | applies and renders manifests |

`goose` and `sqlc` need no separate install: both are pinned as Go tool
dependencies in `go.mod` (`go 1.24+`'s `tool` directive) and run via `go run
./core/cmd/vekst-core migrate ...` and `go tool sqlc generate` respectively.

## Getting started

```sh
make dev     # creates the k3d cluster if absent, then starts Tilt
```

Then open either:

- **http://vekst.localhost:8081** — through the cluster Ingress. `/rpc` is routed
  to `core` and everything else to `web`, which is exactly what a deployment does.
- **http://localhost:5173** — Tilt's port-forward straight to the web pod. Here
  Vite's dev proxy forwards `/rpc` to the `core` Service instead.

Both work and both are same-origin. The second exists because it is what the
Tilt UI links to; the first is the one that matches production.

`make dev` switches your kubectl context to `k3d-vekst`. The Tiltfile also pins
`allow_k8s_contexts`, so a stray kubeconfig cannot point this at a real cluster —
worth knowing if you work on other clusters from this machine.

```sh
make down    # deletes the cluster and everything in it
```

## Make targets

| Target | Does |
| --- | --- |
| `make dev` | Create the cluster if absent, then `tilt up` |
| `make down` | Delete the cluster and its volumes |
| `make gen` | Regenerate every stub from `/proto` and `/core/internal/db/query` |
| `make build` | Build `core` with its version stamped in |
| `make migrate-up` | Apply pending migrations to `DATABASE_URL_MIGRATOR` |
| `make migrate-down` | Roll back one migration on `DATABASE_URL_MIGRATOR` |
| `make lint` | `buf lint`, `go vet`, the DB-entry-point check, `ruff`, `mypy`, `tsc` |
| `make test` | `go test`, `pytest`, `vitest` |
| `make ci` | What CI runs |

## Database

Two roles, never one — the mechanism that makes `FORCE ROW LEVEL SECURITY`
meaningful rather than advisory (`CLAUDE.md`, openspec design D1):

| Role | Owns | Used by |
| --- | --- | --- |
| `vekst_migrator` | the schema; applies migrations | the migration Job only, via `DATABASE_URL_MIGRATOR` |
| `vekst_app` | nothing — DML rights only, `NOBYPASSRLS` | `core` itself, via `DATABASE_URL` |

`core` never holds `vekst_migrator` credentials. Migrations are Go's
`goose`, embedded into the `vekst-core` binary (`core/migrations`) — the
binary that migrates and the binary that serves are the same binary, so
`vekst-core migrate up|down` is what both `make migrate-up`/`make
migrate-down` and the cluster's migration Job run; the standalone `goose`
CLI cannot see `00002_river.go`'s Go migration and must not be used to apply
migrations directly.

**A psql prompt in the local cluster:**

```sh
kubectl exec -it deploy/postgres -- psql -U vekst_migrator -d vekst
```

`vekst_migrator` (not `vekst_app`) because that Deployment's own credentials
are what's mounted there; `vekst_app`'s password is `vekst_app` locally if
you want to connect as it instead — `psql "postgres://vekst_app:vekst_app@localhost:5432/vekst"`
after `kubectl port-forward svc/postgres 5432:5432`.

## Layout

```
proto/vekst/v1/            browser-facing contract  (Connect)
proto/vekst/internal/v1/   core -> classifier       (native gRPC)
proto/vekst/type/v1/       Money — shared by both contracts above
core/                      Go: chi, connect-go, pgx/v5. Owns all state
  cmd/vekst-core/          main; also `migrate up|down`
  migrations/              goose migrations, embedded into the binary
  internal/db/             the pool and InTx — the one transaction entry point
  internal/migrate/        applies migrations; derives /readyz's required version
  internal/jobs/           River: client, worker registry, the no-op job
  internal/money/          Money type, ISO-4217 exponent table, no-float guard
  internal/server/         router, handlers, /healthz, /readyz, shutdown
  internal/ingest/         Track A — change 2.1 onward
  internal/dedup/          Track A — change 2.6
  classify/                Track B — the Classifier interface + gRPC client
  internal/report/         Track B — change 4.1
  gen/                     generated, committed (buf + sqlc)
classifier/                Python: grpcio. Stateless, no database, ever
  src/vekst_classifier/    the service
  src/vekst/               generated, committed
web/                       React 19 + Vite + Tailwind v4
  src/gen/                 generated, committed
deploy/                    Tiltfile, Dockerfiles, Kustomize base + overlays
  db/rls-exempt-tables.txt infrastructure tables exempt from 1.1's RLS coverage test
```

Ownership follows `.github/CODEOWNERS`. After the foundation, the two tracks do
not edit the same files.

## Generated code is never hand-edited

`/core/gen`, `/web/src/gen` and `/classifier/src/vekst` are generated from
`/proto` by `make gen`, committed, and checked in CI. If a diff appears there,
run `make gen` and commit the result — never edit the file.

Every generator is local and pinned by its language's lockfile, so regeneration
works offline and cannot fail because a registry is rate-limiting you.

## The development loop

Tilt live-updates the running containers instead of rebuilding images.

Measured through the cluster Ingress, not against a local process:

- **Go**: ~2 s from saving a file to the change being visible.
- **Python**: ~5 s — sync, then the gRPC server restarts.
- **Web**: Vite HMR, no pod restart.

Two things had to be right for the Python loop to work, both worth knowing
before you change them:

- **The gRPC probes use `timeoutSeconds: 3`, not the 1-second default.** A
  restarting process cannot answer within a second. With the default, the
  kubelet killed the container on every live update, it came back from the
  image, and the edit silently vanished.
- **`PYTHONDONTWRITEBYTECODE=1` in the classifier image.** `live_update`
  preserves the host file's mtime, which is older than the `.pyc` Python wrote
  on its last in-container import — so Python judges the stale bytecode valid
  and keeps serving old code while the file on disk is correct.

## Known issues

- **`VEKST_VERSION` shadows the `"dev"` fallback in `buildinfo.py`.** The image
  sets it from a build arg, so editing the fallback constant appears to do
  nothing. It is not a bug, but it will cost you an afternoon if you use that
  constant to test whether a change reached the container. Change the function
  body instead.
- **`make down` leaves kubectl pointing at whatever context was there before** —
  which on this machine is a production GKE cluster. `make dev` switches back,
  and the Tiltfile pins `allow_k8s_contexts`, but a bare `kubectl` between the
  two commands talks to the wrong cluster. Check `kubectl config current-context`
  after a teardown.
- **First `make dev` after `make down` takes several minutes.** The core dev
  image carries the Go toolchain so `live_update` can recompile in place, so the
  first pull through the local registry is large. Subsequent starts reuse it.
- **The Demo has no hosted environment yet.** The manifests exist and a
  deployment overlay is purely additive, but nothing provisions a cluster, DNS,
  TLS or backups. No change owns that work.

## Documents

`docs/ARCHITECTURE.md` — the technical shape and the decisions behind it.
`docs/IMPLEMENTATION_PLAN.md` — milestones and the change list.
`docs/WORKFLOW.md` — the product flow end to end.
`openspec/changes/` — the change currently being built.