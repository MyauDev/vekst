## Why

Every module works. Nothing is joined to the next one.

`db.CreateOrganization` has **zero non-test callers**: sign-in writes a `users` row and a
`user_identities` row and stops, so a real customer holds no membership and every RPC
answers `ErrNotAMember`. `GetCurrentUser` carries no organisation, so the browser has no
identifier to send even if one existed. Three of the four screens read fixtures:
`web/src/data/imports.ts`, `report.ts` and `review.ts` compute their tables in the
browser, while `ReportService` and `ReviewService` sit served and uncalled.

`IMPLEMENTATION_PLAN.md` §3 defines not-done as "a demo on invented data." That is where
the P&L is today.

## What Changes

- **An organisation can be created.** A browser-facing `OrgService.CreateOrganization`
  wraps the existing `db.CreateOrganization`. It is the one RPC a caller with a session
  but no membership may call.
- **Creating one adopts the 60-leaf industry template**, in the same transaction, from a
  new global `category_templates` table. Without it the P&L has only the shared level-1
  and level-2 categories.
- **A new global `currencies` table**, seeded from `core/internal/money`, closes a gap
  `add-transaction-ledger` (2.5) left open.
- **`GetCurrentUser` carries the caller's organisations and entities.** An empty list is
  the first-run signal; the browser never guesses an identifier.
- **A first-run screen**, for a signed-in user with no organisation.
- **`imports.ts`, `report.ts` and `review.ts` call core.** `ReviewService.ListCategories`
  is added, because the review screen's category picker has no backend today.
- **One end-to-end test**: create an organisation, upload a Priorbank fixture, validate,
  persist, classify, read a P&L figure the pipeline computed.

## Non-goals

- Deployment, provisioning, hosting, DNS, TLS, backups. Out of scope by instruction.
- Role enforcement, org switching, invitations, a second entity.
- The column-mapping UI, the wizard, report-package scoring.
- Any change to the classifier contract or the engine.
- The eleven open `add-web-experience` tasks, except a direct dependency.

## Capabilities

### New Capabilities
(none)

### Modified Capabilities
- `tenancy`: organisation creation as a product action, and template adoption.
- `identity-access`: `GetCurrentUser` answers which organisations the caller belongs to.
- `web-app`: the three screens read computed data.

**Three capabilities, where `openspec/config.yaml` asks for a split.** The exception is
argued in `design.md` §0: no half is demonstrable alone.

## Impact

- New: `proto/vekst/v1/org.proto`, `core/internal/tenancy`, migration 00018,
  `web/src/data/{org,session}.ts`, `web/src/app/FirstRunScreen.tsx`.
- Modified: `proto/vekst/v1/{identity,review}.proto` (additive), `core/internal/server`,
  `web/src/data/{imports,report,review}.ts`, `AppLayout`, `TopBar`.
- `/proto` and the RLS-exempt allowlist each need two reviewers.
- Stacks on every merged Demo change. Blocks nothing.
