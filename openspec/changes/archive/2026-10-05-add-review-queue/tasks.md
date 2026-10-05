**Budget.** `docs/IMPLEMENTATION_PLAN.md` §3 allocates **3 person-days** to change 3.3.
These tasks total **≈ 22 hours ≈ 3 person-days**, and the estimate is honest only because
the screen is change 5.2 and the chat is its own change. The part most likely to overrun is
section 4: role enforcement is new ground, and `memberships.role` has never been read.

**Ordering.** Apply after 2.5. Section 1 can be written against hand-inserted
classifications, so it does not wait for the classification worker — but the queue is not
demonstrable until that worker exists.

**Ownership.** Track B. `/proto/vekst/v1` and `/core/internal/db` need both reviewers.

## 0. Before anything else

- [x] 0.1 **Settled.** Both `T` and `N` resolve to the seeded leaf `09` OUT OF P&L; `review_decisions.outcome` keeps them apart so 2.6 can upgrade a transfer claim into a confirmed pair. No taxonomy amendment — `categories` is FORCE'd and a shared row cannot be added without a `NO FORCE` window that would desynchronise the generator (design §D2)
- [x] 0.2 **Replaced, and §5.5 now says so.** `review_items` carried a per-transaction `state`, which is a second representation of a fact `classifications` already holds; two representations drift, and this drift is silent in the worst direction — a row marked resolved with no classification is absent from the queue *and* from the report, so nobody is told the money went missing. `review_decisions` stores the thing nothing else records: the human act, one row per counterparty

## 1. Migration 008 — `review_decisions`

- [x] 1.1 The table per design §D1, with the outcome/category CHECK and the whole-undo CHECK
- [x] 1.2 The partial unique index: one live decision per counterparty per key version
- [x] 1.3 RLS: enable, `FORCE`, an ordinary tenant policy — no shared rows
- [x] 1.4 Grants: no DELETE. An undo stamps, it does not remove
- [x] 1.5 Migration 008 down, and `up → down → up`; bump the two tripwires

## 2. Queries — `core/internal/db/query/review.sql`

- [x] 2.1 `ReviewGroups`: the grouped aggregate of design §D3, filtered by entity, ordered by absolute base amount then count then key
- [x] 2.2 `ReviewGroupRows`: the transactions behind one group, for the drill-down the screen opens
- [x] 2.3 `UnclassifiedTotals`: count and sum over the whole queue, for "412 rows · 88 counterparties left"
- [x] 2.4 `InsertReviewDecision`, `UndoReviewDecision`, `DecisionsForCounterparty`
- [x] 2.5 `DeleteVendor` — the one non-append-only write in this flow (design §D5)
- [x] 2.6 Run `make gen`; confirm the codegen drift job stays green

## 3. Proto — `proto/vekst/v1/review.proto`, both reviewers

- [x] 3.1 `ListReviewGroups(entity_id, cursor, limit)` → groups with key, display name, row count, total as `vekst.type.v1.Money`
- [x] 3.2 `ListGroupTransactions(counterparty_key, cursor)` → the rows behind a group
- [x] 3.3 `ResolveGroup(counterparty_key, outcome, category_code)` → what it covered: count and total
- [x] 3.4 `UndoDecision(decision_id)` → what it reversed
- [x] 3.5 Money is `vekst.type.v1.Money` everywhere; no `double` anywhere in this file
- [x] 3.6 `buf lint` and `buf breaking`; a new file in an existing package is additive
- [x] 3.7 Run `make gen`; commit the regenerated Go, TypeScript and Python output

## 4. Role enforcement — `core/internal/identity`

- [x] 4.1 `Role(ctx, org, user)` reading `memberships`, the first read of that column
- [x] 4.2 `RequireResolver` — owner, admin or approver; everything else is `permission_denied` with a code
- [x] 4.3 The check lives in the handler path, **not** in an RLS policy (design §D4) — add a test asserting no policy references `memberships.role`
- [x] 4.4 A `viewer` can call both list RPCs and neither write RPC

## 5. `core/internal/review` — the seam

- [x] 5.1 `Group` and `Decision` domain types, money as `money.Money`
- [x] 5.2 `Queue(ctx, org, entity)` — the read, paginated, deterministic order
- [x] 5.3 `Resolve(ctx, org, decision)` — the four writes of design §D2 in one `db.InTx`, in that order
- [x] 5.4 `Undo(ctx, org, decisionID)` — supersede the classifications, delete the vendor row, stamp the decision
- [x] 5.5 Every single-row write goes through `db.ExactlyOneRow`

## 6. Connect handlers

- [x] 6.1 The four RPCs wired into the server, each opening its transaction through `db.InTx` and nowhere else
- [x] 6.2 Error codes, never sentences — and the catalogue entries change 5.1b will translate

## 7. Tests

- [x] 7.1 **Cross-tenant isolation:** A cannot see, resolve or undo B's groups, and a resolve naming B's counterparty affects nothing
- [x] 7.2 **One decision covers the group:** resolving a counterparty with 500 rows writes 500 classifications, one vendor row and one decision
- [x] 7.3 **Atomicity:** a failure in any of the four writes leaves none of them — asserted by making the classification insert collide
- [x] 7.4 **Ordering:** the biggest absolute amount comes first, a refund sorts with a payment of the same size, and two reads return the same order
- [x] 7.5 **Multi-currency ordering:** a JPY row and a EUR row sort by their base amounts, not their face values (design §D3)
- [x] 7.6 **The unidentified bucket** is present in the queue rather than hidden
- [x] 7.7 **Role:** `viewer` is refused both writes with a code; `approver`, `admin` and `owner` are allowed
- [x] 7.8 **Undo:** classifications superseded, vendor row gone, decision stamped and still present; the group reappears in the queue
- [x] 7.9 **Undo is not a delete:** the decision row survives and `classifications` still holds both generations
- [x] 7.10 **Memory works:** after resolving, a new transaction for that counterparty is answered by L0 rather than reaching the queue — the learning loop, end to end
- [x] 7.11 **Concurrency:** two resolves of one counterparty, one wins, the other fails cleanly. **It earned its complexity, and not because of the index.** The index already decided the race; what the test found is that the loser reached the client as an internal error, because a 23505 on `review_decisions_one_live_idx` was wrapped like any other database failure. Two people settling the same vendor within a second is what a shared screen produces on the first day of a month, so it now has a code of its own (`review_already_decided`, `FailedPrecondition`). The test asserts the invariant — exactly one success, one live decision, two live classifications, one vendor row, an empty queue — rather than which coded failure the loser gets, because that depends on whether the winner committed before or after the loser read the group, and pinning it down would be asserting a scheduling detail. Measured over ten runs: seven lose on the index, three on the empty group
- [x] 7.12 **No-float:** money in the generated review types is never a floating-point field

## 8. Close

- [x] 8.1 `CODEOWNERS`: `/core/internal/review/` and migration 008
- [x] 8.2 `ARCHITECTURE.md` §4.2 — the learning loop now points at `core/internal/review`, and records the two decisions inside it that are not mechanics: only a categorisation is remembered, and memory is deleted on an undo while classifications are retracted; §5.5 — `review_items` becomes `review_decisions`, with the reason
- [x] 8.3 `docs/IMPLEMENTATION_PLAN.md` §3: 3 planned, ~3.5 actual, with the two things the tests found rather than inspection. D-9 is unchanged and now says so explicitly — every vendor row carries the deciding organisation's `org_id` and the key's `key_version`, so a later yes has both the tenant boundary and the key provenance it would need
- [x] 8.4 Update the capability spec and run the full suite. Two requirements added from what the implementation settled: the concurrent resolve, and that the queue's own state is not stored
