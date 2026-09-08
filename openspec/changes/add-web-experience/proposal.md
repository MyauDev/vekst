## Why

The product has a backend contract, a database with forced row-level security and a sign-in
flow. It has no interface. `web/src` holds five components that render a health card and a
sign-in button, all five of them styled with literal Tailwind colours that `docs/DESIGN.md`
§3 has called a defect since 2026-09-06.

Two blocking inputs — D-1, the category list, and D-2, the seven real export files — are ten
days late and stall Track A at 2.2 and Track B at 3.1. `docs/FRONTEND_PLAN.md` §4 identifies
what they do *not* block, and it is most of the web work. This change is that work.

It also fixes a defect that makes the shipped sign-in flow unreachable: the Ingress routes
`/rpc` to `core` and everything else to `web`, so `/auth/google/start`, `/auth/google/callback`
and `/auth/logout` — the three routes `CLAUDE.md` names as the one non-Connect browser
surface — are answered by a static file server. `web/vite.config.ts` proxies only `/rpc`, so
the same is true under `vite dev`. Change 1.2 is recorded complete at 55 of 55 tasks with its
main flow broken in both environments, because the requirement it was written against says
"all other paths to the `web` Service".

Milestone: **Demo**. Capability: **`web-app`** (new), extending **`platform-foundation`**.

## Scope note — read before reviewing

**This change carries more than one capability delta, deliberately and against the rule in
`CLAUDE.md`.** It covers what `docs/IMPLEMENTATION_PLAN.md` splits into 5.1a, 5.1b, 5.2a,
5.2b and 5.2c, plus a landing page that has no change and no owner. It was proposed this way
by decision on 2026-09-08 rather than by oversight.

What that costs, stated so a reviewer can price it: this is a large change to review at once,
and `openspec archive` will sync several requirements to `openspec/specs/web-app` in one step.
What it buys: the token layer, the two registers and the six screens share one visual system,
and splitting them across five changes means five separate arguments about the same palette.

If it needs splitting later, §2 (shell), §5 (landing), §6–§7 (Imports, Review) and §8
(charts) are the seams — each is a section of `tasks.md` with no forward dependency on the
ones after it.

## What Changes

- **`/auth` is routed to `core`** in the Kustomize base and in the Vite dev proxy. The
  `platform-foundation` requirement that mandates the current behaviour is modified rather
  than worked around.
- **A token layer in `web/src/index.css`**, with two complete palettes. The chrome is
  monochrome and the accent is ink; the three state colours are the only hue in the
  interface, because `DESIGN.md` §2 lists the reason a figure is blocked among the things
  that may never be removed, and in a wholly monochrome interface a rejected import and an
  imported one differ by one word. Dark mode ships rather than waiting for Product.
- **`scripts/check-web-tokens.sh`, wired into `make lint`.** A literal colour class, a raw
  hex or `oklch()` value in a component, `black`/`white`, or any Register B gap above 32px
  fails the build. `DESIGN.md` §3 has priced dark mode at half a day since 2026-09-06 on the
  condition that no component writes a literal colour; nothing enforced the condition, and
  there were 22 violations by the time anyone counted.
- **The five existing components are migrated onto tokens** and the duplicate application
  shell in `src/App.tsx` is deleted. It is unreachable from `main.tsx` and survives only
  because `App.test.tsx` mounts it.
- **Two spaces in one bundle.** `/` is a public landing page; `/app/*` is the authenticated
  dashboard. Both registers share the one token layer, which is what `DESIGN.md` §1 asks for
  and what two builds would make a convention rather than a fact.
- **The product is called Veekst on screen.** `veekst.com` is the domain. The double `e` is
  the brand and lives in the message catalogue; every code identifier stays `vekst` — the Go
  module, `@vekst/web`, the `vekst_app` and `vekst_migrator` roles, the Kubernetes Services.
  Renaming those buys nothing and would touch the database roles.
- **Every screen that reads data reads it through a typed data layer.** `web/src/data/` holds
  one module per screen —  Imports, batch detail, Review, the P&L and the drill-down:
  interfaces shaped like the proto messages that do not exist yet — money as `int64`
  minor-unit strings, states as codes — with fixtures behind them. Components never touch a
  transport, so connecting the backend replaces a module body and not a component.
- **The charts from `WORKFLOW.md` §5.3, and the headline row of stat tiles.** Brought forward
  from Commercial on 2026-09-08. This forces a decision `docs/DESIGN.md` §0 explicitly
  deferred — it "does not decide chart colour ramps" — because a monochrome interface whose
  only colour is state cannot absorb eight categorical hues without a rule for how the two
  relate. The rule is now §13: hue carries identity only inside a chart's own frame, and a
  status colour is never a series colour. The palette is validated rather than chosen — every
  slot passes a lightness band, a chroma floor, colourblind separation under simulated
  protanopia and deuteranopia, and contrast against each surface, in both themes.
- **Organisation and entity are read from an extended `User`.** `proto/vekst/v1/identity.proto`
  already states that change 1.1 extends this message; 1.1's tasks do not, and no change in
  the plan gives the browser a way to learn either value. Without it `DESIGN.md` §8's top bar
  has no data source for two of its three selectors.

## Capabilities

### New Capabilities
- `web-app`: the browser interface — the token layer and its enforcement, the two registers,
  routing and URL state, and the six screens.

### Modified Capabilities
- `platform-foundation`: one requirement changes. **The browser reaches the API same-origin
  through the Ingress** — the routing sends `/rpc/*` *and* `/auth/*` to `core`, not `/rpc`
  alone. As written the requirement mandates the defect, so the Ingress was correct against
  its spec and the spec was wrong.
- `identity-access`: one requirement changes. **Signing in grants no access to data** — the
  current-user response may now carry the organisation and entity the session resolved. It
  previously required carrying *no* organisation, which was right while no tenancy model
  existed and which `add-tenancy-and-rls` has since made obsolete. Extending the `User`
  message without this delta would have shipped a contract change contradicting a live
  requirement. What does not change: no role is carried, membership authorises nothing, and
  the tenant is resolved server-side and never accepted from the client.

## Non-goals

- **The ingest error catalog.** Blocked by 2.3 — it translates error codes that do not exist.
  The i18n *mechanism* is in scope; the catalog of ingest codes is not.
- **Deviation highlighting.** `WORKFLOW.md` §5.2 puts it at Commercial and it needs the
  deterministic threshold rules, which do not exist. Charts arriving early does not bring it.
- **Real data.** Every screen renders fixtures. `IMPLEMENTATION_PLAN.md` §3 bans a Demo on
  invented data, and this change does not lift that: it builds the screens the real data will
  arrive into.
- **The `/site` Astro extraction.** `ARCHITECTURE.md` §8 schedules it at Commercial. Nothing
  here blocks it, and the landing must not reach into application state, which is what would.
- **Roles and permissions.** A `viewer` seeing the review queue read-only is Product.
- **Column-mapping UI**, **XLSX/PDF export**, **the wizard**, **payment**.

## Impact

Touches `/web` throughout, `/deploy/k8s/base/ingress.yaml`, `/scripts`, `/Makefile`,
`/proto/vekst/v1/identity.proto` and `docs/`.

`/deploy/k8s/base` and `/proto` both require **both reviewers** per `.github/CODEOWNERS`.

`docs/DESIGN.md` §3, §4, §6, §9, §11 and §12 and `docs/WORKFLOW.md` §0 are amended by this
change. They are constraint documents, and one that disagrees with the running application is
worse than none because the next person trusts it.

`add-tenancy-and-rls` archived on 2026-09-08, so the `User` extension's dependency on
`organizations` and `memberships` is satisfied. **Every section of this change is unblocked.**
