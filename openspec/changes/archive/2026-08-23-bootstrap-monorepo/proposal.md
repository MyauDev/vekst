## Why

The repository holds documents and no code. Nothing in the Demo plan starts until a
skeleton exists that compiles, generates types from proto, crosses both service boundaries
and proves itself in CI. This is change 0.1 of `docs/IMPLEMENTATION_PLAN.md` §3.

Milestone: **Demo (2026-10-01)**. Capability: **`platform-foundation`**.

## What Changes

- Go module `github.com/MyauDev/vekst` at `/core`: chi, connect-go, structured logging,
  environment config, graceful shutdown, `/healthz` for probes.
- **A Python `classifier` service at `/classifier`** — grpcio, uv, ruff, mypy, pytest.
  The service boundary stands up now rather than at Commercial. It holds no database
  credentials and answers `Version` only; `ClassifyBatch` is change 3.2.
- A `buf` workspace at `/proto` generating Go, TypeScript **and Python** stubs:
  `vekst/v1` browser-facing, `vekst/internal/v1` for `core` → `classifier`.
- React 19 + Vite + TypeScript + Tailwind v4 at `/web`, TanStack Query and Router, calling
  `core` through the generated Connect client, same-origin via Ingress.
- **The walking skeleton is the definition of done**: `tilt up`, and a browser page shows a
  version string that travelled browser → Connect → `core` → gRPC → `classifier` and back,
  through types nobody wrote by hand.
- `/deploy`: Kustomize base, a `local` overlay, and a `Tiltfile` building three images into
  a local k3d cluster with live reload. Development Postgres runs in-cluster, empty.
- One GitHub Actions workflow: `buf lint`/`breaking`, codegen drift, Go, Python, web,
  `gitleaks`, manifest validation, image builds.
- Hygiene: `.gitignore`, `.editorconfig`, `CODEOWNERS`, `README`, `CLAUDE.md`.

## Capabilities

### New Capabilities
- `platform-foundation`: the repository, both generated-code contracts, the service
  boundary, the deployment manifests, and the CI gates every later change inherits.

### Modified Capabilities

None — this is the first change.

## Non-goals

- **`ClassifyBatch` and engine layers L0–L2** — change 3.2. The wire exists; nothing
  classifies yet.
- **Postgres schema, `goose`, `sqlc`, `Money`, no-float tests** — change 0.2.
- **River** — needs the database, so 0.2. It is named in no Demo change.
- **A provisioned cluster.** The Demo is confirmed hosted, so this is required work, but
  nothing budgets a host, DNS, TLS or backups. It needs its own change.
- Auth, tenancy, RLS, ingest, reports, `/site`, i18n, dark mode.

## Impact

Creates `/proto`, `/core`, `/classifier`, `/web`, `/deploy`, `.github/workflows`.
Toolchain: Go 1.26, Node 24, Python 3.14, `uv`, `buf`, Docker, `k3d`, `tilt`, `kubectl`.
Two runtimes and two lint/test pipelines exist from day one — a cost `ARCHITECTURE.md` §2.2
defers to Commercial, brought forward deliberately.
