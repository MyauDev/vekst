**Budget note.** These tasks total **≈ 72 hours ≈ 9 person-days** against the 4 person-days
allocated to change 0.1 in `docs/IMPLEMENTATION_PLAN.md` §3. Two founder decisions taken
after the plan was written account for the difference:

- **The Python classifier from day one** (design D2): ≈ +15 hours — the service, its
  toolchain, the internal proto, the gRPC client in `core`, its tests and three more CI
  jobs. `ARCHITECTURE.md` §2.1 priced this split at 3 days.
- **Local Kubernetes and Tilt** (design D5): ≈ +11 hours over a Compose file, buying the
  deployment readiness the now-confirmed hosted Demo needs.
- Writing `CLAUDE.md` and a real README after the code exists adds the rest.

**What is actually free to trim: about 1.5 hours.** Task 6.3 (TanStack Router — one route
does not need it) and task 9.3 (recording required status checks) are named in no
requirement, so dropping them costs nothing but convenience.

**What is not free, despite looking it.** §8.7 (`govulncheck`, `gitleaks`) and §8.8
(container image builds) are named in the CI requirement, and §8.7 additionally carries the
"a committed secret is rejected" scenario. Deferring either makes this change fail its own
spec. They can be moved to 0.2, but only together with a MODIFIED requirement that says so —
which is a decision to take deliberately, not a corner to cut at the end of a long week.

The remaining gap is structural: these are foundation costs that move between changes but do
not shrink. **The right response is to re-baseline the Demo plan, not to shave this list** —
the capacity arithmetic in `IMPLEMENTATION_PLAN.md` §2 predates both decisions and no longer
holds, and the Demo now also needs a provisioning change that no milestone budgets at all.

Do **not** defer §2, §8.2, §8.9, or tasks 7.8–7.10 under any circumstances. Proto gating gets
harder to add once code depends on it, and without `live_update` the Kubernetes environment
is slow enough that the team will route around it.

**Ownership.** §1 and §2 are both developers, in the same room — the seam agreement from
plan §2.1, now covering two contracts rather than one. From §3 the tracks split: **B** takes
`core`, `/classifier` and CI; **A** takes the web application and the cluster environment.
`/proto` and `/deploy/k8s/base` need both reviewers.

## 1. Repository skeleton and hygiene — both

- [x] 1.1 Set the Go module path to `github.com/MyauDev/vekst`, and update the git remote — it still points at `github.com/tuxqeq/vekst`, and the two must not disagree
- [x] 1.2 Add `.gitignore` and `.editorconfig`; `git rm --cached` the three tracked `.DS_Store` files
- [x] 1.3 Create the directory layout from design D8, with a one-line `doc.go` or `__init__.py` in each empty package so both tracks own files from day one
- [x] 1.4 Add `CODEOWNERS`: ingest and dedup to Track A; classify, report and `/classifier` to Track B; `/proto` and `/deploy/k8s/base` requiring both reviewers
- [x] 1.5 Initialise `go.mod` (Go 1.26), `.nvmrc` and `packageManager` (**Node 24** — the machine has 22.17.1, so this is an install); pin exact versions, commit the lockfile
- [x] 1.6 Install `uv`; create `/classifier/pyproject.toml` and `.python-version` pinning **Python 3.14**; commit `uv.lock`

## 2. Proto and code generation — both

- [x] 2.1 Create the `buf` workspace at `/proto` with `buf.yaml`, lint and breaking configuration
- [x] 2.2 Write `/proto/vekst/v1/health.proto` as specified in design D3, including `classifier_version` and the comment marking the service browser-facing and tenant-free
- [x] 2.3 Write `/proto/vekst/internal/v1/classifier.proto` as specified in design D3 — `Version` only, commented as internal and credential-free. **Do not declare `ClassifyBatch`**; change 3.2 owns those shapes
- [x] 2.4 Write the generator templates — `buf.gen.{go,connect,grpc,web}.yaml` scoped per package, plus `grpc_tools.protoc` for Python in the `Makefile`. All generators local and pinned by their language's lockfile, after the BSR rate-limited remote plugins mid-implementation (design D4)
- [x] 2.5 Create `/proto/vekst/internal/v1/README.md` naming change 3.2 and Track B as the owner of `ClassifyBatch`, and stating why only `Version` exists today
- [x] 2.6 Add `make gen`, run it, and commit the generated output in all three languages

## 3. Go core service — B

- [x] 3.1 `cmd/vekst-core/main.go`: environment configuration, `slog` structured logging, signal handling
- [x] 3.2 `internal/server`: chi router, plain `/healthz` for container probes, graceful shutdown with a bounded drain
- [x] 3.3 Mount the connect-go `HealthService` handler under `/rpc`; the handler takes no database handle and no clock
- [x] 3.4 Stamp the git SHA and build time via `-ldflags`; return `dev` when unstamped
- [x] 3.5 `/core/classify`: the `Classifier` Go interface required by `ARCHITECTURE.md` A-2, and its gRPC client adapter with a bounded timeout. `Check` calls it best-effort and leaves `classifier_version` empty on failure

## 4. Python classifier service — B

- [x] 4.1 **First**: verify `grpcio` and `grpcio-tools` publish Python 3.14 wheels for linux/amd64 and darwin/arm64. 3.14 is decided (design Q7), so a missing wheel is a blocker to raise on day one, not a silent fallback
- [x] 4.2 Set up the `uv` project: `ruff`, `mypy` in strict mode, `pytest`; declare **no database driver and no ORM** — design D2, and a spec scenario asserts it
- [x] 4.3 Implement the grpcio server and `Classifier/Version`, returning `engine_version` and `built_at`
- [x] 4.4 Register the standard gRPC health-checking service, so Kubernetes can probe it natively with no HTTP server
- [x] 4.5 Stamp the build version into the image the same way `core` does, so the two report versions the same way

## 5. Tests — B

- [x] 5.1 Go handler test: `Check` returns `STATUS_SERVING` with a non-empty `version` and an RFC 3339 `built_at`
- [x] 5.2 Go startup test: the process starts and the health RPC succeeds with **no reachable Postgres**
- [x] 5.3 Go guard test: the health code path opens no database connection and touches no tenant-scoped table — the constraint every later change inherits
- [x] 5.4 Go degradation test: with the classifier unreachable, `Check` still returns `STATUS_SERVING`, `classifier_version` is empty, and the call returns within its timeout
- [x] 5.5 `pytest`: `Version` returns a populated response, and the gRPC health service reports serving
- [x] 5.6 `pytest`: assert the resolved dependency set contains no Postgres client — the machine-checkable form of `ARCHITECTURE.md` A-4

## 6. Web application — A

- [x] 6.1 Scaffold `/web` with Vite + React 19 + TypeScript, versions pinned exactly
- [x] 6.2 Add Tailwind v4 CSS-first (`@import "tailwindcss"`, `@theme`); no `tailwind.config.js`, no token work — that is change 5.1
- [x] 6.3 Wire TanStack Query and TanStack Router with a single route
- [x] 6.4 Configure the connect-web transport against a relative `/rpc` base URL, so the call is same-origin through the Ingress and needs no CORS in any environment
- [x] 6.5 Render `version`, `built_at` and `classifier_version` from the generated client — the walking skeleton, and the acceptance test for the whole change
- [x] 6.6 Add `vitest` and one test asserting the page renders values returned by a mocked transport

## 7. Cluster environment — A

- [x] 7.1 Multi-stage `deploy/docker/Dockerfile.core`, passing the version `-ldflags`; pin base images by digest
- [x] 7.2 `deploy/docker/Dockerfile.classifier` on a slim Python 3.14 base, installing from `uv.lock` with no build toolchain in the final layer
- [x] 7.3 `deploy/docker/Dockerfile.web` with a dev target running Vite and a static build target for later
- [x] 7.4 `deploy/k8s/base`: Deployment and Service for `core`, `classifier` and `web`. `core` and `web` probe `/healthz`; `classifier` uses a gRPC probe. All ClusterIP
- [x] 7.5 `deploy/k8s/base`: the Ingress, path-routing `/rpc/*` to `core` and everything else to `web`. **No route to `classifier`, and no Postgres in the base** — design D5 and D6
- [x] 7.6 `deploy/k8s/overlays/local`: development Postgres 16, the localhost ingress host, relaxed resource limits; confirm `kustomize build` succeeds
- [x] 7.7 `Tiltfile`: build all three images, apply the local overlay, port-forward, and label the resources so `tilt up` is readable
- [x] 7.8 Tilt `live_update` for `core` — sync Go sources, compile in-container, restart. Measure the edit-to-visible loop and record the number in the README
- [x] 7.9 Tilt `live_update` for `classifier` — sync `/classifier/src`, restart the gRPC server, no image rebuild
- [x] 7.10 Tilt `live_update` for `web` — sync `/web/src`, let Vite HMR apply it with no pod restart
- [x] 7.11 Root `Makefile`: `dev` (create the k3d cluster if absent, **pinning the k3s image tag** — an unpinned local cluster drifts away from what you deploy to — then `tilt up`), `down` (delete cluster and volumes), `gen`, `test`, `lint`, `ci`
- [x] 7.12 Verify the teardown loop: `make down` then `make dev` reproduces the environment from manifests alone, with no manual step

## 8. Continuous integration — B

- [x] 8.1 One workflow with a Go job: `go vet` and `go test`
- [x] 8.2 Add `buf lint`, and `buf breaking` against `main` configured to **pass** rather than error when `main` has no proto files
- [x] 8.3 Add the codegen drift job: run `buf generate`, fail on a non-empty `git diff --exit-code` across all three output directories
- [x] 8.4 Add the Python job: `ruff`, `mypy --strict`, `pytest`
- [x] 8.5 Add the web job: `tsc --noEmit` and `vitest run`
- [x] 8.6 Add the manifest job: `kustomize build` every overlay and validate the output with `kubeconform`
- [x] 8.7 Add `govulncheck` and `gitleaks`
- [x] 8.8 Add the container image build jobs for all three images
- [ ] 8.9 Prove the gates work: open a throwaway pull request that removes a proto field and commits a fake secret, confirm both jobs go red, then close it. An untested gate is not a gate

## 9. Documentation and close

- [x] 9.1 **After the code exists**, write `README.md`: the module path, required tool versions (Go 1.26, Node 24, Python 3.14, uv, buf, Docker, k3d, Tilt, kubectl), the `make` targets, the track split, the measured live-update loop, and the rule that generated code is never hand-edited (A)
- [x] 9.2 **After the code exists**, write `CLAUDE.md` describing the repository as built, not as planned: build and test commands, the directory layout with track ownership, and the invariants later changes must not violate — `int64` minor units for money, forced RLS on every tenant table, append-only classifications, one row per payment or posting, the classifier never holding database credentials, and `/gen` never hand-edited (B)
- [ ] 9.3 Record which CI jobs must be required status checks on `main` (B)
- [x] 9.4 Verify 9.1 and 9.2 against reality: every command they name runs, every directory they describe exists (both)
- [x] 9.5 Carry the resolved decisions into the plan: add River to change 0.2's scope in `IMPLEMENTATION_PLAN.md` §3, amend `ARCHITECTURE.md` A-1/A-2 and §5's `extract-classifier-service-python` to match design D2, and rename §3.2's `service Classifier` to `ClassifierService` to match the shipped contract (both)
- [ ] 9.6 Raise what is still open with the founder: where the hosted Demo runs (D-6 names no host), which change provisions it, and where production Postgres lives (both)
- [ ] 9.7 Update the capability spec and run the full suite
